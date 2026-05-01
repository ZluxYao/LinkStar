package stun

import (
	"fmt"

	"net"
	"sync"
	"time"

	"github.com/pion/stun"
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
func (s *STUNService) GetBackupServer() (string, error) {

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
	// 只能单次运行
	if !s.updating.TryLock() {
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
				return
			}
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

	dalay := time.Since(star)

	return &STUNServiceDelay{Server: srv, Delay: dalay}, nil
}
