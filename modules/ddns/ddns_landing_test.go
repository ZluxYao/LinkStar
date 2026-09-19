package ddns

import (
	"net"
	"testing"
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
