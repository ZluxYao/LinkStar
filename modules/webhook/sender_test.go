package webhook

import (
	"os"
	"strings"
	"testing"
)

// go test ./modules/webhook -run TestSendCloudflareSRVRecord -v
func TestSendCloudflareSRVRecord(t *testing.T) {
	token := strings.TrimSpace(os.Getenv("CF_API_TOKEN"))
	if token == "" {
		t.Skip("缺少环境变量 CF_API_TOKEN，跳过真实 Cloudflare API 测试")
	}

	cfg := WebhookConfig{
		URL:    "https://api.cloudflare.com/client/v4/zones/",
		Method: "PUT",
		Headers: strings.Join([]string{
			"Authorization: Bearer #{token}",
			"Content-Type: application/json",
		}, "\n"),
		Body: `{
			"type": "SRV",
			"name": "_aa._tcp.istore",
			"ttl": 60,
			"data": {
				"service": "_aa",
				"proto": "_tcp",
				"name": "istore",
				"priority": 5,
				"weight": 0,
				"port": #{port},
				"target": "example.top"
			}
		}`,
		DisableSuccessCheck: true,
		SuccessContains:     `"success":true`,
	}

	resp, err := Send(cfg, map[string]string{
		"token": token,
		"port":  "13311",
	})
	if err != nil {
		t.Fatalf("Send failed: %v\nresp: %s", err, resp)
	}

	t.Logf("Cloudflare response: %s", resp)
}

func Test非法代理地址返回错误而不是空指针(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("非法代理不应 panic: %v", r)
		}
	}()
	_, err := Send(WebhookConfig{
		URL:    "http://127.0.0.1:1/",
		Method: "GET",
		Proxy:  "://bad-proxy",
	}, nil)
	if err == nil || !strings.Contains(err.Error(), "代理地址无效") {
		t.Fatalf("应返回代理地址无效，实际 %v", err)
	}
}
