//go:build linux || darwin

// tunprobe：实测开着代理 TUN 时，STUN 和路由探测怎样才能从物理网卡出去。
//
// 给 modules/stun 改造前做验证用的一次性工具，结论拿到就可以删。
// 用法：开着 TUN 跑一次，关掉 TUN 再跑一次，两次输出都贴回来。
package main

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/pion/stun/v3"
)

var stunServers = []string{"stun.hot-chilli.net:3478", "stun.sip.us:3478", "stun.nextcloud.com:3478"}

const (
	traceTarget = "114.114.114.114"
	maxTTL      = 6
)

// outIface 挑出来的物理出口
type outIface struct {
	Name    string
	Index   int
	IP      net.IP
	Gateway string
}

type bindMode struct {
	name     string
	src, dev bool
}

var (
	modeNone = bindMode{"不绑定", false, false}
	modeSrc  = bindMode{"只绑源地址", true, false}
	modeDev  = bindMode{"源地址+绑网卡", true, true}
)

type hop struct {
	ttl     int
	ip      string
	reached bool
}

func main() {
	fmt.Printf("tunprobe  %s/%s  uid=%d\n", runtime.GOOS, runtime.GOARCH, os.Getuid())

	section("1. 路由环境")
	printRouteEnv()

	section("2. 出口网卡")
	ifc, err := detectOutboundIface()
	if err != nil {
		fmt.Println("  ✗", err)
		fmt.Println("  找不到物理出口，后面的绑定测试做不了")
		return
	}
	fmt.Printf("  → 选中 %s  index=%d  本机 %s  网关 %s\n", ifc.Name, ifc.Index, ifc.IP, ifc.Gateway)

	section("3. DNS")
	servers := resolveServers(ifc)
	if len(servers) == 0 {
		fmt.Println("  ✗ 一个 STUN 服务器都解析不出来")
		return
	}

	section("4. 查公网 IP")
	probePublicIP(servers, ifc)

	section("5. 路由跳数（目标 " + traceTarget + "）")
	traceProbes(ifc)
}

func section(title string) {
	fmt.Printf("\n===== %s =====\n", title)
}

// ===================== DNS =====================

// resolveServers 系统解析和「从物理网卡直连 223.5.5.5」各解析一次，看是不是 fake-ip
func resolveServers(ifc outIface) []string {
	direct := &net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, network, _ string) (net.Conn, error) {
			network = network[:3] + "4" // "udp"/"tcp" → "udp4"/"tcp4"，出口只绑了 IPv4
			d, _ := boundDialer(network, ifc, modeDev)
			return d.DialContext(ctx, network, "223.5.5.5:53")
		},
	}

	var out []string
	for _, s := range stunServers {
		host, port, _ := net.SplitHostPort(s)
		sys := lookup(net.DefaultResolver, host)
		dir := lookup(direct, host)
		fmt.Printf("  %-22s 系统解析 %-24s 直连解析 %s\n", host, sys, dir)

		ip := dir
		if net.ParseIP(ip) == nil {
			ip = sys
		}
		if net.ParseIP(ip) != nil {
			out = append(out, net.JoinHostPort(ip, port))
		}
	}
	return out
}

func lookup(r *net.Resolver, host string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	ips, err := r.LookupIP(ctx, "ip4", host)
	if err != nil || len(ips) == 0 {
		return "失败"
	}
	s := ips[0].String()
	if _, fake, _ := net.ParseCIDR("198.18.0.0/15"); fake.Contains(ips[0]) {
		s += "(fake-ip)"
	}
	return s
}

// ===================== 公网 IP =====================

func probePublicIP(servers []string, ifc outIface) {
	for _, network := range []string{"tcp4", "udp4"} {
		got := map[string]string{}
		for _, m := range []bindMode{modeNone, modeSrc, modeDev} {
			var lastErr error
			for _, srv := range servers {
				ip, local, devErr, err := stunQuery(network, srv, ifc, m)
				if err != nil {
					lastErr = err
					continue
				}
				got[m.name] = ip
				fmt.Printf("  %s  %-14s 公网 %-16s 本地 %-22s %s\n", strings.ToUpper(network[:3]), m.name, ip, local, srv)
				if devErr != nil {
					fmt.Printf("       ⚠ 绑网卡失败（实际只绑了源地址）: %v\n", devErr)
				}
				lastErr = nil
				break
			}
			if lastErr != nil {
				fmt.Printf("  %s  %-14s ✗ %v\n", strings.ToUpper(network[:3]), m.name, lastErr)
			}
		}

		none, src, dev := got[modeNone.name], got[modeSrc.name], got[modeDev.name]
		switch {
		case dev == "":
			fmt.Println("     结论：绑了网卡也查不到，看上面的报错")
		case none != dev:
			fmt.Println("     结论：默认出口被 TUN 接管了，不绑定拿到的是代理节点的 IP")
		default:
			fmt.Println("     结论：三种一样，这次默认出口没被接管（TUN 没开，或代理没接管这类流量）")
		}
		if dev != "" && none != dev {
			if src == dev {
				fmt.Println("     结论：只绑源地址就够")
			} else {
				fmt.Println("     结论：只绑源地址不够，必须绑网卡")
			}
		}
	}
}

