package stun

import (
	"strings"
	"testing"
	"time"

	"linkstar/modules/stun/model"
)

func Test不是公网地址会被拒绝(t *testing.T) {
	cases := []struct {
		ip   string
		want bool
	}{
		{"203.0.113.10", false},
		{"", true},
		{"not-an-ip", true},
		{"10.1.2.3", true},
		{"192.168.1.1", true},
		{"172.16.0.1", true},
		{"100.64.0.1", true},
		{"127.0.0.1", true},
		{"169.254.1.1", true},
	}
	for _, c := range cases {
		err := checkPublicIP(c.ip)
		if (err != nil) != c.want {
			t.Errorf("checkPublicIP(%q) err=%v，想要拒绝=%v", c.ip, err, c.want)
		}
	}
}

func Test探测失败不会清空已有公网IP(t *testing.T) {
	oldCfg := networkConfig.Load()
	oldPublic := Runtime.Network.PublicIP
	oldLocal := Runtime.Network.LocalIP
	t.Cleanup(func() {
		networkConfig.Store(oldCfg)
		Runtime.Network.PublicIP = oldPublic
		Runtime.Network.LocalIP = oldLocal
	})
	networkConfig.Store(&model.NetworkConfig{DNSMode: model.ModeSystem})
	Runtime.Network.PublicIP = "203.0.113.10"
	Runtime.Network.LocalIP = "192.0.2.10"

	info, err := GetPublicIPInfo("127.0.0.1:1")
	if err == nil {
		t.Fatalf("连不上 STUN 服务器时应返回错误，实际 public=%q", info.PublicIP)
	}
	if strings.Contains(err.Error(), "获取真实公网ip失败") {
		t.Fatalf("错误不应再走 stdout 的旧文案: %v", err)
	}

	start := time.Now()
	if updateNetworkAddress("127.0.0.1:1") {
		t.Fatal("失败的探测不应返回成功")
	}
	if time.Since(start) > 8*time.Second {
		t.Fatalf("失败探测耗时 %s，拨号超时不应叠成两轮很长的等待", time.Since(start))
	}
	if Runtime.Network.PublicIP != "203.0.113.10" {
		t.Fatalf("已有公网 IP 被改成 %q", Runtime.Network.PublicIP)
	}
}
