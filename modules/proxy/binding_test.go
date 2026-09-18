package proxy

import (
	"fmt"
	"strings"
	"testing"

	"linkstar/modules/proxy/model"
)

// describe 把一个 binding 压成一行，比一个字段一个字段断言好读：
// 端口 / 协议 / 证书 [挂在上面的站点]
func describe(bs []binding) string {
	parts := make([]string, 0, len(bs))
	for _, b := range bs {
		scheme := "http"
		if b.TLS {
			scheme = "https"
		}
		hosts := make([]string, 0, len(b.Sites))
		for _, s := range b.Sites {
			hosts = append(hosts, s.PrimaryHost())
		}
		parts = append(parts, fmt.Sprintf("%d/%s/cert%d[%s]", b.Port, scheme, b.CertID, strings.Join(hosts, " ")))
	}
	return strings.Join(parts, " ")
}

func bsite(host string, mut ...func(*model.Site)) model.Site {
	s := model.Site{Hosts: []string{host}, Backend: "127.0.0.1:1", Enabled: true}
	for _, f := range mut {
		f(&s)
	}
	return s
}

func onPort(p int) func(*model.Site)    { return func(s *model.Site) { s.ListenPort = p } }
func useTLS(s *model.Site)              { s.HTTPS = true }
func useCert(id uint) func(*model.Site) { return func(s *model.Site) { s.CertID = id } }
func off(s *model.Site)                 { s.Enabled = false }

func TestPlanBindings(t *testing.T) {
	cases := []struct {
		name string
		cfg  model.ProxyConfig
		want string
	}{
		{
			name: "关掉了就一个监听都不开",
			cfg: model.ProxyConfig{
				Enabled: false, HTTPPort: 80, HTTPSPort: 443,
				Sites: []model.Site{bsite("a.com")},
			},
			want: "",
		},
		{
			name: "没站点就不开端口，免得白占 80",
			cfg:  model.ProxyConfig{Enabled: true, HTTPPort: 80, HTTPSPort: 443},
			want: "",
		},
		{
			name: "默认入口：全都上 HTTP，勾了 HTTPS 的再上一份 HTTPS",
			cfg: model.ProxyConfig{
				Enabled: true, HTTPPort: 80, HTTPSPort: 443,
				Sites: []model.Site{bsite("a.com", useTLS), bsite("b.com")},
			},
			want: "80/http/cert0[a.com b.com] 443/https/cert0[a.com]",
		},
		{
			name: "一个都没勾 HTTPS 就不开 443",
			cfg: model.ProxyConfig{
				Enabled: true, HTTPPort: 80, HTTPSPort: 443,
				Sites: []model.Site{bsite("a.com")},
			},
			want: "80/http/cert0[a.com]",
		},
		{
			name: "HTTP 入口留空就只开 HTTPS",
			cfg: model.ProxyConfig{
				Enabled: true, HTTPSPort: 443,
				Sites: []model.Site{bsite("a.com", useTLS), bsite("b.com")},
			},
			want: "443/https/cert0[a.com]",
		},
		{
			name: "停用的站点不算",
			cfg: model.ProxyConfig{
				Enabled: true, HTTPPort: 80,
				Sites: []model.Site{bsite("a.com"), bsite("b.com", off)},
			},
			want: "80/http/cert0[a.com]",
		},
		{
			name: "站点自己占端口：单开一个监听，协议它自己说了算",
			cfg: model.ProxyConfig{
				Enabled: true, HTTPPort: 80, HTTPSPort: 443,
				Sites: []model.Site{bsite("a.com"), bsite("b.com", onPort(8443), useTLS), bsite("c.com", onPort(8080))},
			},
			want: "80/http/cert0[a.com] 8080/http/cert0[c.com] 8443/https/cert0[b.com]",
		},
		{
			name: "同一个自定义端口上的几个站点合成一个监听",
			cfg: model.ProxyConfig{
				Enabled: true,
				Sites:   []model.Site{bsite("a.com", onPort(8443), useTLS), bsite("b.com", onPort(8443), useTLS)},
			},
			want: "8443/https/cert0[a.com b.com]",
		},
		{
			name: "指定的端口正好是默认入口，当共享处理——同一个端口不能 bind 两次",
			cfg: model.ProxyConfig{
				Enabled: true, HTTPPort: 80, HTTPSPort: 443,
				Sites: []model.Site{bsite("a.com", onPort(443), useTLS), bsite("b.com", onPort(80))},
			},
			want: "80/http/cert0[a.com b.com] 443/https/cert0[a.com]",
		},
		{
			name: "一个端口上大家指同一张证书，就用那张",
			cfg: model.ProxyConfig{
				Enabled: true, HTTPSPort: 443,
				Sites: []model.Site{bsite("a.com", useTLS, useCert(7)), bsite("b.com", useTLS, useCert(7))},
			},
			want: "443/https/cert7[a.com b.com]",
		},
		{
			name: "证书指得不一样就退回 SNI 自动匹配",
			cfg: model.ProxyConfig{
				Enabled: true, HTTPSPort: 443,
				Sites: []model.Site{bsite("a.com", useTLS, useCert(7)), bsite("b.com", useTLS, useCert(9))},
			},
			want: "443/https/cert0[a.com b.com]",
		},
		{
			name: "站点没指定证书就用入口的默认证书",
			cfg: model.ProxyConfig{
				Enabled: true, HTTPSPort: 443, CertID: 3,
				Sites: []model.Site{bsite("a.com", useTLS)},
			},
			want: "443/https/cert3[a.com]",
		},
		{
			name: "站点指定了证书就盖过入口的默认证书",
			cfg: model.ProxyConfig{
				Enabled: true, HTTPSPort: 443, CertID: 3,
				Sites: []model.Site{bsite("a.com", useTLS, useCert(7))},
			},
			want: "443/https/cert7[a.com]",
		},
		{
			name: "自定义端口不吃入口的默认证书：那张是给默认入口配的",
			cfg: model.ProxyConfig{
				Enabled: true, HTTPSPort: 443, CertID: 3,
				Sites: []model.Site{bsite("a.com", onPort(8443), useTLS)},
			},
			want: "8443/https/cert0[a.com]",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := describe(planBindings(c.cfg)); got != c.want {
				t.Fatalf("planBindings =\n  %q\n期望\n  %q", got, c.want)
			}
		})
	}
}

// 端口相同、证书相同的两次重算必须给出同一个 spec，
// 否则 Apply 会认为参数变了，把好端端的监听重开一遍。
func TestBindingSpecStableAcrossSiteEdits(t *testing.T) {
	cfg := model.ProxyConfig{
		Enabled: true, HTTPPort: 80, HTTPSPort: 443,
		Sites: []model.Site{bsite("a.com", useTLS)},
	}
	before := planBindings(cfg)

	cfg.Sites = append(cfg.Sites, bsite("b.com"))
	after := planBindings(cfg)

	if len(before) != 2 || len(after) != 2 {
		t.Fatalf("端口数变了：before=%d after=%d", len(before), len(after))
	}
	for i := range before {
		if before[i].spec() != after[i].spec() {
			t.Fatalf("加一个站点不该改变监听参数：%+v → %+v", before[i].spec(), after[i].spec())
		}
	}
}
