package stun

import (
	"context"
	"errors"
	"fmt"
	"linkstar/modules/stun/model"
	"net"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/libp2p/go-reuseport"
)

// 出口网卡：关掉代理 TUN 之后，系统本来会从哪张网卡、哪个网关出去。
//
// 打洞、UPnP、查公网 IP、测 NAT 都得认这张卡：开着 Clash / sing-box 的 TUN 时，
// 默认路由被 TUN 接管，不指定出口的连接全进了 TUN —— 查到的公网 IP 是代理节点的，
// DDNS 会把域名推过去；UDP 的 STUN 包更是有去无回。
//
// 只绑源地址在 Windows 和 macOS 上就够（TUN 加的是拆分路由，物理网卡那条默认
// 路由还在）；Linux 的代理用策略路由，光绑地址不一定绕得开，所以再把套接字钉在
// 网卡上（各平台的 bindToIface）。钉不上不算错，源地址还兜着。
//
// 各平台怎么找这张卡、怎么钉、怎么测路由层级，见 outbound_iface_<os>.go。

// OutboundIface 一张出口网卡
type OutboundIface struct {
	Name      string `json:"name"`    // 网卡名，Windows 上是「以太网」「WLAN」这种友好名
	Index     int    `json:"-"`       // 网卡序号，钉网卡用；0 表示不钉（跟随系统）
	LocalIP   string `json:"localIP"` // 本机在这张卡上的 IPv4
	PrefixLen int    `json:"-"`       // 本机地址的前缀长度，0 表示不知道
	Gateway   string `json:"gateway"` // IPv4 网关；PPP 这种点对点链路可能为空

	Tunnel bool `json:"tunnel"` // TUN / VPN 这类隧道，自动模式不会选它
	Auto   bool `json:"auto"`   // 自动模式会选的就是它，只在网卡列表里标

	candidate bool   // 平台判断这张能当出口：有默认路由、不是隧道
	metric    uint32 // 候选之间比大小，小的优先
}

func (o OutboundIface) String() string {
	if o.LocalIP == "" {
		return "无可用出口"
	}
	return fmt.Sprintf("%s(%s 网关 %s)", o.Name, o.LocalIP, o.Gateway)
}

// same 出口变没变：卡、地址、网关有一样不同就算变了
func (o OutboundIface) same(p OutboundIface) bool {
	return o.Name == p.Name && o.Index == p.Index && o.LocalIP == p.LocalIP &&
		o.PrefixLen == p.PrefixLen && o.Gateway == p.Gateway
}

// Subnet 本机所在的网段，前缀长度不知道时按家用最常见的 /24
func (o OutboundIface) Subnet() *net.IPNet {
	ip := net.ParseIP(o.LocalIP).To4()
	if ip == nil {
		return nil
	}
	ones := o.PrefixLen
	if ones <= 0 || ones > 32 {
		ones = 24
	}
	mask := net.CIDRMask(ones, 32)
	return &net.IPNet{IP: ip.Mask(mask), Mask: mask}
}

var errNoOutboundIface = errors.New("没有可用的出口网卡（需要已连接、不是 TUN/VPN、有 IPv4 网关）")

// networkConfig 当前生效的网络设置（出口网卡、DNS 用哪种模式），
// InitSTUN 和设置接口写，挑网卡、解析域名时读
var networkConfig atomic.Pointer[model.NetworkConfig]

func currentNetworkConfig() model.NetworkConfig {
	if c := networkConfig.Load(); c != nil {
		return *c
	}
	return model.NetworkConfig{}
}

// DetectOutboundIface 按网络设置现查一次出口网卡，不走缓存
func DetectOutboundIface() (OutboundIface, error) {
	list, err := listIfaces()
	if err != nil {
		return OutboundIface{}, err
	}
	switch cfg := currentNetworkConfig(); cfg.IfaceMode {
	case model.ModeSystem:
		return systemRouteIface(list)
	case model.ModeCustom:
		for _, o := range list {
			if o.Name == cfg.Iface {
				return o, nil
			}
		}
		// 不自动换别的卡：指定网卡多半是两条宽带选线路，换过去 DDNS 就把域名推到另一条上了
		return OutboundIface{}, fmt.Errorf("指定的网卡 %s 现在不可用（没连上或没有 IPv4）", cfg.Iface)
	default:
		return pickAutoIface(list)
	}
}

// ListOutboundIfaces 本机所有已连接、有 IPv4 的网卡，自动模式会选的那张标上 Auto
func ListOutboundIfaces() ([]OutboundIface, error) {
	list, err := listIfaces()
	if err != nil {
		return nil, err
	}
	if auto, err := pickAutoIface(list); err == nil {
		for i := range list {
			list[i].Auto = list[i].same(auto)
		}
	}
	return list, nil
}

