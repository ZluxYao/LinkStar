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
		URL:    "https://api.cloudflare.com/client/v4/zones/55452537bf50dd7e2946971f53fa12ae/dns_records/4489eb6ab6a5f33b093207cf8ac8472a",
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
				"target": "zlux.top"
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
