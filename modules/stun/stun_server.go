package stun

import (
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"sync"
	"time"

	"github.com/pion/stun"
	"github.com/sirupsen/logrus"
)

type STUNServiceDelay struct {
	Server string
	Delay  time.Duration
}

type STUNService struct {
	BestSTUNServer       string
	AvailableSTUNServers []string
	AllSTUNServers       []string

	updating sync.Mutex // 保证 Update 单次运行
}

// 创建并初始化 STUNService
func NewSTUNService(allSTUNServers []string) *STUNService {
	s := &STUNService{
		AllSTUNServers: allSTUNServers,
	}
	s.UpdateSTUNService()
	return s
}

// 返回最优STUN服务器
func (s *STUNService) GetBestSTUNServer() (string, error) {
	if s.BestSTUNServer == "" {
		return "", fmt.Errorf("no best STUN server")
	}

	return s.BestSTUNServer, nil
}

// 返回备用STUN服务器
func (s *STUNService) GetBackupSTUNServer() (string, error) {

	// 如果没有可用服务器，先更新在返回
	if len(s.AvailableSTUNServers) == 0 {
		s.UpdateSTUNService()
		if s.BestSTUNServer == "" {
			return "", fmt.Errorf("no best STUN server")
		}
		return s.BestSTUNServer, nil
	}

	var wg sync.WaitGroup
	resultCh := make(chan string, len(s.AvailableSTUNServers))

	//遍历测试返回可用的
	wg.Add(len(s.AvailableSTUNServers))
	for _, server := range s.AvailableSTUNServers {
		go func() {
			defer wg.Done()
			serverDeelay, err := getSTUNServerDelay(server)
			if err != nil {
				return
			}

			resultCh <- serverDeelay.Server
		}()
	}

	go func() {
		wg.Wait()
		close(resultCh)
	}()

	// 只要拿到可用服务器直接返回
	if server, ok := <-resultCh; ok {
		return server, nil
	}

	//如果遍历完了都没有
	s.UpdateSTUNService()
	if s.BestSTUNServer == "" {
		return s.BestSTUNServer, fmt.Errorf("no available STUN server")
	}
	return s.BestSTUNServer, nil
}

// 更新当前STUN服务器
func (s *STUNService) UpdateSTUNService() {
	// 只能单次运行,当有运行，等待运行完成即可返回
	if !s.updating.TryLock() {
		s.updating.Lock()
		s.updating.Unlock()
		return
	}
	defer s.updating.Unlock()

	results := make(chan STUNServiceDelay, len(s.AllSTUNServers))

	// 遍历测试每一个STUN服务器
	var wg sync.WaitGroup
	for _, server := range s.AllSTUNServers {
		wg.Add(1)
		go func(srv string) {
			defer wg.Done()
			res, err := getSTUNServerDelay(srv)
			if err != nil {
				logrus.Info(err)
				return
			}
			logrus.Infof("STUN服务器 %s 延时: %v", res.Server, res.Delay)
			results <- *res
		}(server)
	}

	go func() {
		wg.Wait()
		close(results)
	}()

	// 获取最优的STUN服务器
	var bestServer string
	var bestDelay time.Duration = time.Hour
	var available []string
	for res := range results {
		available = append(available, res.Server)
		if res.Delay < bestDelay {
			bestDelay = res.Delay
			bestServer = res.Server
		}
	}

	s.BestSTUNServer = bestServer
	s.AvailableSTUNServers = available
}

// stunDialTimeout 一次 Binding 的拨号和读写上限。
// 用 var 是为了测试能把它调短，运行时没有别的地方改它。
var stunDialTimeout = 3 * time.Second

// dialSTUNBinding 先在 TCP 上做完一次 Binding，任何一步失败再在 UDP 上做一遍。
//
// 只在 DialTCP 失败时改走 UDP 不够：有的服务器 TCP 端口能握手，但不回答 STUN，
// 同一地址的 UDP Binding 却是通的。写请求之前就要设好超时，否则对端不回包会一直挂着。
func dialSTUNBinding(srv string) (net.IP, error) {
	o := CurrentOutboundIface()
	ip, tcpErr := bindingOn(srv, o.DialTCP, true)
	if tcpErr == nil {
		return ip, nil
	}
	ip, udpErr := bindingOn(srv, o.DialUDP, false)
	if udpErr != nil {
		return nil, fmt.Errorf("连接 %s 失败: TCP %v；UDP %v", srv, tcpErr, udpErr)
	}
	logrus.Infof("STUN 服务器 %s 的 TCP Binding 失败，改用 UDP：%v", srv, tcpErr)
	return ip, nil
}