// pickAutoIface 候选里跃点数最小的；并列取先出现的
func pickAutoIface(list []OutboundIface) (OutboundIface, error) {
	var best *OutboundIface
	for i := range list {
		if list[i].candidate && (best == nil || list[i].metric < best.metric) {
			best = &list[i]
		}
	}
	if best == nil {
		return OutboundIface{}, errNoOutboundIface
	}
	return *best, nil
}

// systemRouteIface 跟随系统：让系统对公网地址选一次路，用它挑的那个源地址。
// UDP 的 Dial 不发包。Index 置 0，不钉网卡 —— 开着 TUN 时就是从 TUN 出去。
func systemRouteIface(list []OutboundIface) (OutboundIface, error) {
	conn, err := net.Dial("udp4", "114.114.114.114:53")
	if err != nil {
		return OutboundIface{}, fmt.Errorf("系统没有可用的路由: %w", err)
	}
	ip := conn.LocalAddr().(*net.UDPAddr).IP.String()
	conn.Close()

	o := OutboundIface{LocalIP: ip}
	for _, item := range list {
		if item.LocalIP == ip {
			o = item
			break
		}
	}
	o.Index = 0
	return o, nil
}

// currentOutboundIface 由网络更新器每 5 秒刷新一次（network.go）
var currentOutboundIface atomic.Pointer[OutboundIface]

// CurrentOutboundIface 当前出口；更新器还没跑过时现查一次
func CurrentOutboundIface() OutboundIface {
	if o := currentOutboundIface.Load(); o != nil {
		return *o
	}
	o, _ := DetectOutboundIface()
	currentOutboundIface.CompareAndSwap(nil, &o)
	return *currentOutboundIface.Load()
}

// control 套接字建好、还没 bind/connect 时钉到网卡上
func (o OutboundIface) control(network, address string, c syscall.RawConn) error {
	if o.Index > 0 {
		_ = c.Control(func(fd uintptr) { o.bindToIface(fd) })
	}
	return nil
}

func (o OutboundIface) ip() net.IP {
	return net.ParseIP(o.LocalIP).To4()
}

// dialer 源地址绑在出口网卡上的 Dialer；出口未知时就是普通 Dialer，交给系统选路
func (o OutboundIface) dialer(network string, timeout time.Duration) *net.Dialer {
	d := &net.Dialer{Timeout: timeout, Control: o.control}
	if ip := o.ip(); ip != nil {
		if strings.HasPrefix(network, "udp") {
			d.LocalAddr = &net.UDPAddr{IP: ip}
		} else {
			d.LocalAddr = &net.TCPAddr{IP: ip}
		}
	}
	return d
}

// DialTCP 从出口网卡发起 TCP 连接；addr 是域名时也从出口去解析
func (o OutboundIface) DialTCP(addr string, timeout time.Duration) (net.Conn, error) {
	addr, err := o.resolve(addr)
	if err != nil {
		return nil, err
	}
	return o.dialer("tcp4", timeout).Dial("tcp4", addr)
}

// ListenUDP 在出口网卡上开一个临时 UDP 套接字
func (o OutboundIface) ListenUDP() (*net.UDPConn, error) {
	lc := net.ListenConfig{Control: o.control}
	addr := "0.0.0.0:0"
	if ip := o.ip(); ip != nil {
		addr = net.JoinHostPort(ip.String(), "0")
	}
	pc, err := lc.ListenPacket(context.Background(), "udp4", addr)
	if err != nil {
		return nil, err
	}
	return pc.(*net.UDPConn), nil
}

// DialReusable 端口复用 + 钉网卡，替代 reuseport.Dial。
// 打洞要在同一个本地端口上既连 STUN 又监听入站，两样都得要。
func (o OutboundIface) DialReusable(network, localAddr, remoteAddr string) (net.Conn, error) {
	local, err := reuseport.ResolveAddr(network, localAddr)
	if err != nil {
		return nil, err
	}
	remoteAddr, err = o.resolve(remoteAddr)
	if err != nil {
		return nil, err
	}
	d := net.Dialer{
		LocalAddr: local,
		Timeout:   3 * time.Second,
		Control: func(network, address string, c syscall.RawConn) error {
			if err := reuseport.Control(network, address, c); err != nil {
				return err
			}
			return o.control(network, address, c)
		},
	}
	return d.Dial(network, remoteAddr)
}

// ResolveUDP 从出口网卡解析 "host:port"
func (o OutboundIface) ResolveUDP(addr string) (*net.UDPAddr, error) {
	addr, err := o.resolve(addr)
	if err != nil {
		return nil, err
	}
	return net.ResolveUDPAddr("udp4", addr)
}

