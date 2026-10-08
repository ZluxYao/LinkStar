//go:build darwin

package main

import (
	"errors"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"time"

	"golang.org/x/net/icmp"
	"golang.org/x/net/ipv4"
	"golang.org/x/net/route"
	"golang.org/x/sys/unix"
)

func printRouteEnv() {
	fmt.Println("  $ netstat -rn -f inet（前 25 行）")
	runCmd(25, "netstat", "-rn", "-f", "inet")
	fmt.Println("  $ route -n get " + traceTarget + "   ← 不绑定时实际走哪张卡")
	runCmd(12, "route", "-n", "get", traceTarget)
}

// detectOutboundIface 用路由套接字读整张 IPv4 路由表，挑不经过 utun 等隧道的默认路由
func detectOutboundIface() (outIface, error) {
	rib, err := route.FetchRIB(unix.AF_INET, route.RIBTypeRoute, 0)
	if err != nil {
		return outIface{}, fmt.Errorf("读路由表失败: %w", err)
	}
	msgs, err := route.ParseRIB(route.RIBTypeRoute, rib)
	if err != nil {
		return outIface{}, fmt.Errorf("解析路由表失败: %w", err)
	}

	var best *outIface
	for _, m := range msgs {
		rm, ok := m.(*route.RouteMessage)
		if !ok || rm.Flags&unix.RTF_UP == 0 || rm.Flags&unix.RTF_GATEWAY == 0 {
			continue
		}
		if len(rm.Addrs) <= unix.RTAX_NETMASK || !isZeroV4(rm.Addrs[unix.RTAX_DST]) {
			continue // 只要 0.0.0.0
		}
		if mask := rm.Addrs[unix.RTAX_NETMASK]; mask != nil && !isZeroV4(mask) {
			continue // 0/1、128/1 这种拆分路由不是默认路由
		}
		gw := ""
		if a, ok := rm.Addrs[unix.RTAX_GATEWAY].(*route.Inet4Addr); ok {
			gw = net.IP(a.IP[:]).String()
		}
		ifi, err := net.InterfaceByIndex(rm.Index)
		if err != nil {
			continue
		}
		ip := firstIPv4(ifi)
		tunnel := isTunnelName(ifi.Name)
		scoped := rm.Flags&unix.RTF_IFSCOPE != 0
		fmt.Printf("  默认路由 %-8s 网关 %-15s 本机 %-15s ifscope=%-5v 隧道=%v\n", ifi.Name, gw, ip, scoped, tunnel)
		if tunnel || ip == nil || gw == "" {
			continue
		}
		// 路由表按优先级排，第一条非隧道的就是系统本来会走的；同时有带 ifscope 和不带的，用不带的
		if best == nil || (!scoped && best.Gateway == "") {
			best = &outIface{Name: ifi.Name, Index: ifi.Index, IP: ip, Gateway: gw}
		}
	}
	if best == nil {
		return outIface{}, errors.New("路由表里没有非隧道的默认路由")
	}
	return *best, nil
}

func isZeroV4(a route.Addr) bool {
	switch v := a.(type) {
	case *route.Inet4Addr:
		return v.IP == [4]byte{}
	case nil:
		return true
	}
	return false
}

func isTunnelName(name string) bool {
	for _, p := range []string{"utun", "ipsec", "ppp", "tun", "tap", "gif", "stf", "bridge", "lo"} {
		if strings.HasPrefix(name, p) {
			return true
		}
	}
	return false
}

func firstIPv4(ifi *net.Interface) net.IP {
	addrs, _ := ifi.Addrs()
	for _, a := range addrs {
		if n, ok := a.(*net.IPNet); ok {
			if ip := n.IP.To4(); ip != nil && !ip.IsLoopback() && !ip.IsLinkLocalUnicast() {
				return ip
			}
		}
	}
	return nil
}

