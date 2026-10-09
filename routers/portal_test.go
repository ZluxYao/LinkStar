package routers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"linkstar/modules/stun"
	"linkstar/modules/stun/model"

	"github.com/gin-gonic/gin"
)

// holeRunner 假装打通了一个洞：报一次外部端口，然后一直挂着直到被取消。
// 不碰网络，只为让调度器里有一个真实的 ExternalPort。
type holeRunner struct{ port uint16 }

func (r holeRunner) Run(ctx context.Context, _ stun.STUNRequest, onState func(stun.STUNState)) error {
	onState(stun.STUNState{State: stun.STUNMapped, ExternalPort: r.port})
	<-ctx.Done()
	return nil
}

// setupPortal 造一份最小的 STUN 配置并返回挂好 307 索引的引擎。
// port 为 0 时不起调度器——模块是并发初始化的，索引必须能在调度器还没起来时
// 如实回「还没就绪」，而不是 panic；非 0 时起一个假调度器，让服务以这个端口打通。
func setupPortal(t *testing.T, port uint16, services ...model.Service) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)

	stun.Runtime.Config = model.Config{
		Devices: []model.Device{{DeviceID: 1, Name: "nas", Services: services}},
	}
	stun.Runtime.Network = model.NetworkState{PublicIP: "203.0.113.7"}
	stun.Runtime.Scheduler = nil
	if port != 0 {
		s := stun.NewScheduler(holeRunner{port})
		stun.Runtime.Scheduler = s
		s.StartAll(stun.Runtime.Config.Devices)
		t.Cleanup(s.Close)
		waitForPort(t, services, port)
	}
	t.Cleanup(func() {
		stun.Runtime.Config = model.Config{}
		stun.Runtime.Network = model.NetworkState{}
		stun.Runtime.Scheduler = nil
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

// waitForPort 调度器是异步拉起服务的，等启用的那几个都报上端口
func waitForPort(t *testing.T, services []model.Service, port uint16) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for _, svc := range services {
		if !svc.Enabled {
			continue
		}
		for stun.LiveExternalPort(1, &svc) != port {
			if time.Now().After(deadline) {
				t.Fatalf("服务 %s 2 秒内没报上端口", svc.Name)
			}
			time.Sleep(5 * time.Millisecond)
		}
	}
}

func do(r *gin.Engine, path string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
	return w
}

var fwService = model.Service{
	ID: 1, Name: "fw", Protocol: "TCP", InternalPort: 8080,
	Domain: "fw.example.com", TLSTerminate: true, Enabled: true,
}

func TestPortalRedirect(t *testing.T) {
	r := setupPortal(t, 34521, fwService)

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
			svc:  model.Service{ID: 1, Name: "plain", Domain: "p.example.com", Enabled: true},
			want: "http://p.example.com:1111",
		},
		{
			name: "内网自己是 HTTPS 也算 https",
			svc:  model.Service{ID: 1, Name: "plain", Domain: "p.example.com", Https: true, Enabled: true},
			want: "https://p.example.com:1111",
		},
		{
			name: "没配域名回落公网 IP",
			svc:  model.Service{ID: 1, Name: "plain", Enabled: true},
			want: "http://203.0.113.7:1111",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			w := do(setupPortal(t, 1111, tc.svc), "/go/plain")
			if got := w.Header().Get("Location"); got != tc.want {
				t.Errorf("Location = %q, 想要 %q", got, tc.want)
			}
		})
	}
}

func TestPortalUnavailable(t *testing.T) {
	// 端口还没打出来：必须是可读的 503，而不是一个注定 404 的重定向
	down := model.Service{ID: 1, Name: "fw", Domain: "fw.example.com", Enabled: true}
	w := do(setupPortal(t, 0, down), "/go/fw")
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("状态码 = %d, 想要 503", w.Code)
	}
	if loc := w.Header().Get("Location"); loc != "" {
		t.Errorf("不该发重定向，却给了 Location = %q", loc)
	}

	// 服务被禁用同样是 503，而不是当作不存在
	disabled := model.Service{ID: 1, Name: "fw", Domain: "fw.example.com"}
	if w := do(setupPortal(t, 0, disabled), "/go/fw"); w.Code != http.StatusServiceUnavailable {
		t.Errorf("禁用服务状态码 = %d, 想要 503", w.Code)
	}
}

func TestPortalUnknownName(t *testing.T) {
	r := setupPortal(t, 34521, fwService)

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

// TestPortalIgnoresStaleUPnPPort 老配置里存着的 upnpMappedPort 是当初表单里随手填的，
// 从来不是真实映射。洞没打通时不能拿它拼地址，否则把人送到一个不存在的端口。
func TestPortalIgnoresStaleUPnPPort(t *testing.T) {
	old := model.Service{ID: 1, Name: "fw", Domain: "fw.example.com", UPnPMappedPort: 34521, Enabled: true}
	w := do(setupPortal(t, 0, old), "/go/fw")
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("洞没通应该 503，实际 %d，Location = %q", w.Code, w.Header().Get("Location"))
	}
}
