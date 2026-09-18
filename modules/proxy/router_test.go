package proxy

import (
	"net/url"
	"testing"

	"linkstar/modules/proxy/model"
)

func site(id uint, host, prefix, backend string) model.Site {
	return model.Site{
		ID:         id,
		Hosts:      []string{host},
		PathPrefix: prefix,
		Backend:    backend,
		Enabled:    true,
	}
}

func TestRouterMatch(t *testing.T) {
	r := NewRouter([]model.Site{
		site(1, "fn.zlux.top", "", "192.168.1.10:8080"),
		site(2, "nas.zlux.top", "", "192.168.1.20:5000"),
		site(3, "nas.zlux.top", "/photo", "192.168.1.20:5001"),
		site(4, "nas.zlux.top", "/photo/raw", "192.168.1.20:5002"),
		site(5, "*.zlux.top", "", "192.168.1.30:80"),
		{ID: 6, Hosts: []string{"off.zlux.top"}, Backend: "192.168.1.40:80", Enabled: false},
		// 一个站点挂两个域名：裸域 + www，两个都该落到同一条上
		{ID: 7, Hosts: []string{"blog.zlux.top", "www.blog.zlux.top"}, Backend: "192.168.1.50:2368", Enabled: true},
	})

	cases := []struct {
		name string
		host string
		path string
		want uint // 0 = 期望不命中
	}{
		{"精确 Host", "fn.zlux.top", "/", 1},
		// 洞是高位端口，浏览器一定会带上，不剥端口这条就废了
		{"Host 带端口", "fn.zlux.top:34521", "/", 1},
		{"大写 Host", "FN.Zlux.Top", "/", 1},
		{"Host 带尾点", "fn.zlux.top.", "/", 1},

		{"同 Host 无前缀兜底", "nas.zlux.top", "/", 2},
		{"最长前缀优先", "nas.zlux.top", "/photo/a.jpg", 3},
		{"更长的前缀再优先", "nas.zlux.top", "/photo/raw/a.dng", 4},
		{"前缀本身", "nas.zlux.top", "/photo", 3},
		// /photo 不能吃掉 /photobooth，否则加新服务就会被老规则抢走
		{"前缀必须卡在分隔处", "nas.zlux.top", "/photobooth", 2},

		{"通配兜底", "other.zlux.top", "/", 5},
		// 精确站点不该被 *.zlux.top 抢走
		{"精确优先于通配", "fn.zlux.top", "/", 1},
		// 通配只吃一层，裸域和多层都不算
		{"通配不匹配裸域", "zlux.top", "/", 0},
		{"通配不匹配多层", "a.b.zlux.top", "/", 0},

		// 一条站点挂多个域名，每个都要能进来
		{"多域名·第一个", "blog.zlux.top", "/", 7},
		{"多域名·第二个", "www.blog.zlux.top", "/", 7},

		{"禁用的站点不生效", "off.zlux.top", "/", 5}, // 落到通配上
		{"Host 为空", "", "/", 0},
		{"完全不相干的域名", "example.com", "/", 0},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := r.Match(c.host, c.path)
			if c.want == 0 {
				if got != nil {
					t.Fatalf("Match(%q, %q) 命中了站点 %d，期望不命中", c.host, c.path, got.Site.ID)
				}
				return
			}
			if got == nil {
				t.Fatalf("Match(%q, %q) 未命中，期望站点 %d", c.host, c.path, c.want)
			}
			if got.Site.ID != c.want {
				t.Fatalf("Match(%q, %q) = 站点 %d，期望 %d", c.host, c.path, got.Site.ID, c.want)
			}
		})
	}
}

func TestNewRouterSkipsIncomplete(t *testing.T) {
	r := NewRouter([]model.Site{
		site(1, "a.zlux.top", "", ""), // 没有后端，转发不到任何地方
		site(2, "  ", "", "  "),
	})
	if !r.Empty() {
		t.Fatalf("填写不全的站点不该进转发表，实际 Hosts=%v", r.Hosts())
	}
}

