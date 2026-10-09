package stun_api

import (
	"bytes"
	"context"
	"encoding/json"
	"linkstar/middleware"
	"linkstar/modules/stun"
	"linkstar/modules/stun/model"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/gin-gonic/gin"
)

type idleRunner struct{}

func (idleRunner) Run(ctx context.Context, _ stun.STUNRequest, _ func(stun.STUNState)) error {
	<-ctx.Done()
	return nil
}

// TestServiceUpdateDropsUPnPMappedPort 表单不再有「UPnP 映射端口」，老配置里留着的数也得清掉：
// 它从来不是真实映射，留着会让首页在洞没通时显示一个不存在的端口。
func TestServiceUpdateDropsUPnPMappedPort(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := os.MkdirAll("config", 0755); err != nil {
		t.Fatal(err)
	}
	old, oldScheduler := stun.Runtime.Config, stun.Runtime.Scheduler
	t.Cleanup(func() { stun.Runtime.Config, stun.Runtime.Scheduler = old, oldScheduler })
	// 保存完会重启服务；服务是停用的，调度器只做个样子，不会真去打洞
	stun.Runtime.Scheduler = stun.NewScheduler(idleRunner{})
	t.Cleanup(stun.Runtime.Scheduler.Close)
	stun.Runtime.Config = model.Config{Devices: []model.Device{{
		DeviceID: 1, Name: "本机",
		Services: []model.Service{{ID: 1, Name: "mc", InternalPort: 25565, Protocol: "TCP", UPnPMappedPort: 34521}},
	}}}

	// 老前端还会把这个字段带上来，新后端照样不认
	raw, _ := json.Marshal(map[string]any{
		"deviceId": 1, "serviceId": 1, "name": "mc", "internalPort": 25565, "protocol": "TCP",
		"upnpMappedPort": 40000, "enabled": false,
	})
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.PUT("/u", middleware.BindJsonMiddleware[StunServiceUpdateViewRequest], StunApi{}.StunServiceUpdateView)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("PUT", "/u", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(rec, req)

	if got := stun.Runtime.Config.Devices[0].Services[0].UPnPMappedPort; got != 0 {
		t.Fatalf("UPnPMappedPort = %d，保存后应该清成 0（响应 %s）", got, rec.Body.String())
	}
}
