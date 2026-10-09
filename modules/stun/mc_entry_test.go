package stun

import (
	"errors"
	"linkstar/modules/ddns/dns"
	"linkstar/modules/stun/model"
	"strings"
	"testing"
)

// fakeSRV 记下最后一次写了什么
type fakeSRV struct {
	name, target string
	port         uint16
	err          error
}

func (f *fakeSRV) SyncSRVRecord(_, name, target string, port uint16) (bool, error) {
	if f.err != nil {
		return false, f.err
	}
	changed := f.port != port
	f.name, f.target, f.port = name, target, port
	return changed, nil
}

func (f *fakeSRV) RemoveSRVRecord(_, _ string) (bool, error) { return true, nil }

func useFakeSRV(t *testing.T, f *fakeSRV) {
	t.Helper()
	oldFactory, oldEnsurer := srvSyncerFactory, landingRecordEnsurer
	t.Cleanup(func() { srvSyncerFactory, landingRecordEnsurer = oldFactory, oldEnsurer })
	srvSyncerFactory = func(uint) (dns.SRVRecordProvider, error) { return f, nil }
	landingRecordEnsurer = func(uint, string, string) (bool, error) { return true, nil }
}

func TestSyncMCEntry(t *testing.T) {
	f := &fakeSRV{}
	useFakeSRV(t, f)
	cfg := model.MCEntryConfig{Enabled: true, ProviderID: 3, Host: " MC.example.com. "}

	msg, changed, err := SyncMCEntry(cfg, 18083)
	if err != nil || !changed {
		t.Fatalf("changed=%v err=%v", changed, err)
	}
	// 朋友填 mc.example.com，MC 去查的是 _minecraft._tcp.mc.example.com，指回域名本身
	if f.name != "_minecraft._tcp.MC.example.com" || f.target != "MC.example.com" || f.port != 18083 {
		t.Fatalf("写进去的不对: %+v", f)
	}
	if !strings.Contains(msg, "已加进 DDNS") {
		t.Errorf("域名自己的 A 记录补上了要说一声: %q", msg)
	}

	if _, changed, _ := SyncMCEntry(cfg, 18083); changed {
		t.Error("端口没变，不该算改了")
	}

	f.err = errors.New("boom")
	if _, _, err := SyncMCEntry(cfg, 20001); err == nil {
		t.Error("服务商报错要往上抛")
	}
}

func TestSyncMCEntryNotReady(t *testing.T) {
	useFakeSRV(t, &fakeSRV{})
	for name, c := range map[string]struct {
		cfg  model.MCEntryConfig
		port uint16
	}{
		"没开":    {model.MCEntryConfig{Host: "mc.example.com", ProviderID: 3}, 18083},
		"没填域名":  {model.MCEntryConfig{Enabled: true, ProviderID: 3}, 18083},
		"洞还没打通": {model.MCEntryConfig{Enabled: true, Host: "mc.example.com", ProviderID: 3}, 0},
	} {
		if _, _, err := SyncMCEntry(c.cfg, c.port); err == nil {
			t.Errorf("%s：应该直接拒掉", name)
		}
	}
}

// TestMCEntryChanged 保存时什么情况要把旧的 SRV 收回去
func TestMCEntryChanged(t *testing.T) {
	on := model.MCEntryConfig{Enabled: true, ProviderID: 3, Host: "mc.example.com"}
	cases := []struct {
		name string
		old  model.MCEntryConfig
		cur  model.MCEntryConfig
		want bool
	}{
		{"原来就没开", model.MCEntryConfig{Host: "mc.example.com"}, on, false},
		{"没变", on, on, false},
		{"大小写不算变", on, model.MCEntryConfig{Enabled: true, ProviderID: 3, Host: "MC.example.com."}, false},
		{"关掉了", on, model.MCEntryConfig{ProviderID: 3, Host: "mc.example.com"}, true},
		{"换了域名", on, model.MCEntryConfig{Enabled: true, ProviderID: 3, Host: "game.example.com"}, true},
		{"换了服务商", on, model.MCEntryConfig{Enabled: true, ProviderID: 1, Host: "mc.example.com"}, true},
	}
	for _, c := range cases {
		if got := MCEntryChanged(c.old, c.cur); got != c.want {
			t.Errorf("%s：got %v，want %v", c.name, got, c.want)
		}
	}
}

// TestDomainInUseCountsMCEntry 删一个服务时，MC 联机域名还被别的服务用着就不能删它的 A 记录
func TestDomainInUseCountsMCEntry(t *testing.T) {
	old := Runtime.Config
	t.Cleanup(func() { Runtime.Config = old })
	Runtime.Config = model.Config{Devices: []model.Device{{Services: []model.Service{
		{Name: "mc", MCEntry: model.MCEntryConfig{Enabled: true, Host: "mc.example.com"}},
		{Name: "关着的", MCEntry: model.MCEntryConfig{Host: "off.example.com"}},
	}}}}

	if !domainInUse("MC.example.com") {
		t.Error("mc.example.com 还有 MC 入口用着")
	}
	if domainInUse("off.example.com") {
		t.Error("MC 入口关着的不算在用")
	}
}

func TestShouldSyncMCEntry(t *testing.T) {
	e := &serviceEntry{}
	if !e.shouldSyncMCEntry(18083) {
		t.Fatal("第一次要同步")
	}
	if e.shouldSyncMCEntry(18083) {
		t.Fatal("在途的那次还没回来，不该再发")
	}
	e.recordMCEntry(18083, nil)
	if e.shouldSyncMCEntry(18083) {
		t.Fatal("端口没变、上次成功，不该再打 API")
	}
	if !e.shouldSyncMCEntry(20001) {
		t.Fatal("端口变了要同步")
	}
	e.recordMCEntry(20001, errors.New("403"))
	if e.shouldSyncMCEntry(20001) {
		t.Fatal("刚失败，还在冷却")
	}
}
