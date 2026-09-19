package stun

import (
	"errors"
	"testing"

	"linkstar/modules/ddns/dns"
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

// fakeRedirectSyncer 记下上层到底叫服务商干了哪几件事
type fakeRedirectSyncer struct {
	ruleErr    error
	removedRul []string
	removedRec []string
}

func (f *fakeRedirectSyncer) SyncRedirectRule(zoneDomain, ruleKey, ruleLabel, entryHost, targetURL string) (bool, string, error) {
	return false, "", nil
}

func (f *fakeRedirectSyncer) RemoveRedirectRule(zoneDomain, ruleKey string) error {
	if f.ruleErr != nil {
		return f.ruleErr
	}
	f.removedRul = append(f.removedRul, ruleKey)
	return nil
}

func (f *fakeRedirectSyncer) RemoveEntryRecord(zoneDomain, entryHost string) (bool, error) {
	f.removedRec = append(f.removedRec, entryHost)
	return true, nil
}

func (f *fakeRedirectSyncer) InspectEntryRecord(zoneDomain, entryHost string) (dns.EntryRecordState, error) {
	return dns.EntryRecordState{}, nil
}

// useFakeSyncer 把服务商换成假的，测完还原
func useFakeSyncer(t *testing.T) *fakeRedirectSyncer {
	t.Helper()
	old := redirectSyncerFactory
	t.Cleanup(func() { redirectSyncerFactory = old })
	f := &fakeRedirectSyncer{}
	redirectSyncerFactory = func(uint) (RedirectSyncer, error) { return f, nil }
	return f
}

func redirectCfg() model.RedirectConfig {
	return model.RedirectConfig{
		Enabled:    true,
		ProviderID: 1,
		EntryHost:  "nas.example.com",
		ZoneDomain: "example.com",
	}
}

// TestCleanupRedirectDropsEntryRecord 规则删掉之后，当初替入口域名建的那条占位记录也得收回去。
//
// 不收的话 DNS 里留一条指向 192.0.2.1 的已代理记录：规则早没了，
// 访问的人撞上的是 Cloudflare 的错误页，而 LinkStar 这边什么都不显示——
// 那个服务在它眼里已经不存在了。
func TestCleanupRedirectDropsEntryRecord(t *testing.T) {
	f := useFakeSyncer(t)

	cleanupRedirectNow(redirectCfg(), 1, 2, true)

	if len(f.removedRul) != 1 || f.removedRul[0] != "linkstar:1-2" {
		t.Fatalf("规则没删或者删错了：%v", f.removedRul)
	}
	if len(f.removedRec) != 1 || f.removedRec[0] != "nas.example.com" {
		t.Fatalf("入口域名那条记录没收回：%v", f.removedRec)
	}
}

// TestCleanupRedirectKeepsRecordWhenRuleRemains 规则没删掉，记录就不能删。
//
// 顺序反了的后果：记录没了、规则还在，那条规则从此一次都不会执行
// （请求进不了 Cloudflare），而规则列表里它看着完全正常。
func TestCleanupRedirectKeepsRecordWhenRuleRemains(t *testing.T) {
	f := useFakeSyncer(t)
	f.ruleErr = errors.New("打不通 Cloudflare")

	cleanupRedirectNow(redirectCfg(), 1, 2, true)

	if len(f.removedRec) != 0 {
		t.Fatalf("规则都没删掉，记录却删了：%v", f.removedRec)
	}
}

// TestCleanupRedirectKeepsSharedEntryRecord 别的服务还用着同一个入口域名时，那条记录不能删。
func TestCleanupRedirectKeepsSharedEntryRecord(t *testing.T) {
	f := useFakeSyncer(t)

	cleanupRedirectNow(redirectCfg(), 1, 2, false)

	if len(f.removedRul) != 1 {
		t.Fatalf("自己那条规则还是该删的：%v", f.removedRul)
	}
	if len(f.removedRec) != 0 {
		t.Fatalf("还有服务用着这个入口，记录却被删了：%v", f.removedRec)
	}
}

// TestEntryHostInUse 谁还占着这个入口域名。判断做早了（配置还没存盘）会永远返回 true，
// 那这条记录就永远清不掉；判断漏了，就是替还在用的服务把记录删了。
func TestEntryHostInUse(t *testing.T) {
	oldCfg := Runtime.Config
	t.Cleanup(func() { Runtime.Config = oldCfg })

	Runtime.Config = model.Config{Devices: []model.Device{{
		DeviceID: 1,
		Services: []model.Service{
			// 另一个服务，同一个入口域名，写法不一样
			{ID: 3, Redirect: model.RedirectConfig{Enabled: true, EntryHost: "NAS.example.com."}},
			// 关着重定向的：服务商那边本来就没有它的规则
			{ID: 4, Redirect: model.RedirectConfig{Enabled: false, EntryHost: "pve.example.com"}},
		},
	}}}

	if !entryHostInUse("nas.example.com") {
		t.Fatal("同一个入口域名换个大小写和末尾点就没认出来")
	}
	if entryHostInUse("pve.example.com") {
		t.Fatal("重定向关着的服务不该算占用")
	}
	if entryHostInUse("other.example.com") {
		t.Fatal("没人用的入口域名被当成占用了")
	}
	if entryHostInUse("  ") {
		t.Fatal("空入口域名不该算占用")
	}
}
