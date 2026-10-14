package stun

import (
	"context"
	"net"
	"sync/atomic"
	"testing"
	"time"

	"github.com/pion/stun"

	"linkstar/modules/stun/model"
)

func TestReview内置DNS失败时只问桩解析器(t *testing.T) {
	blackhole := "127.0.0.1:1"
	var calls atomic.Int32
	restore := useLoopbackResolver(t, &calls, net.ParseIP("203.0.113.10"))
	defer restore()

	oldTimeout := dnsLookupTimeout
	oldServers := append([]string(nil), DefaultDNSServers...)
	oldCfg := networkConfig.Load()
	dnsLookupTimeout = 200 * time.Millisecond
	DefaultDNSServers = []string{blackhole}
	t.Cleanup(func() {
		dnsLookupTimeout = oldTimeout
		DefaultDNSServers = oldServers
		networkConfig.Store(oldCfg)
	})
	networkConfig.Store(&model.NetworkConfig{})

	o := OutboundIface{Name: "lo", LocalIP: "127.0.0.1"}
	got, err := o.resolve("stun.example.com:3478")
	if err != nil {
		t.Fatalf("内置 DNS 失败后应改用桩解析器: %v", err)
	}
	host, port, err := net.SplitHostPort(got)
	if err != nil || port != "3478" || host != "203.0.113.10" {
		t.Fatalf("解析结果 = %q，想要 203.0.113.10:3478", got)
	}
	if calls.Load() != 1 {
		t.Fatalf("系统解析器被调用 %d 次，想要 1", calls.Load())
	}
}

func TestReview自定义DNS即使与内置列表相同也不走系统解析器(t *testing.T) {
	blackhole := "127.0.0.1:1"
	var calls atomic.Int32
	restore := useLoopbackResolver(t, &calls, net.ParseIP("203.0.113.10"))
	defer restore()

	oldTimeout := dnsLookupTimeout
	oldServers := append([]string(nil), DefaultDNSServers...)
	oldCfg := networkConfig.Load()
	dnsLookupTimeout = 200 * time.Millisecond
	DefaultDNSServers = []string{blackhole}
	t.Cleanup(func() {
		dnsLookupTimeout = oldTimeout
		DefaultDNSServers = oldServers
		networkConfig.Store(oldCfg)
	})
	// 填的地址和内置列表一模一样，但模式是自定义，失败后不能问系统解析器。
	networkConfig.Store(&model.NetworkConfig{
		DNSMode: model.ModeCustom,
		DNS:     []string{blackhole},
	})

	o := OutboundIface{Name: "lo", LocalIP: "127.0.0.1"}
	if _, err := o.resolve("stun.example.com:3478"); err == nil {
		t.Fatal("自定义 DNS 失败时不应改用系统解析器")
	}
	if calls.Load() != 0 {
		t.Fatalf("系统解析器被调用 %d 次，自定义模式应为 0", calls.Load())
	}
}

func TestReviewTCPBinding失败时改用UDP(t *testing.T) {
	addr := startTCPBlackholeUDPSTUN(t, net.ParseIP("203.0.113.50"))

	old := stunDialTimeout
	oldOutbound := currentOutboundIface.Load()
	oldCfg := networkConfig.Load()
	stunDialTimeout = 200 * time.Millisecond
	currentOutboundIface.Store(&OutboundIface{Name: "lo", LocalIP: "127.0.0.1"})
	networkConfig.Store(&model.NetworkConfig{DNSMode: model.ModeSystem})
	t.Cleanup(func() {
		stunDialTimeout = old
		currentOutboundIface.Store(oldOutbound)
		networkConfig.Store(oldCfg)
	})

	got, err := getSTUNServerDelay(addr)
	if err != nil {
		t.Fatalf("TCP 不回答 Binding 时应改用同端口 UDP: %v", err)
	}
	if got.Server != addr {
		t.Fatalf("选中的服务器 = %q，想要 %q", got.Server, addr)
	}
}

func TestReviewTCP响应分片仍能选中服务器(t *testing.T) {
	mapped := net.ParseIP("203.0.113.60")
	addr := startSplitTCPSTUN(t, mapped)

	old := stunDialTimeout
	oldOutbound := currentOutboundIface.Load()
	oldCfg := networkConfig.Load()
	stunDialTimeout = time.Second
	currentOutboundIface.Store(&OutboundIface{Name: "lo", LocalIP: "127.0.0.1"})
	networkConfig.Store(&model.NetworkConfig{DNSMode: model.ModeSystem})
	t.Cleanup(func() {
		stunDialTimeout = old
		currentOutboundIface.Store(oldOutbound)
		networkConfig.Store(oldCfg)
	})

	ip, err := dialSTUNBinding(addr)
	if err != nil {
		t.Fatalf("TCP 把 Binding 响应拆成两段时仍应读完整帧: %v", err)
	}
	if !ip.Equal(mapped) {
		t.Fatalf("映射地址 = %s，想要 %s", ip, mapped)
	}
	got, err := getSTUNServerDelay(addr)
	if err != nil {
		t.Fatalf("分片的 TCP STUN 服务器应被选中: %v", err)
	}
	if got.Server != addr {
		t.Fatalf("选中的服务器 = %q，想要 %q", got.Server, addr)
	}
}

