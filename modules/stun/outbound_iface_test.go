package stun

import (
	"linkstar/modules/stun/model"
	"net"
	"testing"
)

// TestCollectHops 路由层级只记私网和 CGN，碰到 CGN 或公网就停；
// 某一跳不回包时重问一次，还不回就跳过这一跳接着往下。
func TestCollectHops(t *testing.T) {
	cases := []struct {
		name    string
		replies map[int][]string // ttl → 每次探测的回包地址，"" 表示超时
		want    []string
	}{
		{
			name: "家里两层路由再到运营商 CGN",
			replies: map[int][]string{
				1: {"192.168.100.1"}, 2: {"192.168.31.1"}, 3: {"100.72.0.1"}, 4: {"203.0.113.1"},
			},
			want: []string{"192.168.100.1", "192.168.31.1", "100.72.0.1"},
		},
		{
			name: "光猫桥接，第二跳就是公网",
			replies: map[int][]string{
				1: {"192.168.1.1"}, 2: {"203.0.113.1"},
			},
			want: []string{"192.168.1.1"},
		},
		{
			name: "CGN 第一次没回，重问一次回了",
			replies: map[int][]string{
				1: {"192.168.100.1"}, 2: {"", "100.72.0.1"},
			},
			want: []string{"192.168.100.1", "100.72.0.1"},
		},
		{
			name: "中间一跳一直不回，跳过它接着往下",
			replies: map[int][]string{
				1: {"192.168.100.1"}, 2: {"", ""}, 3: {"100.72.0.1"},
			},
			want: []string{"192.168.100.1", "100.72.0.1"},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			asked := map[int]int{}
			probe := func(ttl int) (string, bool, error) {
				replies := c.replies[ttl]
				i := asked[ttl]
				asked[ttl]++
				if i >= len(replies) {
					return "", false, nil
				}
				return replies[i], false, nil
			}

			hops, err := collectHops("114.114.114.114", 6, probe)
			if err != nil {
				t.Fatal(err)
			}
			var got []string
			for i, h := range hops {
				if h.NatLevel != uint(i+1) {
					t.Errorf("第 %d 个的 NatLevel = %d，层级要连续", i, h.NatLevel)
				}
				got = append(got, h.LanIp)
			}
			if len(got) != len(c.want) {
				t.Fatalf("got %v，want %v", got, c.want)
			}
			for i := range got {
				if got[i] != c.want[i] {
					t.Fatalf("got %v，want %v", got, c.want)
				}
			}
		})
	}
}

// TestOutboundIfaceSubnet UPnP 按出口网卡的真实网段挑网关，不再按「前三段相同」。
func TestOutboundIfaceSubnet(t *testing.T) {
	cases := []struct {
		name    string
		iface   OutboundIface
		gateway string
		want    bool
	}{
		{"/24 同网段", OutboundIface{LocalIP: "192.168.100.187", PrefixLen: 24}, "192.168.100.1", true},
		{"/16 时前三段不同也是同网段", OutboundIface{LocalIP: "10.0.5.20", PrefixLen: 16}, "10.0.0.1", true},
		{"/24 时第三段不同就不是", OutboundIface{LocalIP: "192.168.100.187", PrefixLen: 24}, "192.168.31.1", false},
		{"不知道前缀按 /24", OutboundIface{LocalIP: "192.168.1.5"}, "192.168.1.1", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			subnet := c.iface.Subnet()
			if subnet == nil {
				t.Fatal("Subnet() = nil")
			}
			if got := subnet.Contains(net.ParseIP(c.gateway)); got != c.want {
				t.Fatalf("%s 在不在 %s：got %v，want %v", c.gateway, subnet, got, c.want)
			}
		})
	}

	if (OutboundIface{}).Subnet() != nil {
		t.Error("没有出口时 Subnet() 应为 nil")
	}
}

// TestIsTunnelName 读不到网卡类型时按名字兜底：代理 / 组网 / 虚拟机的网卡要认出来，
// 物理网卡不能误伤。
func TestIsTunnelName(t *testing.T) {
	tunnels := []string{"utun4", "tun0", "Meta", "et_10_1ymx", "wg0", "tailscale0", "docker0", "vmnet8", "br-1a2b", "lo"}
	for _, name := range tunnels {
		if !isTunnelName(name) {
			t.Errorf("isTunnelName(%q) = false，应该认成隧道/虚拟网卡", name)
		}
	}
	physical := []string{"en0", "eth0", "enp3s0", "wlan0", "以太网", "WLAN", "ppp0"}
	for _, name := range physical {
		if isTunnelName(name) {
			t.Errorf("isTunnelName(%q) = true，这是物理网卡或拨号", name)
		}
	}
}