// boundDialer 按模式配 Dialer；绑网卡的结果写进返回的 *error
func boundDialer(network string, ifc outIface, m bindMode) (*net.Dialer, *error) {
	devErr := new(error)
	d := &net.Dialer{Timeout: 3 * time.Second}
	if m.src {
		if strings.HasPrefix(network, "tcp") {
			d.LocalAddr = &net.TCPAddr{IP: ifc.IP}
		} else {
			d.LocalAddr = &net.UDPAddr{IP: ifc.IP}
		}
	}
	if m.dev {
		d.Control = func(_, _ string, c syscall.RawConn) error {
			return c.Control(func(fd uintptr) { *devErr = bindDevice(fd, ifc) })
		}
	}
	return d, devErr
}

func stunQuery(network, server string, ifc outIface, m bindMode) (mapped, local string, devErr, err error) {
	d, devErrP := boundDialer(network, ifc, m)
	conn, err := d.Dial(network, server)
	if err != nil {
		return "", "", nil, err
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(3 * time.Second))

	req := stun.MustBuild(stun.TransactionID, stun.BindingRequest)
	if _, err := conn.Write(req.Raw); err != nil {
		return "", "", nil, err
	}

	var raw []byte
	if strings.HasPrefix(network, "tcp") {
		header := make([]byte, 20)
		if _, err := io.ReadFull(conn, header); err != nil {
			return "", "", nil, err
		}
		raw = make([]byte, 20+int(binary.BigEndian.Uint16(header[2:4])))
		copy(raw, header)
		if _, err := io.ReadFull(conn, raw[20:]); err != nil {
			return "", "", nil, err
		}
	} else {
		buf := make([]byte, 1500)
		n, err := conn.Read(buf)
		if err != nil {
			return "", "", nil, err
		}
		raw = buf[:n]
	}

	resp := &stun.Message{Raw: raw}
	if err := resp.Decode(); err != nil {
		return "", "", nil, err
	}
	if resp.TransactionID != req.TransactionID {
		return "", "", nil, fmt.Errorf("事务 ID 不匹配")
	}
	var xor stun.XORMappedAddress
	if err := xor.GetFrom(resp); err != nil {
		return "", "", nil, err
	}
	return xor.IP.String(), conn.LocalAddr().String(), *devErrP, nil
}

// ===================== 路由跳数 =====================

func printHops(hops []hop, err error) {
	var parts []string
	for _, h := range hops {
		switch {
		case h.ip == "":
			parts = append(parts, fmt.Sprintf("%d *", h.ttl))
		case h.reached:
			parts = append(parts, fmt.Sprintf("%d %s(到达)", h.ttl, h.ip))
		default:
			parts = append(parts, fmt.Sprintf("%d %s(%s)", h.ttl, h.ip, classify(h.ip)))
		}
	}
	if len(parts) > 0 {
		fmt.Println("      " + strings.Join(parts, " → "))
	}
	if err != nil {
		fmt.Println("      ✗", err)
	}
}

// done 到头了，或者已经出了公网
func (h hop) done() bool {
	return h.reached || (h.ip != "" && classify(h.ip) == "公网")
}

func classify(ip string) string {
	p := net.ParseIP(ip)
	if _, cgn, _ := net.ParseCIDR("100.64.0.0/10"); cgn.Contains(p) {
		return "CGN"
	}
	if p.IsPrivate() {
		return "私网"
	}
	return "公网"
}

func icmpChecksum(b []byte) uint16 {
	var sum uint32
	for i := 0; i+1 < len(b); i += 2 {
		sum += uint32(b[i])<<8 | uint32(b[i+1])
	}
	if len(b)%2 == 1 {
		sum += uint32(b[len(b)-1]) << 8
	}
	for sum>>16 != 0 {
		sum = sum&0xffff + sum>>16
	}
	return ^uint16(sum)
}

// runCmd 跑一个系统命令，输出缩进打印，最多 maxLines 行
func runCmd(maxLines int, name string, args ...string) {
	if _, err := exec.LookPath(name); err != nil {
		fmt.Printf("      （没有 %s 命令）\n", name)
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, name, args...).CombinedOutput()
	lines := strings.Split(strings.TrimRight(string(out), "\n"), "\n")
	if len(lines) > maxLines {
		lines = append(lines[:maxLines], fmt.Sprintf("...（还有 %d 行）", len(lines)-maxLines))
	}
	for _, l := range lines {
		if l != "" {
			fmt.Println("      " + l)
		}
	}
	if err != nil {
		fmt.Println("      ✗", err)
	}
}
