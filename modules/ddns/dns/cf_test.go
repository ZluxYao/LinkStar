package dns

import (
	"fmt"
	"testing"
)

// 运行方式（PowerShell）:
//
//	go test ./modules/ddns/dns/ -v -run TestCloudflareGetZone
func TestCloudflareGetZone(t *testing.T) {
	token := ""
	domain := "zlux.top"
	if token == "" || domain == "" {
		t.Skip("未设置 token / domain,跳过")
	}

	cf := NewCloudflare(token)

	resp, err := cf.GetZone(domain)
	if err != nil {
		t.Fatalf("GetZone 请求失败: %v", err)
	}

	if !resp.Success {
		t.Fatalf("API 返回失败: %v", resp.Messages)
	}

	if len(resp.Result) == 0 {
		t.Fatalf("未找到域名 %s 对应的 zone", domain)
	}

	t.Logf("zone ID: %s, name: %s, status: %s",
		resp.Result[0].ID, resp.Result[0].Name, resp.Result[0].Status)
	fmt.Println(resp)
}