// bindDevice IP_BOUND_IF：把 IPv4 套接字钉在某张网卡上
func bindDevice(fd uintptr, ifc outIface) error {
	return unix.SetsockoptInt(int(fd), unix.IPPROTO_IP, unix.IP_BOUND_IF, ifc.Index)
}

// ===================== 路由跳数 =====================

func traceProbes(ifc outIface) {
	for _, m := range []bindMode{modeNone, modeDev} {
		fmt.Printf("  [ICMP datagram socket（不用 root），%s]\n", m.name)
		hops, err := traceICMPDgram(ifc, m)
		printHops(hops, err)
	}
	fmt.Println("  [系统 traceroute -s]")
	runCmd(12, "traceroute", "-n", "-q", "1", "-w", "1", "-m", strconv.Itoa(maxTTL), "-s", ifc.IP.String(), traceTarget)
	fmt.Println("  [系统 traceroute 不加参数]")
	runCmd(12, "traceroute", "-n", "-q", "1", "-w", "1", "-m", strconv.Itoa(maxTTL), traceTarget)
}

// traceICMPDgram macOS 的 SOCK_DGRAM + IPPROTO_ICMP 不用 root；看它能不能收到中间路由的 TTL 超时
func traceICMPDgram(ifc outIface, m bindMode) ([]hop, error) {
	fd, err := unix.Socket(unix.AF_INET, unix.SOCK_DGRAM, unix.IPPROTO_ICMP)
	if err != nil {
		return nil, fmt.Errorf("建 ICMP datagram socket 失败: %w", err)
	}
	defer unix.Close(fd)

	// 收到的包不带 IP 头，直接就是 ICMP 报文
	const ipStripHdr = 0x17
	_ = unix.SetsockoptInt(fd, unix.IPPROTO_IP, ipStripHdr, 1)
	if m.dev {
		if err := bindDevice(uintptr(fd), ifc); err != nil {
			fmt.Println("      ⚠ IP_BOUND_IF 失败:", err)
		}
	}
	if m.src {
		sa := &unix.SockaddrInet4{}
		copy(sa.Addr[:], ifc.IP)
		if err := unix.Bind(fd, sa); err != nil {
			return nil, fmt.Errorf("bind: %w", err)
		}
	}
	_ = unix.SetsockoptTimeval(fd, unix.SOL_SOCKET, unix.SO_RCVTIMEO, &unix.Timeval{Sec: 1})

	dst := &unix.SockaddrInet4{}
	copy(dst.Addr[:], net.ParseIP(traceTarget).To4())
	id := os.Getpid() & 0xffff

	var hops []hop
	for ttl := 1; ttl <= maxTTL; ttl++ {
		_ = unix.SetsockoptInt(fd, unix.IPPROTO_IP, unix.IP_TTL, ttl)
		msg := icmp.Message{Type: ipv4.ICMPTypeEcho, Body: &icmp.Echo{ID: id, Seq: ttl, Data: []byte("linkstar")}}
		b, _ := msg.Marshal(nil)
		if err := unix.Sendto(fd, b, 0, dst); err != nil {
			return hops, fmt.Errorf("第 %d 跳发送失败: %w", ttl, err)
		}

		h := hop{ttl: ttl}
		deadline := time.Now().Add(time.Second)
		buf := make([]byte, 1500)
		for time.Now().Before(deadline) {
			n, from, err := unix.Recvfrom(fd, buf, 0)
			if err != nil {
				break
			}
			rm, err := icmp.ParseMessage(1, buf[:n])
			if err != nil {
				continue
			}
			f, ok := from.(*unix.SockaddrInet4)
			if !ok {
				continue
			}
			src := net.IP(f.Addr[:]).String()
			if rm.Type == ipv4.ICMPTypeTimeExceeded {
				h.ip = src
				break
			}
			if rm.Type == ipv4.ICMPTypeEchoReply {
				h.ip, h.reached = src, true
				break
			}
		}
		hops = append(hops, h)
		if h.done() {
			break
		}
	}
	return hops, nil
}