// TestRouterFallbackSite 没填域名的站点 = 当端口转发使：
// 这个端口上的请求不管 Host 写的什么都该转过去
func TestRouterFallbackSite(t *testing.T) {
	r := NewRouter([]model.Site{
		{ID: 1, Hosts: []string{"nas.zlux.top"}, Backend: "192.168.1.20:5000", Enabled: true},
		{ID: 2, Backend: "127.0.0.1:3333", ListenPort: 666, Enabled: true}, // 不限域名
	})

	cases := []struct {
		name string
		host string
		want uint
	}{
		// 写明域名的站点优先，兜底不能把它抢走
		{"精确域名照样优先", "nas.zlux.top", 1},
		{"没配过的域名落到兜底", "随便什么.example.com", 2},
		// 洞是按端口来的，外面可能直接敲 IP，根本没有域名可言
		{"敲 IP 访问", "192.168.1.9:666", 2},
		{"Host 为空也该转", "", 2},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := r.Match(c.host, "/")
			if got == nil {
				t.Fatalf("Match(%q) 未命中，期望站点 %d", c.host, c.want)
			}
			if got.Site.ID != c.want {
				t.Fatalf("Match(%q) = 站点 %d，期望 %d", c.host, got.Site.ID, c.want)
			}
		})
	}
}

// TestRouterFallbackPrefix 兜底站点之间照样按路径前缀分，长的优先
func TestRouterFallbackPrefix(t *testing.T) {
	r := NewRouter([]model.Site{
		{ID: 1, Backend: "127.0.0.1:1", ListenPort: 666, Enabled: true},
		{ID: 2, PathPrefix: "/api", Backend: "127.0.0.1:2", ListenPort: 666, Enabled: true},
	})
	for path, want := range map[string]uint{"/": 1, "/index.html": 1, "/api": 2, "/api/v1": 2} {
		got := r.Match("whatever", path)
		if got == nil || got.Site.ID != want {
			t.Errorf("Match(_, %q) = %v，期望站点 %d", path, got, want)
		}
	}
}

// TestRouterSizeCountsSitesNotHosts 一个站点挂 N 个域名还是一条站点，
// 页面上的「N 站点」不能跟着域名数涨
func TestRouterSizeCountsSitesNotHosts(t *testing.T) {
	r := NewRouter([]model.Site{
		{ID: 1, Hosts: []string{"a.com", "www.a.com", "*.a.com"}, Backend: "127.0.0.1:1", Enabled: true},
		{ID: 2, Hosts: []string{"b.com"}, Backend: "127.0.0.1:2", Enabled: true},
		{ID: 3, Backend: "127.0.0.1:3", ListenPort: 666, Enabled: true},
	})
	if got := r.Size(); got != 3 {
		t.Fatalf("Size() = %d，期望 3", got)
	}
}

func TestNormalizePrefix(t *testing.T) {
	cases := map[string]string{
		"":             "",
		"/":            "",
		"photo":        "/photo",
		"/photo":       "/photo",
		"/photo/":      "/photo",
		"  /photo/  ":  "/photo",
		"/photo/raw//": "/photo/raw",
	}
	for in, want := range cases {
		if got := normalizePrefix(in); got != want {
			t.Errorf("normalizePrefix(%q) = %q，期望 %q", in, got, want)
		}
	}
}

func TestStripPrefixKeepsPathConsistent(t *testing.T) {
	cases := []struct {
		raw    string
		prefix string
		want   string
	}{
		{"/photo/a.jpg", "/photo", "/a.jpg"},
		{"/photo", "/photo", "/"},
		// 前缀对不上就一个字都别改，宁可原样发过去
		{"/other/a.jpg", "/photo", "/other/a.jpg"},
	}
	for _, c := range cases {
		u := mustURL(t, c.raw)
		stripPrefix(u, c.prefix)
		if u.Path != c.want {
			t.Errorf("stripPrefix(%q, %q).Path = %q，期望 %q", c.raw, c.prefix, u.Path, c.want)
		}
	}
}

func TestParseBackend(t *testing.T) {
	cases := []struct {
		site model.Site
		want string
	}{
		{model.Site{Backend: "192.168.1.10:8080"}, "http://192.168.1.10:8080"},
		{model.Site{Backend: "192.168.1.10:8443", BackendHTTPS: true}, "https://192.168.1.10:8443"},
		// 用户明确写出来的 scheme 比一个勾选框更可信
		{model.Site{Backend: "https://192.168.1.10:8443"}, "https://192.168.1.10:8443"},
		{model.Site{Backend: "http://192.168.1.10:80", BackendHTTPS: true}, "http://192.168.1.10:80"},
		{model.Site{Backend: "http://192.168.1.10:80/app/"}, "http://192.168.1.10:80/app"},
	}
	for _, c := range cases {
		if got := parseBackend(c.site).String(); got != c.want {
			t.Errorf("parseBackend(%+v) = %q，期望 %q", c.site, got, c.want)
		}
	}
}

func mustURL(t *testing.T, raw string) *url.URL {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("url.Parse(%q): %v", raw, err)
	}
	return u
}
