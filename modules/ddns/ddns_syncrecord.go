package ddns

import (
	"context"
	"fmt"
	"io"
	"linkstar/modules/ddns/dns"
	"linkstar/modules/ddns/model"
	"linkstar/modules/stun"
	"net"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

// webIPSource 记录单个 IP 查询源的状态
type webIPSource struct {
	URL        string
	AvgLatency time.Duration
	FailCount  int
}

// score 综合评分，越低越靠前（失败惩罚 1s，高于正常延迟）
func (s *webIPSource) score() int64 {
	return int64(s.FailCount)*int64(1*time.Second) + int64(s.AvgLatency)
}

var defaultIPv4Sources = []*webIPSource{
	// {URL: "https://ipinfo.io/ip"},

	{URL: "https://ipv4.getip.cc"},
	{URL: "https://4.ident.me"},

	{URL: "https://myexternalip.com/raw"},
	{URL: "https://ifconfig.me/ip"},

	{URL: "https://ipv4.ip.sb"},
}

var defaultIPv6Sources = []*webIPSource{
	{URL: "https://api6.ipify.org"},
	{URL: "https://ipv6.icanhazip.com"},
	{URL: "https://ipv6.ip.sb"},
	{URL: "https://6.ident.me"},
}
var sourceMu sync.Mutex

// SyncRecord 同步记录:解析 IP -> 比对 -> 调服务商 API -> 回写状态
func SyncRecord(client dns.DNSProvider, r *model.DDNSRecord) {

	// 无论成功或者失败都标记"已经检测过了"，防止未更新再次加入队列更新
	r.LastCheckAt = time.Now()

	// 1. 解析IP来源
	ip, err := resolveIP(r)
	if err != nil {
		// 临时性失败（取 IP 失败）：不动 LastIP，下个周期自动重试
		r.LastStatus = model.DDNSRecordStatusFailed
		r.LastMessage = "获取 IP 失败: " + err.Error()
		logrus.Warnf("[ddns] %s 获取 IP 失败: %v", r.Name, err)
		return
	}

	// 2. IP 没变，跳过
	if ip == r.LastIP {
		r.LastStatus = model.DDNSRecordStatusSkipped
		return
	}

	// 3. 调用服务器API
	ttl := r.TTL
	if ttl <= 0 {
		ttl = 1 // Cloudflare 1 = auto
	}
	err = client.SetRecord(r.Domain, r.SubDomain, r.RecordType, ip, ttl, r.Proxied)
	if err != nil {
		r.LastStatus = model.DDNSRecordStatusFailed
		r.LastMessage = err.Error()
		logrus.Warnf("[ddns] %s 同步失败: %v", r.Name, err)
		return
	}

	// 4. 成功，回写
	r.LastIP = ip
	r.LastStatus = model.DDNSRecordStatusSuccess
	r.LastMessage = ""
	logrus.Infof("[ddns] %s 同步成功: %s -> %s", r.Name, r.RecordType, ip)

}

//==============================================================================

// resolveIP解析ip来源
func resolveIP(r *model.DDNSRecord) (ip string, err error) {
	switch r.IPSourceType {
	case model.IPSourceSTUN:
		// 直接读 STUN 模块已探测好的 IP
		ip := stun.Runtime.Network.PublicIP
		if ip == "" {
			// TODO 加一个备用获取可靠的STUN公网ip的函数去获取
			return "", fmt.Errorf("STUN 公网 IP 暂未就绪")
		}
		return ip, nil
	case model.IPSourceWeb:
		// HTTP GET 某网站，返回体即 IP
		if r.IPSourceArg != "" {
			// 用户自己配了 URL，直接用，不重试
			switch r.RecordType {
			case model.DNSRecordTypeA:
				return fetchIPFromWeb(r.IPSourceArg, 4)
			case model.DNSRecordTypeAAAA:
				return fetchIPFromWeb(r.IPSourceArg, 6)
			}
		}
		// 用户留空，走内置列表重试
		switch r.RecordType {
		case model.DNSRecordTypeA:
			return fetchIPFromWebList(defaultIPv4Sources, 4)
		case model.DNSRecordTypeAAAA:
			return fetchIPFromWebList(defaultIPv6Sources, 6)
		}
	default:
		return "", fmt.Errorf("暂不支持的 IP 来源: %s", r.IPSourceType)

	}

	return ip, err
}

// 初始化HTTP客户端 方便复用

func newIPClient(network string) *http.Client {
	return &http.Client{
		Timeout: 5 * time.Second,
		Transport: &http.Transport{
			Proxy: nil,
			DialContext: func(ctx context.Context, _, addr string) (net.Conn, error) {
				return (&net.Dialer{
					Timeout:   5 * time.Second,
					KeepAlive: 30 * time.Second,
				}).DialContext(ctx, network, addr)
			},
		},
	}
}

var ipv4HTTPClient = newIPClient("tcp4")
var ipv6HTTPClient = newIPClient("tcp6")

// 初始化 DefaultIPSources 排序
func initDefaultIPSources() {
	var wg sync.WaitGroup

	probe := func(sources []*webIPSource, version int) {
		for _, s := range sources {
			wg.Add(1)
			go func() {
				defer wg.Done()
				start := time.Now()
				ip, err := fetchIPFromWeb(s.URL, version)
				elapsed := time.Since(start)

				sourceMu.Lock()
				if err != nil {
					s.FailCount++
					logrus.Warnf("IP源探测失败 %s: %v", s.URL, err)
				} else {
					s.AvgLatency = elapsed
					logrus.Infof("IP源探测成功 ip %s %s: %v ", ip, s.URL, elapsed)
				}
				sourceMu.Unlock()

			}()
		}
	}

	probe(defaultIPv4Sources, 4)
	probe(defaultIPv6Sources, 6)
	wg.Wait()
}

// fetchIPFromWebList 从默认列表自动尝试
func fetchIPFromWebList(sources []*webIPSource, version int) (string, error) {
	sourceMu.Lock()
	sort.Slice(sources, func(i, j int) bool {
		return sources[i].score() < sources[j].score()
	})
	ordered := make([]*webIPSource, len(sources))
	copy(ordered, sources)
	sourceMu.Unlock()

	var lastErr error
	for _, s := range ordered {
		start := time.Now()
		ip, err := fetchIPFromWeb(s.URL, version)
		elapsed := time.Since(start)

		sourceMu.Lock()
		if err != nil {
			s.FailCount++
			logrus.Warnf("从 %s 获取 IP 失败（failCount=%d），尝试下一个: %v", s.URL, s.FailCount, err)
		} else {
			if s.AvgLatency == 0 {
				s.AvgLatency = elapsed
			} else {
				s.AvgLatency = time.Duration(float64(s.AvgLatency)*0.7 + float64(elapsed)*0.3) // 加权平均
			}
			if s.FailCount > 0 {
				s.FailCount-- // 恢复可信度
			}
		}
		sourceMu.Unlock()

		if err == nil {
			return ip, nil
		}
		lastErr = err
	}
	return "", fmt.Errorf("所有 IP 查询源均失败，最后一个错误: %w", lastErr)

}

// fetchIPFormWeb 从给定URL拉取纯文本ip
func fetchIPFromWeb(url string, version int) (string, error) {

	client := ipv4HTTPClient
	if version == 6 {
		client = ipv6HTTPClient
	}

	// 获取ip
	if url == "" {
		return "", fmt.Errorf("web 来源未配置 URL")
	}
	resp, err := client.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 256))
	if err != nil {
		return "", err
	}
	ip := strings.TrimSpace(string(body))
	if ip == "" {
		return "", fmt.Errorf("从 %s 拿到空 IP", url)
	}
	return ip, nil
}
