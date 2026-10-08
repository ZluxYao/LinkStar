package stun_api

import (
	"bytes"
	"encoding/json"
	"linkstar/middleware"
	"linkstar/modules/stun"
	"linkstar/modules/stun/model"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/gin-gonic/gin"
)

// putStunNetwork 走一遍 PUT stun/network（绑定中间件 + handler），返回 code 和 msg
func putStunNetwork(t *testing.T, body StunNetworkUpdateRequest) (int, string) {
	t.Helper()
	raw, _ := json.Marshal(body)
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.PUT("/n", middleware.BindJsonMiddleware[StunNetworkUpdateRequest], StunApi{}.StunNetworkUpdateView)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("PUT", "/n", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(rec, req)

	var resp struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("响应不是 JSON: %s", rec.Body.String())
	}
	return resp.Code, resp.Msg
}

// TestStunNetworkUpdate 保存网络设置：填错的不存，指定的网卡没连上照样存
func TestStunNetworkUpdate(t *testing.T) {
	// UpdateConfig 写的是工作目录下的 config/，换到临时目录里跑
	t.Chdir(t.TempDir())
	if err := os.MkdirAll("config", 0755); err != nil {
		t.Fatal(err)
	}
	old := stun.Runtime.Config.Network
	t.Cleanup(func() {
		stun.Runtime.Config.Network = old
		_, _ = stun.ApplyNetworkConfig(old)
	})

	cases := []struct {
		name     string
		body     StunNetworkUpdateRequest
		wantCode int
		want     model.NetworkConfig // wantCode 为 0 时检查存下来的
	}{
		{
			name:     "DNS 填了域名",
			body:     StunNetworkUpdateRequest{DNSMode: "custom", DNS: []string{"dns.example.com"}},
			wantCode: 7,
		},
		{
			name:     "自定义 DNS 一个没填",
			body:     StunNetworkUpdateRequest{DNSMode: "custom", DNS: []string{" ", ""}},
			wantCode: 7,
		},
		{
			name:     "指定网卡却没选",
			body:     StunNetworkUpdateRequest{IfaceMode: "custom"},
			wantCode: 7,
		},
		{
			name:     "模式写错",
			body:     StunNetworkUpdateRequest{IfaceMode: "auto"},
			wantCode: 7,
		},
		{
			name: "指定的网卡没连上也存下来，等它连上",
			body: StunNetworkUpdateRequest{IfaceMode: "custom", Iface: "一张不存在的网卡", DNSMode: "custom", DNS: []string{"192.168.100.1", " 223.5.5.5:53 "}},
			want: model.NetworkConfig{IfaceMode: "custom", Iface: "一张不存在的网卡", DNSMode: "custom", DNS: []string{"192.168.100.1", "223.5.5.5:53"}},
		},
		{
			name: "改回默认，老的网卡名和 DNS 不留",
			body: StunNetworkUpdateRequest{Iface: "一张不存在的网卡", DNS: []string{"192.168.100.1"}},
			want: model.NetworkConfig{},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			before := stun.Runtime.Config.Network
			code, msg := putStunNetwork(t, c.body)
			if code != c.wantCode {
				t.Fatalf("code = %d（%s），want %d", code, msg, c.wantCode)
			}
			got := stun.Runtime.Config.Network
			if c.wantCode != 0 {
				if !sameNetworkConfig(got, before) {
					t.Fatalf("填错了还是改了设置：%+v", got)
				}
				return
			}
			if !sameNetworkConfig(got, c.want) {
				t.Fatalf("存下来的是 %+v，want %+v", got, c.want)
			}
		})
	}
}

func sameNetworkConfig(a, b model.NetworkConfig) bool {
	if a.IfaceMode != b.IfaceMode || a.Iface != b.Iface || a.DNSMode != b.DNSMode || len(a.DNS) != len(b.DNS) {
		return false
	}
	for i := range a.DNS {
		if a.DNS[i] != b.DNS[i] {
			return false
		}
	}
	return true
}

// TestGetStunNetworkView 网卡和 DNS 列表不能是 null，前端拿去 .map / .join 会炸
func TestGetStunNetworkView(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	StunApi{}.GetStunNetworkView(c)

	var resp struct {
		Code int `json:"code"`
		Data struct {
			Config     map[string]any `json:"config"`
			Ifaces     []any          `json:"ifaces"`
			DefaultDNS []string       `json:"defaultDns"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil || resp.Code != 0 {
		t.Fatalf("响应不对: %s", rec.Body.String())
	}
	if resp.Data.Config["dns"] == nil {
		t.Error("config.dns 是 null")
	}
	if resp.Data.Ifaces == nil {
		t.Error("ifaces 是 null")
	}
	if len(resp.Data.DefaultDNS) == 0 {
		t.Error("defaultDns 是空的")
	}
}
