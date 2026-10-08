//go:build linux

package main

import (
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"time"
	"unsafe"

	"golang.org/x/sys/unix"
)

func printRouteEnv() {
	fmt.Println("  $ ip -4 route show table main")
	runCmd(15, "ip", "-4", "route", "show", "table", "main")
	fmt.Println("  $ ip rule")
	runCmd(20, "ip", "rule")
	fmt.Println("  $ ip -4 route get " + traceTarget + "   ← 不绑定时实际走哪张卡")
	runCmd(3, "ip", "-4", "route", "get", traceTarget)
}

// detectOutboundIface 读主路由表的默认路由，跳过 TUN/隧道，跃点数最小的那条
func detectOutboundIface() (outIface, error) {
	data, err := os.ReadFile("/proc/net/route")
	if err != nil {
		return outIface{}, err
	}
	var best outIface
	bestMetric := -1
	for _, line := range strings.Split(string(data), "\n")[1:] {
		f := strings.Fields(line)
		if len(f) < 8 || f[1] != "00000000" || f[7] != "00000000" {
			continue // 不是默认路由
		}
		name := f[0]
		metric, _ := strconv.Atoi(f[6])
		tunnel := isTunnel(name)
		ifi, _ := net.InterfaceByName(name)
		ip := firstIPv4(ifi)
		gw := procIPv4(f[2])
		fmt.Printf("  默认路由 %-12s 网关 %-15s metric %-5d 本机 %-15s 隧道=%v\n", name, gw, metric, ip, tunnel)
		if tunnel || ifi == nil || ip == nil {
			continue
		}
		if bestMetric < 0 || metric < bestMetric {
			best = outIface{Name: name, Index: ifi.Index, IP: ip, Gateway: gw}
			bestMetric = metric
		}
	}
	if bestMetric < 0 {
		return outIface{}, errors.New("主路由表里没有非隧道的默认路由")
	}
	return best, nil
}

func isTunnel(name string) bool {
	if _, err := os.Stat("/sys/class/net/" + name + "/tun_flags"); err == nil {
		return true
	}
	t, err := os.ReadFile("/sys/class/net/" + name + "/type")
	if err != nil {
		return false
	}
	switch strings.TrimSpace(string(t)) {
	case "65534", "768", "776", "778": // none(tun/wg)、ipip、sit、gre
		return true
	}
	return false
}

// procIPv4 /proc/net/route 里的地址按本机字节序打印
func procIPv4(h string) string {
	b, err := hex.DecodeString(h)
	if err != nil || len(b) != 4 {
		return ""
	}
	var x [2]byte
	binary.NativeEndian.PutUint16(x[:], 1)
	if x[0] == 1 { // 小端
		b[0], b[1], b[2], b[3] = b[3], b[2], b[1], b[0]
	}
	if ip := net.IP(b); !ip.IsUnspecified() {
		return ip.String()
	}
	return ""
}

