package stun

import (
	"errors"
	"testing"
	"time"
)

// TestMarkRedirectStaleOnPortChange 端口一变，界面上那行就不该再是绿色的「成功」。
//
// 这是用户实际遇到的那一幕：洞重开拿到 25888，入口重定向底下却写着
// 「成功 → https://zlux.top:26853」。看着一切正常，可 Cloudflare 那边真就还是旧端口，
// 此时从入口域名进来是打不开的——最难查的就是这种「显示成功、实际断着」。
func TestMarkRedirectStaleOnPortChange(t *testing.T) {
	e := newServiceEntry(func() {}, 1, 2, "NAS", "DSM")
	e.recordRedirect("https://zlux.top:26853", true, nil)

	if got := e.snapshot("1-2", EventPhaseChanged).RedirectStatus; got != "ok" {
		t.Fatalf("同步成功后状态应为 ok，实际 %q", got)
	}

	if !e.markRedirectStale(25888) {
		t.Fatal("端口从 26853 换到 25888，应该标成 stale")
	}

	snap := e.snapshot("1-2", EventPhaseChanged)
	if snap.RedirectStatus != "stale" {
		t.Fatalf("状态应为 stale，实际 %q", snap.RedirectStatus)
	}
	// 旧地址要留着：那是服务商那边此刻真正的样子，界面得把它摆出来
	if snap.RedirectTarget != "https://zlux.top:26853" {
		t.Fatalf("旧地址应原样保留，实际 %q", snap.RedirectTarget)
	}
}

// TestMarkRedirectStaleIgnoresSamePort 端口没变就什么都别动，
// 免得洞每次重开都白白闪一下黄色。
func TestMarkRedirectStaleIgnoresSamePort(t *testing.T) {
	e := newServiceEntry(func() {}, 1, 2, "NAS", "DSM")
	e.recordRedirect("https://zlux.top:26853", true, nil)

	if e.markRedirectStale(26853) {
		t.Fatal("端口没变不该改状态")
	}
	if got := e.snapshot("1-2", EventPhaseChanged).RedirectStatus; got != "ok" {
		t.Fatalf("状态应保持 ok，实际 %q", got)
	}
}

// TestMarkRedirectStaleSkipsNeverSynced 没开重定向、或者还没同步过的服务，
// 没有「旧的」可言，别凭空给它一个黄色警告。
func TestMarkRedirectStaleSkipsNeverSynced(t *testing.T) {
	e := newServiceEntry(func() {}, 1, 2, "NAS", "DSM")

	if e.markRedirectStale(25888) {
		t.Fatal("从没同步过不该标 stale")
	}
	if got := e.snapshot("1-2", EventPhaseChanged).RedirectStatus; got != "" {
		t.Fatalf("状态应保持空，实际 %q", got)
	}
}

// TestShouldSyncRedirectKeepsTargetUntilWritten 认领了一个新目标之后，
// 对外显示的地址必须还是旧的那个——写还没发出去，界面不能先报成功。
func TestShouldSyncRedirectKeepsTargetUntilWritten(t *testing.T) {
	e := newServiceEntry(func() {}, 1, 2, "NAS", "DSM")
	e.recordRedirect("https://zlux.top:26853", true, nil)

	if !e.shouldSyncRedirect("https://zlux.top:25888") {
		t.Fatal("目标变了应该去同步")
	}

	snap := e.snapshot("1-2", EventPhaseChanged)
	if snap.RedirectTarget != "https://zlux.top:26853" {
		t.Fatalf("写完之前显示的应是旧地址，实际 %q", snap.RedirectTarget)
	}
	if snap.RedirectStatus != "stale" {
		t.Fatalf("写完之前状态应为 stale，实际 %q", snap.RedirectStatus)
	}

	// 在途期间再来一次心跳，不能并发写第二遍
	if e.shouldSyncRedirect("https://zlux.top:25888") {
		t.Fatal("已经有一次在写了，不该再投一次")
	}

	e.recordRedirect("https://zlux.top:25888", true, nil)
	snap = e.snapshot("1-2", EventPhaseChanged)
	if snap.RedirectStatus != "ok" || snap.RedirectTarget != "https://zlux.top:25888" {
		t.Fatalf("写完后应为 ok + 新地址，实际 %q / %q", snap.RedirectStatus, snap.RedirectTarget)
	}
	// 写完了才允许下一次
	if e.shouldSyncRedirect("https://zlux.top:25888") {
		t.Fatal("目标没变不该重复打服务商 API")
	}
}

// TestShouldSyncRedirectRetriesAfterCooldown 失败的那条到了冷却时间要能自己再来一次
func TestShouldSyncRedirectRetriesAfterCooldown(t *testing.T) {
	e := newServiceEntry(func() {}, 1, 2, "NAS", "DSM")
	target := "https://zlux.top:26853"
	e.recordRedirect(target, true, errors.New("token 没权限"))

	if e.shouldSyncRedirect(target) {
		t.Fatal("冷却期内不该重试")
	}

	e.mu.Lock()
	e.redirectRetryAt = time.Now().Add(-time.Second)
	e.mu.Unlock()

	if !e.shouldSyncRedirect(target) {
		t.Fatal("冷却时间过了应该重试")
	}
}

// TestRedirectTargetPort 端口抠不出来时返回 0，不能当成「端口变了」乱标
func TestRedirectTargetPort(t *testing.T) {
	cases := map[string]uint16{
		"https://zlux.top:26853": 26853,
		"http://1.2.3.4:80":      80,
		"https://zlux.top":       0,
		"":                       0,
		"::not a url":            0,
	}
	for in, want := range cases {
		if got := redirectTargetPort(in); got != want {
			t.Errorf("redirectTargetPort(%q) = %d，期望 %d", in, got, want)
		}
	}
}