func TestReview重试间隔不超过五分钟(t *testing.T) {
	if got := nextSTUNRetryWait(15 * time.Second); got != 30*time.Second {
		t.Fatalf("15s 翻倍 = %s，想要 30s", got)
	}
	if got := nextSTUNRetryWait(240 * time.Second); got != stunRetryMax {
		t.Fatalf("240s 翻倍 = %s，想要 %s", got, stunRetryMax)
	}
	if got := nextSTUNRetryWait(stunRetryMax); got != stunRetryMax {
		t.Fatalf("已经到上限后再等 = %s，想要 %s", got, stunRetryMax)
	}
}

// useLoopbackResolver 把系统解析器换成只打本机假 DNS 的桩，并统计被调用次数。
func useLoopbackResolver(t *testing.T, calls *atomic.Int32, answer net.IP) func() {
	t.Helper()
	fake := serveLoopbackDNS(t, answer)
	old := systemResolver
	systemResolver = &net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
			calls.Add(1)
			var d net.Dialer
			return d.DialContext(ctx, "udp", fake)
		},
	}
	return func() { systemResolver = old }
}

func serveLoopbackDNS(t *testing.T, ip net.IP) string {
	t.Helper()
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { pc.Close() })
	go func() {
		buf := make([]byte, 512)
		for {
			n, addr, err := pc.ReadFrom(buf)
			if err != nil {
				return
			}
			resp := dnsAResponse(buf[:n], ip)
			if resp == nil {
				continue
			}
			_, _ = pc.WriteTo(resp, addr)
		}
	}()
	return pc.LocalAddr().String()
}

func dnsAResponse(query []byte, ip net.IP) []byte {
	ip4 := ip.To4()
	if len(query) < 12 || ip4 == nil {
		return nil
	}
	end := 12
	for end < len(query) {
		n := int(query[end])
		if n == 0 {
			end++
			break
		}
		end += 1 + n
		if end > len(query) {
			return nil
		}
	}
	end += 4
	if end > len(query) {
		return nil
	}
	resp := make([]byte, 12, end+16)
	copy(resp, query[:12])
	resp[2] = 0x81
	resp[3] = 0x80
	resp[6], resp[7] = 0, 1
	resp = append(resp, query[12:end]...)
	resp = append(resp, 0xc0, 0x0c, 0, 1, 0, 1, 0, 0, 0, 60, 0, 4)
	return append(resp, ip4...)
}

// startTCPBlackholeUDPSTUN 同一端口：TCP 接受连接但不回答，UDP 回一条 Binding 成功。
func startTCPBlackholeUDPSTUN(t *testing.T, mapped net.IP) string {
	t.Helper()
	tcpLn, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := tcpLn.Addr().(*net.TCPAddr).Port
	udpLn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: port})
	if err != nil {
		tcpLn.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() {
		tcpLn.Close()
		udpLn.Close()
	})
	go func() {
		for {
			c, err := tcpLn.Accept()
			if err != nil {
				return
			}
			go func(conn net.Conn) {
				defer conn.Close()
				buf := make([]byte, 64)
				_, _ = conn.Read(buf) // 读走请求，但不写响应
			}(c)
		}
	}()
	go func() {
		buf := make([]byte, 1500)
		for {
			n, addr, err := udpLn.ReadFromUDP(buf)
			if err != nil {
				return
			}
			res := stun.MustBuild(
				stun.BindingSuccess,
				&stun.XORMappedAddress{IP: mapped, Port: 3478},
			)
			_ = n
			_, _ = udpLn.WriteToUDP(res.Raw, addr)
		}
	}()
	return tcpLn.Addr().String()
}

// startSplitTCPSTUN 只开 TCP：先写出合法响应的前 12 字节，停一下再写剩余部分。
// 没有 UDP 监听，读不完整帧就不能靠 UDP 把这台服务器救回来。
func startSplitTCPSTUN(t *testing.T, mapped net.IP) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go writeSplitSTUNResponse(c, mapped)
		}
	}()
	return ln.Addr().String()
}

func writeSplitSTUNResponse(conn net.Conn, mapped net.IP) {
	defer conn.Close()
	if tc, ok := conn.(*net.TCPConn); ok {
		_ = tc.SetNoDelay(true)
	}
	_ = conn.SetDeadline(time.Now().Add(2 * time.Second))
	buf := make([]byte, 128)
	if _, err := conn.Read(buf); err != nil {
		return
	}
	res := stun.MustBuild(
		stun.BindingSuccess,
		&stun.XORMappedAddress{IP: mapped, Port: 3478},
	)
	raw := res.Raw
	if len(raw) <= 12 {
		return
	}
	if _, err := conn.Write(raw[:12]); err != nil {
		return
	}
	// 留出时间让对端的第一次 Read 只拿到头的前半段，再把剩下的帧送出去。
	time.Sleep(80 * time.Millisecond)
	_, _ = conn.Write(raw[12:])
}