func bindingOn(srv string, dial func(string, time.Duration) (net.Conn, error), tcp bool) (net.IP, error) {
	conn, err := dial(srv, stunDialTimeout)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	return readSTUNBinding(conn, srv, tcp)
}

// readSTUNBinding 在已连接的套接字上完成一次 Binding。超时在写请求之前设好，
// TCP 按帧读和 UDP 按数据报读共用这一次 deadline。
func readSTUNBinding(conn net.Conn, srv string, tcp bool) (net.IP, error) {
	if err := conn.SetDeadline(time.Now().Add(stunDialTimeout)); err != nil {
		return nil, fmt.Errorf("设置 %s 超时失败: %w", srv, err)
	}
	msg := stun.MustBuild(stun.TransactionID, stun.BindingRequest)
	if _, err := conn.Write(msg.Raw); err != nil {
		return nil, fmt.Errorf("向 %s 发送 STUN 请求失败: %w", srv, err)
	}
	raw, err := readSTUNMessage(conn, tcp)
	if err != nil {
		return nil, fmt.Errorf("读取 %s 的 STUN 响应失败: %w", srv, err)
	}
	var response stun.Message
	response.Raw = raw
	if err := response.Decode(); err != nil {
		return nil, fmt.Errorf("解码 %s 的 STUN 响应失败: %w", srv, err)
	}
	var xorAddr stun.XORMappedAddress
	if err := xorAddr.GetFrom(&response); err != nil {
		return nil, fmt.Errorf("读取 %s 的映射地址失败: %w", srv, err)
	}
	return xorAddr.IP, nil
}

// stunHeaderLen 是 STUN 固定头长度。头里 [2:4] 是属性区的大端长度，不含这 20 字节。
const stunHeaderLen = 20

// stunMaxBodyLen 挡住把长度字段写成超大值、把选路卡在一次读上的响应。
const stunMaxBodyLen = 4096

// readSTUNMessage 按传输方式收一条 STUN 消息。
//
// TCP 是字节流：一次 Read 可能只拿到头的前半段，接着 Decode 会报
// “not enough bytes to read header”，一台其实可用的 TCP 服务器就被丢掉。
// UDP 端口如果也不通，打洞就起不来。所以 TCP 先读满 20 字节头，再按长度读属性。
// UDP 一个数据报就是一条消息，保持一次 Read。
func readSTUNMessage(conn net.Conn, tcp bool) ([]byte, error) {
	if !tcp {
		buf := make([]byte, 1024)
		n, err := conn.Read(buf)
		if n == 0 || err != nil {
			if err == nil {
				err = io.ErrUnexpectedEOF
			}
			return nil, err
		}
		return buf[:n], nil
	}

	header := make([]byte, stunHeaderLen)
	if _, err := io.ReadFull(conn, header); err != nil {
		return nil, err
	}
	bodyLen := int(binary.BigEndian.Uint16(header[2:4]))
	if bodyLen > stunMaxBodyLen {
		return nil, fmt.Errorf("STUN 响应体过大: %d", bodyLen)
	}
	raw := make([]byte, stunHeaderLen+bodyLen)
	copy(raw, header)
	if _, err := io.ReadFull(conn, raw[stunHeaderLen:]); err != nil {
		return nil, err
	}
	return raw, nil
}

// 获取连接延时
//
// 从出口网卡发出：开着 TUN 时不指定出口，量到的是到代理节点的延迟，
// 挑出来的「最快」服务器对真实出口毫无意义。
func getSTUNServerDelay(srv string) (*STUNServiceDelay, error) {
	star := time.Now()

	ip, err := dialSTUNBinding(srv)
	if err != nil {
		return nil, err
	}
	// 返回的不是公网IP，视为超时/不可用
	if isNonPublicIP(ip) {
		return nil, fmt.Errorf("STUN服务器 %s 返回非公网IP: %s", srv, ip.String())
	}

	return &STUNServiceDelay{Server: srv, Delay: time.Since(star)}, nil
}

// 判断是不是不可用作公网的IP（私网/回环/链路本地/CGNAT等）
func isNonPublicIP(ip net.IP) bool {
	if ip == nil || ip.IsUnspecified() || ip.IsLoopback() ||
		ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsMulticast() {
		return true
	}
	ip4 := ip.To4()
	if ip4 == nil {
		return false
	}
	// CGNAT 100.64.0.0/10，IsPrivate 不覆盖
	if ip4[0] == 100 && ip4[1] >= 64 && ip4[1] <= 127 {
		return true
	}
	// 0.0.0.0/8 和 240.0.0.0/4 都不是可路由公网。云上的路由扫描会在 242.x 停住。
	if ip4[0] == 0 || ip4[0] >= 240 {
		return true
	}
	return false
}
