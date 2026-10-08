//go:build linux

package stun

import (
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"linkstar/modules/stun/model"
	"net"
	"os"
	"strconv"
	"strings"
	"time"
	"unsafe"

	"golang.org/x/sys/unix"
)

// detectOutboundIface 读主路由表里的默认路由，跳过 TUN / 隧道，跃点数最小的那条。
//
// 只看主路由表正好绕开了代理：sing-box / Clash 的 auto_route 是另开一张表再用
// ip rule 把流量引过去，主表里的默认路由还是物理网卡的；直接改主表默认路由的
// 老式 TUN 被下面的网卡类型判断排除掉。
func detectOutboundIface() (OutboundIface, error) {
	data, err := os.ReadFile("/proc/net/route")
	if err != nil {
		return detectOutboundIfaceByName()
	}

	var best *OutboundIface
	bestMetric := 0
	for _, line := range strings.Split(string(data), "\n")[1:] { // 第一行是表头
		// Iface Destination Gateway Flags RefCnt Use Metric Mask ...
		f := strings.Fields(line)
		if len(f) < 8 || f[1] != "00000000" || f[7] != "00000000" {
			continue // 不是默认路由
		}
		flags, _ := strconv.ParseUint(f[3], 16, 32)
		if flags&unix.RTF_UP == 0 || isTunnelIface(f[0]) {
			continue
		}
		iface, err := net.InterfaceByName(f[0])
		if err != nil || iface.Flags&net.FlagUp == 0 {
			continue
		}
		gw := procRouteIPv4(f[2])
		if gw == "" && iface.Flags&net.FlagPointToPoint == 0 {
			continue // 没网关又不是 PPP 这种点对点链路，不是真出口
		}
		ip, prefix := firstIPv4(iface)
		if ip == "" {
			continue
		}
		metric, _ := strconv.Atoi(f[6])
		if best == nil || metric < bestMetric {
			best = &OutboundIface{Name: iface.Name, Index: iface.Index, LocalIP: ip, PrefixLen: prefix, Gateway: gw}
			bestMetric = metric
		}
	}
	if best == nil {
		return OutboundIface{}, errNoOutboundIface
	}
	return *best, nil
}

// procRouteIPv4 /proc/net/route 打印的是「网络序 4 字节按本机字节序读成的整数」，
// 小端机器上 192.168.1.1 印成 0101A8C0。把整数按本机字节序写回去就是原来的 4 字节。
func procRouteIPv4(h string) string {
	b, err := hex.DecodeString(h)
	if err != nil || len(b) != 4 {
		return ""
	}
	var ip [4]byte
	binary.NativeEndian.PutUint32(ip[:], binary.BigEndian.Uint32(b))
	if v := net.IP(ip[:]); !v.IsUnspecified() {
		return v.String()
	}
	return ""
}

// isTunnelIface TUN/TAP、WireGuard、各种隧道
func isTunnelIface(name string) bool {
	if _, err := os.Stat("/sys/class/net/" + name + "/tun_flags"); err == nil {
		return true
	}
	t, err := os.ReadFile("/sys/class/net/" + name + "/type")
	if err != nil {
		return isTunnelName(name) // /sys 读不到才按名字猜
	}
	switch strings.TrimSpace(string(t)) {
	case "65534", "768", "776", "778": // ARPHRD_NONE(tun/wg)、IPIP、SIT、GRE
		return true
	}
	return false
}

// bindToIface SO_BINDTODEVICE：内核 5.7 之前要 CAP_NET_RAW，普通用户钉不上就算了
func (o OutboundIface) bindToIface(fd uintptr) {
	_ = unix.SetsockoptString(int(fd), unix.SOL_SOCKET, unix.SO_BINDTODEVICE, o.Name)
}

// ===================== 路由层级 =====================

