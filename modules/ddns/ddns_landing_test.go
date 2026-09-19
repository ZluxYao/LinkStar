package ddns

import (
	"net"
	"os"
	"testing"

	"linkstar/modules/ddns/model"
)

func TestSplitZone(t *testing.T) {
	cases := []struct {
		name    string
		host    string
		zone    string
		wantSub string
		wantOK  bool
	}{
		{"域名就是主域名本身", "example.com", "example.com", "@", true},
		{"一层子域名", "home.example.com", "example.com", "home", true},
		{"多层子域名", "a.b.example.com", "example.com", "a.b", true},
		{"大小写不影响", "Home.EXAMPLE.com", "example.com", "Home", true},
		{"不在这个主域名下面", "example.com", "other.org", "", false},
		{"只是后缀像但差一段", "notexample.com", "example.com", "", false},
		{"主域名留空", "example.com", "", "", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			sub, ok := splitZone(c.host, c.zone)
			if ok != c.wantOK || sub != c.wantSub {
				t.Fatalf("splitZone(%q, %q) = (%q, %v)，想要 (%q, %v)",
					c.host, c.zone, sub, ok, c.wantSub, c.wantOK)
			}
		})
	}
}

func TestGuessZone(t *testing.T) {
	cases := map[string]string{
		"example.com":            "example.com",
		"linkstarcf.example.com": "example.com",
		"a.b.c.example.com":      "example.com",
		"localhost":           "localhost",
	}
	for host, want := range cases {
		if got := guessZone(host); got != want {
			t.Fatalf("guessZone(%q) = %q，想要 %q", host, got, want)
		}
	}
}

// TestRecordFQDN 拼法必须和服务商 SetRecord 里那份一致，
// 不然「这个域名已经有记录了」会判错，然后重复建一条。
func TestRecordFQDN(t *testing.T) {
	cases := []struct {
		domain, sub, want string
	}{
		{"example.com", "", "example.com"},
		{"example.com", "@", "example.com"},
		{"example.com", "home", "home.example.com"},
		{"example.com.", " home ", "home.example.com"},
	}
	for _, c := range cases {
		if got := recordFQDN(c.domain, c.sub); got != c.want {
			t.Fatalf("recordFQDN(%q, %q) = %q，想要 %q", c.domain, c.sub, got, c.want)
		}
	}
}

// TestReleaseLandingRecord 服务删掉时只收回 LinkStar 自己补的那条。
//
// 反过来错了的代价不对称：少删一条，用户在列表里看见一条没用的记录，自己删掉就完了；
// 多删一条，删掉的是用户手写的解析记录，他那个域名当场解析不到，而且没有回收站。
func TestReleaseLandingRecord(t *testing.T) {
	t.Chdir(t.TempDir()) // 删记录会落盘，别写进仓库
	if err := os.MkdirAll("config", 0o755); err != nil {
		t.Fatal(err)
	}

	r := &DDNSRuntime{}
	r.Config = model.DDNSConfig{Records: []model.DDNSRecord{
		{ID: 1, Domain: "example.com", SubDomain: "nas", RecordType: model.DNSRecordTypeA, AutoCreated: true},
		{ID: 2, Domain: "example.com", SubDomain: "blog", RecordType: model.DNSRecordTypeA},
	}}

	// 用户自己加的那条：不能动
	removed, err := r.ReleaseLandingRecord("blog.example.com")
	if err != nil {
		t.Fatal(err)
	}
	if removed {
		t.Fatal("用户自己加的记录被删了")
	}

	// 压根没这个域名：当没事发生，别报错
	if removed, err := r.ReleaseLandingRecord("nothing.example.com"); err != nil || removed {
		t.Fatalf("没这个域名时应该什么都不做，得到 removed=%v err=%v", removed, err)
	}

	// 自己补的那条：收回去
	removed, err = r.ReleaseLandingRecord("nas.example.com")
	if err != nil {
		t.Fatal(err)
	}
	if !removed {
		t.Fatal("自动加的记录没被收回")
	}

	left := r.Snapshot().Records
	if len(left) != 1 || left[0].ID != 2 {
		t.Fatalf("剩下的记录不对：%+v", left)
	}
}

// TestListInterfaces 列出来的每一块都得是能选的：有地址，且没有回环/链路本地混进来。
// 选项列表和 resolveFromInterface 的过滤口径一旦分家，用户会选到一个后端挑不出地址的网卡。
func TestListInterfaces(t *testing.T) {
	list, err := ListInterfaces()
	if err != nil {
		t.Fatalf("列网卡失败: %v", err)
	}
	for _, item := range list {
		if item.Name == "" {
			t.Fatal("网卡名为空")
		}
		if len(item.IPv4) == 0 && len(item.IPv6) == 0 {
			t.Fatalf("网卡 %s 一个地址都没有，不该出现在列表里", item.Name)
		}
		for _, s := range append(append([]string{}, item.IPv4...), item.IPv6...) {
			ip := net.ParseIP(s)
			if ip == nil {
				t.Fatalf("网卡 %s 上的 %q 不是合法 IP", item.Name, s)
			}
			if !usableIfaceIP(ip) {
				t.Fatalf("网卡 %s 上的 %s 写进 DNS 没有意义，不该列出来", item.Name, s)
			}
		}
	}
}
