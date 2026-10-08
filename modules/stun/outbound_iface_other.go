//go:build !windows && !linux

package stun

import (
	"fmt"
	"linkstar/modules/stun/model"
	"net"
	"os"
	"runtime"
	"time"

	"golang.org/x/net/icmp"
	"golang.org/x/net/ipv4"
	"golang.org/x/net/route"
	"golang.org/x/sys/unix"
)

// macOS 和 BSD 一路。

// listIfaces 本机网卡，路由表里有不经过 utun 等隧道的默认路由的标 candidate。
//
// 代理 TUN 在 macOS 上加的是 1/8、2/7 …… 128/1 这一串拆分路由，原来那条
// default → en0 一直都在，所以只认目的和掩码都是 0 的那条就绕开了。
// 不用 `route get default`：开着 TUN 时它给的就是 utun。
func listIfaces() ([]OutboundIface, error) {
	list, err := listUpIfaces()
	if err != nil {
		return nil, err
	}
	for i := range list {
		list[i].Tunnel = isTunnelName(list[i].Name)
	}

	rib, err := route.FetchRIB(unix.AF_INET, route.RIBTypeRoute, 0)
	if err != nil {
		return nil, fmt.Errorf("读路由表失败: %w", err)
	}
	msgs, err := route.ParseRIB(route.RIBTypeRoute, rib)
	if err != nil {
		return nil, fmt.Errorf("解析路由表失败: %w", err)
	}

	// 路由表按优先级排好了，越靠前的默认路由越是系统本来会走的，用顺序当跃点数
	rank := uint32(0)
	for _, m := range msgs {
		rm, ok := m.(*route.RouteMessage)
		if !ok || rm.Flags&unix.RTF_UP == 0 || rm.Flags&unix.RTF_GATEWAY == 0 || len(rm.Addrs) <= unix.RTAX_NETMASK {
			continue
		}
		if !isZeroIPv4(rm.Addrs[unix.RTAX_DST]) || !isZeroIPv4(rm.Addrs[unix.RTAX_NETMASK]) {
			continue // 只要 0.0.0.0/0
		}
		gw, ok := rm.Addrs[unix.RTAX_GATEWAY].(*route.Inet4Addr)
		if !ok {
			continue
		}
		for i := range list {
			o := &list[i]
			if o.Index != rm.Index || o.Tunnel || o.candidate {
				continue
			}
			o.Gateway = net.IP(gw.IP[:]).String()
			o.candidate, o.metric = true, rank
			rank++
		}
	}
	return list, nil
}

func isZeroIPv4(a route.Addr) bool {
	switch v := a.(type) {
	case nil:
		return true // 默认路由的掩码在报文里常常直接省掉
	case *route.Inet4Addr:
		return v.IP == [4]byte{}
	}
	return false
}

// bindToIface IP_BOUND_IF：把 IPv4 套接字钉在某张网卡上（只有 macOS 有，BSD 只绑源地址）。
// macOS 上实测只绑源地址已经够了，这里多钉一下防着哪家代理换了路由的玩法。
func (o OutboundIface) bindToIface(fd uintptr) {
	const ipBoundIF = 25 // x/sys 只在 darwin 下导出这个常量
	if runtime.GOOS == "darwin" {
		_ = unix.SetsockoptInt(int(fd), unix.IPPROTO_IP, ipBoundIF, o.Index)
	}
}

// ===================== 路由层级 =====================

// traceHops 用不要 root 的 ICMP 套接字（SOCK_DGRAM + IPPROTO_ICMP）每跳递增 TTL
// 发 echo，中间路由回的 TTL 超时也能收到。系统的 traceroute 要 setuid，这里用不上它。
func (o OutboundIface) traceHops(target string, maxHops int) ([]model.NatRouterInfo, error) {
	dst := net.ParseIP(target).To4()
	if o.ip() == nil || dst == nil {
		return nil, fmt.Errorf("路由探测需要有效的 IPv4：源 %q，目标 %q", o.LocalIP, target)
	}
	fd, err := unix.Socket(unix.AF_INET, unix.SOCK_DGRAM, unix.IPPROTO_ICMP)
	if err != nil {
		return nil, fmt.Errorf("创建 ICMP 套接字失败: %w", err)
	}
	defer unix.Close(fd)

	const ipStripHdr = 0x17 // 收到的包去掉 IP 头，直接就是 ICMP 报文
	_ = unix.SetsockoptInt(fd, unix.IPPROTO_IP, ipStripHdr, 1)
	o.bindToIface(uintptr(fd))
	local := &unix.SockaddrInet4{}
	copy(local.Addr[:], o.ip())
	if err := unix.Bind(fd, local); err != nil {
		return nil, fmt.Errorf("绑定 %s 失败: %w", o.LocalIP, err)
	}
	_ = unix.SetsockoptTimeval(fd, unix.SOL_SOCKET, unix.SO_RCVTIMEO, &unix.Timeval{Usec: 500000})

	remote := &unix.SockaddrInet4{}
	copy(remote.Addr[:], dst)
	id := os.Getpid() & 0xffff
	return collectHops(target, maxHops, func(ttl int) (string, bool, error) {
		if err := unix.SetsockoptInt(fd, unix.IPPROTO_IP, unix.IP_TTL, ttl); err != nil {
			return "", false, err
		}
		echo := icmp.Message{Type: ipv4.ICMPTypeEcho, Body: &icmp.Echo{ID: id, Seq: ttl, Data: []byte("LinkStar")}}
		pkt, _ := echo.Marshal(nil)
		if err := unix.Sendto(fd, pkt, 0, remote); err != nil {
			return "", false, fmt.Errorf("第 %d 跳发送失败: %w", ttl, err)
		}

		deadline := time.Now().Add(500 * time.Millisecond)
		buf := make([]byte, 1500)
		for time.Now().Before(deadline) {
			n, from, err := unix.Recvfrom(fd, buf, 0)
			if err != nil {
				return "", false, nil
			}
			msg, err := icmp.ParseMessage(1, buf[:n])
			addr, ok := from.(*unix.SockaddrInet4)
			if err != nil || !ok {
				continue
			}
			switch msg.Type {
			case ipv4.ICMPTypeTimeExceeded:
				return net.IP(addr.Addr[:]).String(), false, nil
			case ipv4.ICMPTypeEchoReply:
				return net.IP(addr.Addr[:]).String(), true, nil
			}
		}
		return "", false, nil
	})
}