// traceHops 每跳递增 TTL 发一个 UDP 包，路由器回的 ICMP 由内核放进套接字的
// 错误队列（IP_RECVERR），带着回包路由器的地址。tracepath 就是这么做的：
// 不要 root，也不依赖系统装没装 traceroute。
func (o OutboundIface) traceHops(target string, maxHops int) ([]model.NatRouterInfo, error) {
	dst := net.ParseIP(target).To4()
	if o.ip() == nil || dst == nil {
		return nil, fmt.Errorf("路由探测需要有效的 IPv4：源 %q，目标 %q", o.LocalIP, target)
	}
	fd, err := unix.Socket(unix.AF_INET, unix.SOCK_DGRAM|unix.SOCK_CLOEXEC, 0)
	if err != nil {
		return nil, err
	}
	defer unix.Close(fd)

	o.bindToIface(uintptr(fd))
	local := &unix.SockaddrInet4{}
	copy(local.Addr[:], o.ip())
	if err := unix.Bind(fd, local); err != nil {
		return nil, fmt.Errorf("绑定 %s 失败: %w", o.LocalIP, err)
	}
	if err := unix.SetsockoptInt(fd, unix.IPPROTO_IP, unix.IP_RECVERR, 1); err != nil {
		return nil, err
	}

	remote := &unix.SockaddrInet4{}
	copy(remote.Addr[:], dst)
	return collectHops(target, maxHops, func(ttl int) (string, bool, error) {
		if err := unix.SetsockoptInt(fd, unix.IPPROTO_IP, unix.IP_TTL, ttl); err != nil {
			return "", false, err
		}
		remote.Port = 33434 + ttl // traceroute 惯用的端口段，目标收到会回端口不可达
		if err := unix.Sendto(fd, []byte("LinkStar"), 0, remote); err != nil {
			return "", false, fmt.Errorf("第 %d 跳发送失败: %w", ttl, err)
		}
		hop, reached := readICMPError(fd, 500*time.Millisecond)
		return hop, reached, nil
	})
}

// readICMPError 等错误队列里来一条 ICMP 回报；超时返回空。
// 错误队列只会让 poll 报 POLLERR，等到了再带 MSG_ERRQUEUE 去读。
func readICMPError(fd int, timeout time.Duration) (hop string, reached bool) {
	const (
		icmpDestUnreach = 3
		icmpPortUnreach = 3
		icmpTimeExceed  = 11
	)
	deadline := time.Now().Add(timeout)
	buf := make([]byte, 64)
	oob := make([]byte, 256)
	for {
		left := time.Until(deadline)
		if left <= 0 {
			return "", false
		}
		pfd := []unix.PollFd{{Fd: int32(fd), Events: unix.POLLERR}}
		if n, err := unix.Poll(pfd, int(left.Milliseconds())+1); err == unix.EINTR {
			continue
		} else if err != nil || n == 0 {
			return "", false
		}

		_, oobn, _, _, err := unix.Recvmsg(fd, buf, oob, unix.MSG_ERRQUEUE)
		if err != nil {
			return "", false
		}
		msgs, _ := unix.ParseSocketControlMessage(oob[:oobn])
		for _, m := range msgs {
			if m.Header.Level != unix.IPPROTO_IP || m.Header.Type != unix.IP_RECVERR {
				continue
			}
			// sock_extended_err 后面紧跟着回 ICMP 那台机器的 sockaddr_in
			eeLen := int(unsafe.Sizeof(unix.SockExtendedErr{}))
			if len(m.Data) < eeLen+8 {
				continue
			}
			ee := (*unix.SockExtendedErr)(unsafe.Pointer(&m.Data[0]))
			if ee.Origin != unix.SO_EE_ORIGIN_ICMP {
				continue
			}
			from := net.IP(m.Data[eeLen+4 : eeLen+8]).String()
			switch {
			case ee.Type == icmpTimeExceed:
				return from, false
			case ee.Type == icmpDestUnreach && ee.Code == icmpPortUnreach:
				return from, true
			}
		}
	}
}

// detectOutboundIfaceByName /proc 没挂载这种极端情况：按名字过滤后取第一张有 IPv4 的
func detectOutboundIfaceByName() (OutboundIface, error) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return OutboundIface{}, errors.Join(errNoOutboundIface, err)
	}
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 || isTunnelName(iface.Name) {
			continue
		}
		if ip, prefix := firstIPv4(&iface); ip != "" {
			return OutboundIface{Name: iface.Name, Index: iface.Index, LocalIP: ip, PrefixLen: prefix}, nil
		}
	}
	return OutboundIface{}, errNoOutboundIface
}