func firstIPv4(ifi *net.Interface) net.IP {
	if ifi == nil {
		return nil
	}
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

func bindDevice(fd uintptr, ifc outIface) error {
	return unix.SetsockoptString(int(fd), unix.SOL_SOCKET, unix.SO_BINDTODEVICE, ifc.Name)
}

// ===================== 路由跳数 =====================

func traceProbes(ifc outIface) {
	for _, m := range []bindMode{modeNone, modeDev} {
		fmt.Printf("  [UDP + IP_RECVERR，不用 root，%s]\n", m.name)
		hops, err := traceRecvErr(ifc, m)
		printHops(hops, err)
	}

	fmt.Println("  [ICMP datagram socket（不用 root 的 ping），源地址+绑网卡]")
	if g, err := os.ReadFile("/proc/sys/net/ipv4/ping_group_range"); err == nil {
		fmt.Printf("      ping_group_range = %s", g)
	}
	hops, err := traceICMPDgram(ifc)
	printHops(hops, err)

	fmt.Println("  [系统 traceroute -s -i]")
	runCmd(12, "traceroute", "-n", "-q", "1", "-w", "1", "-m", strconv.Itoa(maxTTL), "-s", ifc.IP.String(), "-i", ifc.Name, traceTarget)
	fmt.Println("  [系统 tracepath（不能指定出口）]")
	runCmd(12, "tracepath", "-n", "-m", strconv.Itoa(maxTTL), traceTarget)
}

// traceRecvErr 每跳发一个 UDP 包，从错误队列里读路由器回的 TTL 超时
func traceRecvErr(ifc outIface, m bindMode) ([]hop, error) {
	fd, err := unix.Socket(unix.AF_INET, unix.SOCK_DGRAM|unix.SOCK_CLOEXEC, 0)
	if err != nil {
		return nil, err
	}
	defer unix.Close(fd)

	if m.dev {
		if err := bindDevice(uintptr(fd), ifc); err != nil {
			fmt.Println("      ⚠ SO_BINDTODEVICE 失败:", err)
		}
	}
	if m.src {
		sa := &unix.SockaddrInet4{}
		copy(sa.Addr[:], ifc.IP)
		if err := unix.Bind(fd, sa); err != nil {
			return nil, fmt.Errorf("bind: %w", err)
		}
	}
	if err := unix.SetsockoptInt(fd, unix.IPPROTO_IP, unix.IP_RECVERR, 1); err != nil {
		return nil, fmt.Errorf("IP_RECVERR: %w", err)
	}
	if err := unix.SetsockoptTimeval(fd, unix.SOL_SOCKET, unix.SO_RCVTIMEO, &unix.Timeval{Sec: 1}); err != nil {
		return nil, err
	}

	dst := &unix.SockaddrInet4{Port: 33434}
	copy(dst.Addr[:], net.ParseIP(traceTarget).To4())

	var hops []hop
	for ttl := 1; ttl <= maxTTL; ttl++ {
		if err := unix.SetsockoptInt(fd, unix.IPPROTO_IP, unix.IP_TTL, ttl); err != nil {
			return hops, err
		}
		dst.Port = 33434 + ttl
		if err := unix.Sendto(fd, []byte("linkstar"), 0, dst); err != nil {
			return hops, fmt.Errorf("第 %d 跳发送失败: %w", ttl, err)
		}
		h := hop{ttl: ttl}
		h.ip, h.reached = readErrQueue(fd)
		hops = append(hops, h)
		if h.done() {
			break
		}
	}
	return hops, nil
}

// readErrQueue 等错误队列来消息；超时返回空。
// 错误队列只会让 poll 报 POLLERR，所以用 poll 等，再带 MSG_ERRQUEUE 读。
func readErrQueue(fd int) (ip string, reached bool) {
	deadline := time.Now().Add(time.Second)
	for {
		left := time.Until(deadline)
		if left <= 0 {
			return "", false
		}
		pfd := []unix.PollFd{{Fd: int32(fd), Events: unix.POLLERR}}
		n, err := unix.Poll(pfd, int(left/time.Millisecond)+1)
		if err == unix.EINTR {
			continue
		}
		if err != nil || n == 0 {
			return "", false
		}

		buf := make([]byte, 512)
		oob := make([]byte, 512)
		_, oobn, _, _, err := unix.Recvmsg(fd, buf, oob, unix.MSG_ERRQUEUE)
		if err != nil {
			return "", false
		}
		msgs, err := unix.ParseSocketControlMessage(oob[:oobn])
		if err != nil {
			return "", false
		}
		for _, cm := range msgs {
			if cm.Header.Level != unix.IPPROTO_IP || cm.Header.Type != unix.IP_RECVERR {
				continue
			}
			if len(cm.Data) < int(unsafe.Sizeof(unix.SockExtendedErr{}))+8 {
				continue
			}
			ee := (*unix.SockExtendedErr)(unsafe.Pointer(&cm.Data[0]))
			if ee.Origin != unix.SO_EE_ORIGIN_ICMP {
				continue
			}
			// 紧跟在 sock_extended_err 后面的是回 ICMP 的那台机器的 sockaddr_in
			off := int(unsafe.Sizeof(*ee))
			from := net.IP(cm.Data[off+4 : off+8]).String()
			switch {
			case ee.Type == 11: // Time Exceeded：中间路由
				return from, false
			case ee.Type == 3 && ee.Code == 3: // Port Unreachable：到达目标
				return from, true
			default:
				return from, false
			}
		}
	}
}

// traceICMPDgram 用不需要 root 的 ICMP datagram socket 发 echo，每跳递增 TTL
func traceICMPDgram(ifc outIface) ([]hop, error) {
	fd, err := unix.Socket(unix.AF_INET, unix.SOCK_DGRAM|unix.SOCK_CLOEXEC, unix.IPPROTO_ICMP)
	if err != nil {
		return nil, fmt.Errorf("建 ICMP datagram socket 失败（多半是 ping_group_range 没放开）: %w", err)
	}
	defer unix.Close(fd)
	_ = bindDevice(uintptr(fd), ifc)
	sa := &unix.SockaddrInet4{}
	copy(sa.Addr[:], ifc.IP)
	if err := unix.Bind(fd, sa); err != nil {
		return nil, fmt.Errorf("bind: %w", err)
	}
	if err := unix.SetsockoptInt(fd, unix.IPPROTO_IP, unix.IP_RECVERR, 1); err != nil {
		return nil, err
	}

	dst := &unix.SockaddrInet4{}
	copy(dst.Addr[:], net.ParseIP(traceTarget).To4())

	var hops []hop
	for ttl := 1; ttl <= maxTTL; ttl++ {
		_ = unix.SetsockoptInt(fd, unix.IPPROTO_IP, unix.IP_TTL, ttl)
		pkt := []byte{8, 0, 0, 0, 0, 0, 0, byte(ttl)} // echo request，id 由内核填
		binary.BigEndian.PutUint16(pkt[2:], icmpChecksum(pkt))
		if err := unix.Sendto(fd, pkt, 0, dst); err != nil {
			return hops, fmt.Errorf("第 %d 跳发送失败: %w", ttl, err)
		}
		h := hop{ttl: ttl}
		// 中间路由的 TTL 超时进错误队列；到达目标的 echo reply 走正常接收
		h.ip, _ = readErrQueue(fd)
		if h.ip == "" {
			buf := make([]byte, 512)
			_ = unix.SetsockoptTimeval(fd, unix.SOL_SOCKET, unix.SO_RCVTIMEO, &unix.Timeval{Usec: 300000})
			if n, from, err := unix.Recvfrom(fd, buf, unix.MSG_DONTWAIT); err == nil && n > 0 {
				if f, ok := from.(*unix.SockaddrInet4); ok {
					h.ip, h.reached = net.IP(f.Addr[:]).String(), true
				}
			}
		}
		hops = append(hops, h)
		if h.done() {
			break
		}
	}
	return hops, nil
}