// TestPickAutoIface 自动模式：候选里跃点数最小的；隧道、没网关的虚拟网卡都不选
func TestPickAutoIface(t *testing.T) {
	list := []OutboundIface{
		{Name: "Meta", LocalIP: "198.18.0.1", Tunnel: true},
		{Name: "VMware Network Adapter VMnet8", LocalIP: "192.168.80.1"},
		{Name: "WLAN", LocalIP: "192.168.100.120", Gateway: "192.168.100.1", candidate: true, metric: 35},
		{Name: "以太网", LocalIP: "192.168.100.187", Gateway: "192.168.100.1", candidate: true, metric: 20},
	}
	got, err := pickAutoIface(list)
	if err != nil || got.Name != "以太网" {
		t.Fatalf("pickAutoIface = %v, %v，想要以太网（跃点 20 < 35）", got, err)
	}

	if _, err := pickAutoIface(list[:2]); err == nil {
		t.Fatal("只有隧道和虚拟网卡时应该报没有出口")
	}
}

// TestDetectOutboundIfaceCustom 指定的网卡不在时报错，不偷偷换成别的
func TestDetectOutboundIfaceCustom(t *testing.T) {
	old := networkConfig.Load()
	t.Cleanup(func() { networkConfig.Store(old) })

	networkConfig.Store(&model.NetworkConfig{IfaceMode: model.ModeCustom, Iface: "一张不存在的网卡"})
	if o, err := DetectOutboundIface(); err == nil {
		t.Fatalf("指定的网卡不存在，却挑出了 %v", o)
	}
}

// TestDNSServers 三种 DNS 模式各自用谁
func TestDNSServers(t *testing.T) {
	old := networkConfig.Load()
	t.Cleanup(func() { networkConfig.Store(old) })

	cases := []struct {
		name string
		cfg  model.NetworkConfig
		want []string
	}{
		{"老配置没这一段，用内置", model.NetworkConfig{}, DefaultDNSServers},
		{"跟随系统", model.NetworkConfig{DNSMode: model.ModeSystem}, nil},
		{"自定义", model.NetworkConfig{DNSMode: model.ModeCustom, DNS: []string{"192.168.100.1"}}, []string{"192.168.100.1"}},
		{"自定义却一个没填，退回内置", model.NetworkConfig{DNSMode: model.ModeCustom}, DefaultDNSServers},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			networkConfig.Store(&c.cfg)
			got := dnsServers()
			if len(got) != len(c.want) || (got == nil) != (c.want == nil) {
				t.Fatalf("got %v，want %v", got, c.want)
			}
			for i := range got {
				if got[i] != c.want[i] {
					t.Fatalf("got %v，want %v", got, c.want)
				}
			}
		})
	}
}

// TestNormalizeDNSServer 用户填的 DNS 地址只认 IPv4，可以带端口
func TestNormalizeDNSServer(t *testing.T) {
	ok := map[string]string{
		"114.114.114.114":    "114.114.114.114",
		" 192.168.100.1 ":    "192.168.100.1",
		"192.168.100.1:5353": "192.168.100.1:5353",
	}
	for in, want := range ok {
		if got, err := NormalizeDNSServer(in); err != nil || got != want {
			t.Errorf("NormalizeDNSServer(%q) = %q, %v，want %q", in, got, err, want)
		}
	}
	for _, in := range []string{"dns.example.com", "2001:db8::1", "192.168.100.1:0", "192.168.100.1:70000", ""} {
		if got, err := NormalizeDNSServer(in); err == nil {
			t.Errorf("NormalizeDNSServer(%q) = %q，应该报错", in, got)
		}
	}
}

// TestWithDNSPort 没写端口补 53
func TestWithDNSPort(t *testing.T) {
	if got := withDNSPort("114.114.114.114"); got != "114.114.114.114:53" {
		t.Errorf("got %q", got)
	}
	if got := withDNSPort("192.168.100.1:5353"); got != "192.168.100.1:5353" {
		t.Errorf("got %q", got)
	}
}
