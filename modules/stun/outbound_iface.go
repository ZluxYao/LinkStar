package stun

import (
	"context"
	"errors"
	"fmt"
	"net"
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
	Name      string // 网卡名，Windows 上是「以太网」「WLAN」这种友好名
	Index     int    // 网卡序号，钉网卡用
	LocalIP   string // 本机在这张卡上的 IPv4
	PrefixLen int    // 本机地址的前缀长度，0 表示不知道
	Gateway   string // IPv4 网关；PPP 这种点对点链路可能为空
}

func (o OutboundIface) String() string {
	if o.LocalIP == "" {
		return "无可用出口"
	}
	return fmt.Sprintf("%s(%s 网关 %s)", o.Name, o.LocalIP, o.Gateway)
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

// DetectOutboundIface 现查一次出口网卡，不走缓存
func DetectOutboundIface() (OutboundIface, error) {
	return detectOutboundIface()
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

// stunDNSServers STUN 服务器域名不走系统 DNS。
//
// 代理开着 fake-ip 时，系统 DNS 把所有域名都解析成 198.18.x.x —— 那是 TUN 自己的
// 网段，包发过去必进 TUN，源地址绑得再对也没用。所以从出口网卡直连公共 DNS。
var stunDNSServers = []string{"114.114.114.114:53", "119.29.29.29:53"}

// resolve 把 "host:port" 里的域名换成 IPv4；本来就是 IP 的原样返回
func (o OutboundIface) resolve(addr string) (string, error) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return "", err
	}
	if net.ParseIP(host) != nil {
		return addr, nil
	}

	var lastErr error
	for _, server := range stunDNSServers {
		ips, err := o.lookupVia(server, host)
		if err == nil {
			return net.JoinHostPort(ips[0].String(), port), nil
		}
		lastErr = err
	}
	return "", fmt.Errorf("解析 %s 失败: %w", host, lastErr)
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
