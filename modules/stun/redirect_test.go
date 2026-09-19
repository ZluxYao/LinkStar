package stun

import (
	"testing"

	"linkstar/modules/stun/model"
)

// TestCleanupLandingRecordSkipsSharedDomain 一个域名底下还挂着别的服务时，那条解析记录不能删。
//
// 一台机器上的 NAS、PVE、Alist 常常共用一个域名。删掉其中一个服务就顺手把记录删了，
// 剩下几个的公网 IP 从此没人更新——当时一点错都不报，过几天家宽 IP 一变全都访问不了。
func TestCleanupLandingRecordSkipsSharedDomain(t *testing.T) {
	oldCfg := Runtime.Config
	oldReleaser := landingRecordReleaser
	t.Cleanup(func() {
		Runtime.Config = oldCfg
		landingRecordReleaser = oldReleaser
	})

	var released []string
	landingRecordReleaser = func(host string) (bool, error) {
		released = append(released, host)
		return true, nil
	}

	// 删完 NAS 之后的样子：同一个域名上 PVE 还在
	Runtime.Config = model.Config{Devices: []model.Device{{
		DeviceID: 1,
		Services: []model.Service{{ID: 2, Domain: "home.example.com"}},
	}}}

	CleanupLandingRecord("home.example.com")
	if len(released) != 0 {
		t.Fatalf("还有服务在用这个域名，记录却被删了：%v", released)
	}

	// 带末尾点、大小写不同，也得认出来是同一个域名
	CleanupLandingRecord("HOME.example.com.")
	if len(released) != 0 {
		t.Fatalf("同一个域名换个写法就没认出来：%v", released)
	}

	// 最后一个服务也删了，这条记录才该收回去
	Runtime.Config = model.Config{Devices: []model.Device{{DeviceID: 1}}}
	CleanupLandingRecord("home.example.com")
	if len(released) != 1 || released[0] != "home.example.com" {
		t.Fatalf("没人用了却没收回记录：%v", released)
	}

	// 服务没填域名：别拿空字符串去找记录
	released = nil
	CleanupLandingRecord("  ")
	if len(released) != 0 {
		t.Fatalf("域名为空时不该动任何记录：%v", released)
	}
}