// listUpIfaces 已连接、有 IPv4、不是回环的网卡；网关、候选由各平台按路由表补上
func listUpIfaces() ([]OutboundIface, error) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil, fmt.Errorf("读取网卡列表失败: %w", err)
	}
	var list []OutboundIface
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		if ip, prefix := firstIPv4(&iface); ip != "" {
			list = append(list, OutboundIface{Name: iface.Name, Index: iface.Index, LocalIP: ip, PrefixLen: prefix})
		}
	}
	return list, nil
}

// firstIPv4 网卡上第一个能用的 IPv4（排除回环和 169.254 链路本地）
func firstIPv4(iface *net.Interface) (ip string, prefixLen int) {
	addrs, err := iface.Addrs()
	if err != nil {
		return "", 0
	}
	for _, a := range addrs {
		ipNet, ok := a.(*net.IPNet)
		if !ok {
			continue
		}
		if v4 := ipNet.IP.To4(); v4 != nil && !v4.IsLoopback() && !v4.IsLinkLocalUnicast() {
			ones, _ := ipNet.Mask.Size()
			return v4.String(), ones
		}
	}
	return "", 0
}

// isTunnelName 名字一看就是隧道 / 虚拟网卡的。只在读不到网卡类型时兜底用。
func isTunnelName(name string) bool {
	n := strings.ToLower(name)
	for _, p := range []string{
		"lo", "tun", "tap", "utun", "ipsec", "wg", "tailscale", "zt",
		"docker", "br-", "veth", "virbr", "vmnet", "vboxnet", "bridge", "gif", "stf",
		"et_", "meta", "clash", "singbox", "sing-box",
	} {
		if strings.HasPrefix(n, p) {
			return true
		}
	}
	return false
}

// ===================== DNS =====================

// DefaultDNSServers STUN 服务器域名默认不走系统 DNS。
//
// 代理开着 fake-ip 时，系统 DNS 把所有域名都解析成 198.18.x.x —— 那是 TUN 自己的
// 网段，包发过去必进 TUN，源地址绑得再对也没用。所以从出口网卡直连公共 DNS。
var DefaultDNSServers = []string{"114.114.114.114", "119.29.29.29"}

// dnsServers 按设置挑 DNS；返回 nil 表示跟随系统
func dnsServers() []string {
	cfg := currentNetworkConfig()
	switch {
	case cfg.DNSMode == model.ModeSystem:
		return nil
	case cfg.DNSMode == model.ModeCustom && len(cfg.DNS) > 0:
		return cfg.DNS
	default:
		return DefaultDNSServers
	}
}

// resolve 把 "host:port" 里的域名换成 IPv4；本来就是 IP 的原样返回
func (o OutboundIface) resolve(addr string) (string, error) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return "", err
	}
	if net.ParseIP(host) != nil {
		return addr, nil
	}

	servers := dnsServers()
	if servers == nil {
		ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
		defer cancel()
		ips, err := net.DefaultResolver.LookupIP(ctx, "ip4", host)
		if err != nil {
			return "", fmt.Errorf("解析 %s 失败: %w", host, err)
		}
		return net.JoinHostPort(ips[0].String(), port), nil
	}

	var lastErr error
	for _, server := range servers {
		ips, err := o.lookupVia(withDNSPort(server), host)
		if err == nil {
			return net.JoinHostPort(ips[0].String(), port), nil
		}
		lastErr = err
	}
	return "", fmt.Errorf("解析 %s 失败: %w", host, lastErr)
}

// withDNSPort "114.114.114.114" → "114.114.114.114:53"，带了端口的原样返回
func withDNSPort(server string) string {
	if _, _, err := net.SplitHostPort(server); err == nil {
		return server
	}
	return net.JoinHostPort(server, "53")
}

// NormalizeDNSServer 校验并整理用户填的一个 DNS 地址，只认 IPv4，可带端口
func NormalizeDNSServer(s string) (string, error) {
	s = strings.TrimSpace(s)
	host, port := s, ""
	if h, p, err := net.SplitHostPort(s); err == nil {
		host, port = h, p
	}
	if ip := net.ParseIP(host); ip == nil || ip.To4() == nil {
		return "", fmt.Errorf("「%s」不是 IPv4 地址", s)
	}
	if port == "" {
		return host, nil
	}
	if n, err := strconv.Atoi(port); err != nil || n < 1 || n > 65535 {
		return "", fmt.Errorf("「%s」的端口不对", s)
	}
	return net.JoinHostPort(host, port), nil
}

// lookupVia 从出口网卡去问指定的那台 DNS
func (o OutboundIface) lookupVia(server, host string) ([]net.IP, error) {
	r := &net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, network, _ string) (net.Conn, error) {
			network = network[:3] + "4" // 回包被截断时解析器会改用 tcp 重问
			return o.dialer(network, 0).DialContext(ctx, network, server)
		},
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	return r.LookupIP(ctx, "ip4", host)
}
