package proxy

import (
	"testing"

	"linkstar/modules/proxy/model"
)

func TestNormalizeSiteHosts(t *testing.T) {
	cases := []struct {
		name string
		in   []string
		base string
		want string // 逗号分隔；"!" 开头表示期望报错
	}{
		{"补主域名", []string{"nas"}, "zlux.top", "nas.zlux.top"},
		{"写全了就不补", []string{"nas.other.com"}, "zlux.top", "nas.other.com"},
		{"多个域名各补各的", []string{"nas", "www.nas.zlux.top"}, "zlux.top", "nas.zlux.top,www.nas.zlux.top"},
		{"统一小写", []string{"NAS.Zlux.Top"}, "", "nas.zlux.top"},
		{"空行丢掉", []string{"a.com", "", "  "}, "", "a.com"},
		// 补完再去重：nas 和 nas.zlux.top 补完是同一个，留一个就行
		{"补完撞上了要去重", []string{"nas", "nas.zlux.top"}, "zlux.top", "nas.zlux.top"},
		{"通配也补", []string{"*"}, "zlux.top", "*.zlux.top"},
		{"一个域名都没有，又没占端口", []string{"", " "}, "", "!"},
		{"带端口要拦住", []string{"nas:8080"}, "zlux.top", "!"},
		{"带协议要拦住", []string{"http://nas.zlux.top"}, "", "!"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			out, err := normalizeSite(model.Site{Hosts: c.in, Backend: "127.0.0.1:1"}, c.base)
			if c.want == "!" {
				if err == nil {
					t.Fatalf("期望报错，却拿到 %v", out.Hosts)
				}
				return
			}
			if err != nil {
				t.Fatalf("不该报错：%v", err)
			}
			if got := joinHosts(out.Hosts); got != c.want {
				t.Fatalf("hosts = %q，期望 %q", got, c.want)
			}
		})
	}
}

// TestNormalizeSitePortForward 没填域名的站点：占了端口就放行，当端口转发使
func TestNormalizeSitePortForward(t *testing.T) {
	t.Run("占了端口就放行", func(t *testing.T) {
		out, err := normalizeSite(model.Site{Backend: "127.0.0.1:3333", ListenPort: 666}, "zlux.top")
		if err != nil {
			t.Fatalf("填了端口就不该拦：%v", err)
		}
		if len(out.Hosts) != 0 {
			t.Fatalf("不该凭空补出域名，实际 %v", out.Hosts)
		}
		// 日志和报错里得有个能指代它的名字
		if got := out.Label(); got != ":666" {
			t.Fatalf("Label() = %q，期望 \":666\"", got)
		}
	})

	t.Run("没端口也没域名才拦", func(t *testing.T) {
		if _, err := normalizeSite(model.Site{Backend: "127.0.0.1:3333"}, "zlux.top"); err == nil {
			t.Fatal("挂默认入口又没有域名，应该拦住")
		}
	})
}

// TestConflictSitePortForward 没域名可比时按端口 + 路径比
func TestConflictSitePortForward(t *testing.T) {
	existing := []model.Site{
		{ID: 1, Backend: "127.0.0.1:1", ListenPort: 666},
		{ID: 2, Backend: "127.0.0.1:2", ListenPort: 666, PathPrefix: "/api"},
		// 888 上那条是写了域名的，按 Host 走
		{ID: 3, Hosts: []string{"a.com"}, Backend: "127.0.0.1:3", ListenPort: 888},
	}
	cases := []struct {
		name string
		site model.Site
		want bool
	}{
		{"同端口同路径两条兜底，谁生效全看排序", model.Site{ListenPort: 666}, true},
		{"同端口同路径·带前缀那条", model.Site{ListenPort: 666, PathPrefix: "/api"}, true},
		{"同端口不同路径可以共存", model.Site{ListenPort: 666, PathPrefix: "/photo"}, false},
		{"不同端口各管各的", model.Site{ListenPort: 999}, false},
		{"改自己不算撞", model.Site{ID: 1, ListenPort: 666}, false},
		// 888 上那条按 Host 分流，兜底站点收的是它挑剩的，两者不冲突
		{"同端口上写了域名的站点不算撞", model.Site{ListenPort: 888}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if _, _, ok := conflictSite(existing, c.site); ok != c.want {
				t.Fatalf("冲突判定 = %v，期望 %v", ok, c.want)
			}
		})
	}
}

// TestConflictSiteAcrossHosts 一个站点挂多个域名之后，撞车判据是「域名集合有交集」
func TestConflictSiteAcrossHosts(t *testing.T) {
	existing := []model.Site{
		{ID: 1, Hosts: []string{"a.com", "www.a.com"}, Backend: "127.0.0.1:1"},
		{ID: 2, Hosts: []string{"b.com"}, PathPrefix: "/photo", Backend: "127.0.0.1:2"},
	}

	cases := []struct {
		name     string
		site     model.Site
		wantHost string // "" = 不该撞
	}{
		{"完全不相干", model.Site{Hosts: []string{"c.com"}}, ""},
		{"撞第一个域名", model.Site{Hosts: []string{"a.com"}}, "a.com"},
		// 撞的是别名，报错要说 www.a.com，不能只报 a.com 让用户去猜
		{"撞到别名上", model.Site{Hosts: []string{"www.a.com"}}, "www.a.com"},
		{"改自己不算撞", model.Site{ID: 1, Hosts: []string{"a.com"}}, ""},
		{"同域名不同路径不算撞", model.Site{Hosts: []string{"b.com"}}, ""},
		{"同域名同路径才算撞", model.Site{Hosts: []string{"b.com"}, PathPrefix: "/photo"}, "b.com"},
		{"新站点多域名，只要有一个撞上就算", model.Site{Hosts: []string{"c.com", "www.a.com"}}, "www.a.com"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, host, ok := conflictSite(existing, c.site)
			if c.wantHost == "" {
				if ok {
					t.Fatalf("不该判为冲突，却报了 %q", host)
				}
				return
			}
			if !ok {
				t.Fatalf("应该判为冲突（%s）", c.wantHost)
			}
			if host != c.wantHost {
				t.Fatalf("冲突域名 = %q，期望 %q", host, c.wantHost)
			}
		})
	}
}

func joinHosts(hosts []string) string {
	out := ""
	for i, h := range hosts {
		if i > 0 {
			out += ","
		}
		out += h
	}
	return out
}
