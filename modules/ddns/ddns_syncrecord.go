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

// SyncRecordNow 同步指定记录（“单条立即同步”按钮用），同步执行以便返回即时结果
func (r *DDNSRuntime) SyncRecordNow(id uint) error {
	r.mu.RLock()
	var rec model.DDNSRecord
	var provider model.DDNSProvider
	found := false
	for i := range r.Config.Records {
		if r.Config.Records[i].ID == id {
			rec = r.Config.Records[i]
			for _, p := range r.Config.Providers {
				if p.ID == rec.ProviderID {
					provider = p
					break
				}
			}
			found = true
			break
		}
	}
	r.mu.RUnlock()

	if !found {
		return fmt.Errorf("记录不存在")
	}
	if !rec.Enabled {
		return fmt.Errorf("该记录已停用，请先启用")
	}

	client := dns.BuildClient(provider)
	if client == nil {
		return fmt.Errorf("记录所属服务商无效或未配置")
	}

	SyncRecord(client, &rec)
	r.commitRecord(&rec)

	if rec.LastStatus == model.DDNSRecordStatusFailed {
		return fmt.Errorf("%s", rec.LastMessage)
	}
	return nil
}

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

	// // 2. IP 没变，跳过
	// if ip == r.LastIP {
	// 	r.LastStatus = model.DDNSRecordStatusSkipped
	// 	return
	// }

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
	r.LastSyncAt = time.Now()
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
	case model.IPSourceCustom:
		return parseFixedIP(r.IPSourceArg, r.RecordType)
	case model.IPSourceDNS:
		return resolveFromDNS(r.IPSourceArg, r.RecordType)
	case model.IPSourceInterface:
		return resolveFromInterface(r.IPSourceArg, r.RecordType)
	default:
		return "", fmt.Errorf("暂不支持的 IP 来源: %s", r.IPSourceType)

	}

	return ip, err
}

// parseFixedIP 自定义来源：用户填的那个 IP，一个字都不改。
//
// 用途是那些「压根不会变」的记录——比如入口重定向要的 192.0.2.1 占位地址。
// 这类记录走别的来源都不对：探测出来的是真公网 IP，会把占位地址覆盖掉。
func parseFixedIP(arg string, t model.DNSRecordType) (string, error) {
	s := strings.TrimSpace(arg)
	if s == "" {
		return "", fmt.Errorf("自定义来源要在「来源参数」里填一个 IP")
	}
	ip := net.ParseIP(s)
	if ip == nil {
		return "", fmt.Errorf("「%s」不是合法的 IP 地址", s)
	}
	if err := matchRecordType(ip, t); err != nil {
		return "", err
	}
	return ip.String(), nil
}

// resolveFromDNS 跟随另一个域名：解析它，把结果写到自己头上。
// 用来让多条记录跟着一个主记录走，不用每条都各探测一次。
func resolveFromDNS(arg string, t model.DNSRecordType) (string, error) {
	host := strings.TrimSpace(arg)
	if host == "" {
		return "", fmt.Errorf("DNS 来源要在「来源参数」里填一个要跟随的域名")
	}
	ips, err := net.LookupIP(host)
	if err != nil {
		return "", fmt.Errorf("解析 %s 失败: %w", host, err)
	}
	for _, ip := range ips {
		if matchRecordType(ip, t) == nil {
			return ip.String(), nil
		}
	}
	return "", fmt.Errorf("%s 解析不出 %s 记录要的地址", host, t)
}

// resolveFromInterface 读本地网卡上的地址。
//
// 只在「这台机器自己就拿着公网地址」时有意义，最典型的是原生 IPv6：
// 地址就在网卡上，绕一圈去问外部网站反而慢且可能拿到别人的出口地址。
// arg 是网卡名，留空就在所有网卡里挑。
//
// 排除回环和链路本地（fe80::/10、169.254/16）：它们出了本机就没有意义，
// 写进公网 DNS 等于让所有人解析到一个连不上的地址。
func resolveFromInterface(arg string, t model.DNSRecordType) (string, error) {
	name := strings.TrimSpace(arg)

	ifaces, err := net.Interfaces()
	if err != nil {
		return "", fmt.Errorf("读取网卡列表失败: %w", err)
	}

	var fallback string // 私有地址（192.168 / fd00::）垫底，实在没有公网的才用
	matched := false

	for _, iface := range ifaces {
		if name != "" && !strings.EqualFold(iface.Name, name) {
			continue
		}
		matched = true
		if iface.Flags&net.FlagUp == 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			ipNet, ok := addr.(*net.IPNet)
			if !ok {
				continue
			}
			ip := ipNet.IP
			if !usableIfaceIP(ip) {
				continue
			}
			if matchRecordType(ip, t) != nil {
				continue
			}
			if ip.IsPrivate() {
				if fallback == "" {
					fallback = ip.String()
				}
				continue
			}
			return ip.String(), nil
		}
	}

	if fallback != "" {
		return fallback, nil
	}
	if name != "" && !matched {
		return "", fmt.Errorf("没有叫「%s」的网卡", name)
	}
	return "", fmt.Errorf("网卡上没找到 %s 记录能用的地址", t)
}

// matchRecordType A 记录只能填 IPv4，AAAA 只能填 IPv6。
// 填反了服务商回的报错通常很含糊，不如在本地就把话说清楚。
func matchRecordType(ip net.IP, t model.DNSRecordType) error {
	isV4 := ip.To4() != nil
	switch t {
	case model.DNSRecordTypeA:
		if !isV4 {
			return fmt.Errorf("A 记录要填 IPv4，%s 是 IPv6——把记录类型改成 AAAA", ip)
		}
	case model.DNSRecordTypeAAAA:
		if isV4 {
			return fmt.Errorf("AAAA 记录要填 IPv6，%s 是 IPv4——把记录类型改成 A", ip)
		}
	}
	return nil
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
