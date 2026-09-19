package stun

import (
	"context"
	"net"
	"testing"
	"time"
)

// TestIsAliveErrRefused 「端口关着」必须能和「没这台机器」分开。
//
// 整个扫描就靠这一条判断：分不开的话，要么一台都扫不出来，
// 要么把 254 个地址全当成在线。而且不能靠错误信息里的英文单词——
// 中文 Windows 的报错是「由于目标计算机积极拒绝」，一个 refused 都没有。
func TestIsAliveErrRefused(t *testing.T) {
	// 先占一个端口再放掉，拿到一个确定没人听的端口号
	l, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Skipf("监听不了，跳过: %v", err)
	}
	addr := l.Addr().String()
	_ = l.Close()

	_, err = net.DialTimeout("tcp4", addr, time.Second)
	if err == nil {
		t.Fatalf("%s 上居然连上了，这个端口没空着，测不了", addr)
	}
	if !isAliveErr(err) {
		t.Fatalf("端口关着应该算「机器在」，却判成了不在: %#v", err)
	}
}

// TestIsAliveErrTimeout 超时是「没人应」，不能算在线
func TestIsAliveErrTimeout(t *testing.T) {
	// 192.0.2.0/24 是 RFC 5737 留给文档用的，路由不过去
	_, err := net.DialTimeout("tcp4", "192.0.2.1:9", 300*time.Millisecond)
	if err == nil {
		t.Skip("这个地址居然连得上，环境特殊，跳过")
	}
	if isAliveErr(err) {
		t.Fatalf("连不到的地址被判成了在线: %#v", err)
	}
}

// TestProbePortOpen 端口开着的时候两个返回值都得是真
func TestProbePortOpen(t *testing.T) {
	l, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Skipf("监听不了，跳过: %v", err)
	}
	defer l.Close()

	_, portStr, _ := net.SplitHostPort(l.Addr().String())
	var port uint16
	for _, c := range portStr {
		port = port*10 + uint16(c-'0')
	}

	open, alive := probePort(context.Background(), "127.0.0.1", port, time.Second)
	if !open || !alive {
		t.Fatalf("端口明明开着，却得到 open=%v alive=%v", open, alive)
	}
}

// TestSubnetIPs /24 掐掉网络号和广播地址，剩 254 个
func TestSubnetIPs(t *testing.T) {
	_, n, _ := net.ParseCIDR("192.168.100.0/24")
	got := subnetIPs(n)
	if len(got) != 254 {
		t.Fatalf("/24 应该是 254 个地址，得到 %d", len(got))
	}
	if got[0] != "192.168.100.1" {
		t.Fatalf("第一个该是 .1（.0 是网络号），得到 %s", got[0])
	}
	if got[253] != "192.168.100.254" {
		t.Fatalf("最后一个该是 .254（.255 是广播），得到 %s", got[253])
	}
}

// TestNarrowTo24 比 /24 大的网段得收窄，不然 10.0.0.0/8 要扫一千六百万个地址
func TestNarrowTo24(t *testing.T) {
	n := narrowTo24(net.ParseIP("10.3.4.5"), net.CIDRMask(8, 32))
	if n.String() != "10.3.4.0/24" {
		t.Fatalf("/8 该收窄成本机所在的 /24，得到 %s", n.String())
	}
	// 本来就比 /24 小的不能动，收窄反而会扫到隔壁网段
	n = narrowTo24(net.ParseIP("192.168.1.130"), net.CIDRMask(25, 32))
	if n.String() != "192.168.1.128/25" {
		t.Fatalf("/25 不该被改动，得到 %s", n.String())
	}
}

// TestScanLanRejectsPublic 只扫自己家的内网。
//
// 运营商给的 100.64/10 里住的是同片区其他宽带用户，公网地址就更不用说了，
// 扫它们既扫不出有用的东西，也不该是这个按钮干的事。
func TestScanLanRejectsPublic(t *testing.T) {
	for _, cidr := range []string{"1.1.1.0/24", "100.72.0.0/24"} {
		if _, err := ScanLan(context.Background(), cidr); err == nil {
			t.Fatalf("%s 不该扫得动", cidr)
		}
	}
}

// TestScanLanRejectsHuge 网段太大要当场拦下来，不能让界面转几分钟圈
func TestScanLanRejectsHuge(t *testing.T) {
	if _, err := ScanLan(context.Background(), "10.0.0.0/16"); err == nil {
		t.Fatal("/16 有 65534 个地址，不该放过去")
	}
}

// TestIPLess 按数值排，不按字符串排
func TestIPLess(t *testing.T) {
	if !ipLess("192.168.1.9", "192.168.1.10") {
		t.Fatal(".9 应该排在 .10 前面，按字符串排就反了")
	}
}
