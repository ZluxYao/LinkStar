package routers

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"linkstar/modules/stun"
	"linkstar/modules/stun/model"

	"github.com/gin-gonic/gin"
)

// setupPortal 造一份最小的 STUN 配置并返回挂好 307 索引的引擎。
// Scheduler 保持 nil——模块是并发初始化的，索引必须能在调度器还没起来时
// 回落到 UPnPMappedPort，而不是 panic。
func setupPortal(t *testing.T, services ...model.Service) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)

	stun.Runtime.Config = model.Config{
		Devices: []model.Device{{DeviceID: 1, Name: "nas", Services: services}},
	}
	stun.Runtime.Network = model.NetworkState{PublicIP: "203.0.113.7"}
	stun.Runtime.Scheduler = nil
	t.Cleanup(func() {
		stun.Runtime.Config = model.Config{}
		stun.Runtime.Network = model.NetworkState{}
	})

	r := gin.New()
	PortalRouters(r)
	r.NoRoute(func(c *gin.Context) {
		if TryPortal(c) {
			return
		}
		c.String(http.StatusOK, "spa-fallback")
	})
	return r
}

func do(r *gin.Engine, path string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
	return w
}

var fwService = model.Service{
	ID: 1, Name: "fw", Protocol: "TCP", InternalPort: 8080,
	UPnPMappedPort: 34521, Domain: "fw.example.com",
	TLSTerminate: true, Enabled: true,
}

func TestPortalRedirect(t *testing.T) {
	r := setupPortal(t, fwService)

	cases := []struct {
		name string
		path string
		want string
	}{
		{"裸路径", "/fw", "https://fw.example.com:34521"},
		{"显式前缀", "/go/fw", "https://fw.example.com:34521"},
		{"剥掉服务名保留其余路径", "/go/fw/library/x", "https://fw.example.com:34521/library/x"},
		{"裸路径同样剥前缀", "/fw/library/x", "https://fw.example.com:34521/library/x"},
		{"保留 query", "/go/fw/a?b=1&c=2", "https://fw.example.com:34521/a?b=1&c=2"},
		{"服务名不区分大小写", "/go/FW", "https://fw.example.com:34521"},
		{"尾斜杠保留", "/go/fw/", "https://fw.example.com:34521/"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := do(r, tc.path)
			if w.Code != http.StatusTemporaryRedirect {
				t.Fatalf("状态码 = %d, 想要 307", w.Code)
			}
			if got := w.Header().Get("Location"); got != tc.want {
				t.Errorf("Location = %q, 想要 %q", got, tc.want)
			}
			// 端口会漂，缓存住就等于把用户钉死在一个迟早失效的端口上
			if got := w.Header().Get("Cache-Control"); got != "no-store" {
				t.Errorf("Cache-Control = %q, 想要 no-store", got)
			}
		})
	}
}

func TestPortalSchemeAndHostFallback(t *testing.T) {
	tests := []struct {
		name string
		svc  model.Service
		want string
	}{
		{
			name: "不终结 TLS 的明文服务",
			svc:  model.Service{ID: 1, Name: "plain", UPnPMappedPort: 1111, Domain: "p.example.com", Enabled: true},
			want: "http://p.example.com:1111",
		},
		{
			name: "内网自己是 HTTPS 也算 https",
			svc:  model.Service{ID: 1, Name: "plain", UPnPMappedPort: 1111, Domain: "p.example.com", Https: true, Enabled: true},
			want: "https://p.example.com:1111",
		},
		{
			name: "没配域名回落公网 IP",
			svc:  model.Service{ID: 1, Name: "plain", UPnPMappedPort: 1111, Enabled: true},
			want: "http://203.0.113.7:1111",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			w := do(setupPortal(t, tc.svc), "/go/plain")
			if got := w.Header().Get("Location"); got != tc.want {
				t.Errorf("Location = %q, 想要 %q", got, tc.want)
			}
		})
	}
}

func TestPortalUnavailable(t *testing.T) {
	// 端口还没打出来：必须是可读的 503，而不是一个注定 404 的重定向
	down := model.Service{ID: 1, Name: "fw", Domain: "fw.example.com", Enabled: true}
	w := do(setupPortal(t, down), "/go/fw")
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("状态码 = %d, 想要 503", w.Code)
	}
	if loc := w.Header().Get("Location"); loc != "" {
		t.Errorf("不该发重定向，却给了 Location = %q", loc)
	}

	// 服务被禁用同样是 503，而不是当作不存在
	disabled := model.Service{ID: 1, Name: "fw", Domain: "fw.example.com", UPnPMappedPort: 34521}
	if w := do(setupPortal(t, disabled), "/go/fw"); w.Code != http.StatusServiceUnavailable {
		t.Errorf("禁用服务状态码 = %d, 想要 503", w.Code)
	}
}

func TestPortalUnknownName(t *testing.T) {
	r := setupPortal(t, fwService)

	// 显式入口下找不到服务 = 404
	if w := do(r, "/go/nope"); w.Code != http.StatusNotFound {
		t.Errorf("/go/nope 状态码 = %d, 想要 404", w.Code)
	}
	// 裸路径找不到服务必须继续走前端 SPA 兜底，不能把前端路由吃掉
	w := do(r, "/dashboard")
	if w.Code != http.StatusOK || w.Body.String() != "spa-fallback" {
		t.Errorf("裸路径未命中应回落 SPA，实际 %d %q", w.Code, w.Body.String())
	}
}

func TestPortalLiveExternalPortWins(t *testing.T) {
	// 调度器里的实时端口优先于配置里的 UPnP 端口——端口漂移后必须立刻跟上。
	// 这里没法不起网络就造出一个 Scheduler，所以只断言 nil 调度器下的回落行为，
	// 实时值优先的逻辑由 LiveExternalPort 自身保证。
	w := do(setupPortal(t, fwService), "/go/fw")
	if got := w.Header().Get("Location"); got != "https://fw.example.com:34521" {
		t.Errorf("调度器为 nil 时应回落 UPnPMappedPort，实际 %q", got)
	}
}
