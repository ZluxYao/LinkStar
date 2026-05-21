package stun

import (
	"fmt"

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

// 获取连接延时
func getSTUNServerDelay(srv string) (*STUNServiceDelay, error) {
	star := time.Now()

	// 建立TCP连接
	conn, err := net.DialTimeout("tcp", srv, 3*time.Second)
	if err != nil {
		return nil, fmt.Errorf("TCP connection failed to be established")
	}
	defer conn.Close()

	// 发送stun请求
	msg := stun.MustBuild(stun.TransactionID, stun.BindingRequest)
	_, err = conn.Write(msg.Raw)
	if err != nil {
		return nil, fmt.Errorf("Sending STUN request failed")
	}

	// 读取响应
	conn.SetDeadline(time.Now().Add(3 * time.Second))
	buf := make([]byte, 1024)
	n, err := conn.Read(buf)
	if n == 0 || err != nil {
		return nil, fmt.Errorf("%s read failed:", err)
	}

	// 解析响应，拿到映射地址
	var response stun.Message
	response.Raw = buf[:n]
	if err := response.Decode(); err != nil {
		return nil, fmt.Errorf("decode STUN failed: %w", err)
	}
	var xorAddr stun.XORMappedAddress
	if err := xorAddr.GetFrom(&response); err != nil {
		return nil, fmt.Errorf("get mapped address failed: %w", err)
	}

	// 返回的不是公网IP，视为超时/不可用
	if isNonPublicIP(xorAddr.IP) {
		return nil, fmt.Errorf("STUN服务器 %s 返回非公网IP: %s", srv, xorAddr.IP.String())
	}

	dalay := time.Since(star)

	return &STUNServiceDelay{Server: srv, Delay: dalay}, nil
}

// 判断是不是不可用作公网的IP（私网/回环/链路本地/CGNAT等）
func isNonPublicIP(ip net.IP) bool {
	if ip == nil || ip.IsUnspecified() || ip.IsLoopback() ||
		ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsMulticast() {
		return true
	}
	// CGNAT 100.64.0.0/10，IsPrivate 不覆盖
	if ip4 := ip.To4(); ip4 != nil && ip4[0] == 100 && ip4[1] >= 64 && ip4[1] <= 127 {
		return true
	}
	return false
}
