package dns

import (
	"bytes"
	"encoding/json"
	"fmt"
	"linkstar/modules/ddns/model"
	httpUtils "linkstar/utils/http_utils"
	"net/http"
	"time"
)

// GoDaddy DNS
// 文档: https://developer.godaddy.com/doc/endpoint/domains
const godaddyEndpoint = "https://api.godaddy.com"

// GoDaddyDNS GoDaddy
type GoDaddyDNS struct {
	APIKey     string
	APISecret  string
	httpClient *http.Client
}

// godaddyRecord 记录实体
type godaddyRecord struct {
	Data string `json:"data"`
	Name string `json:"name"`
	TTL  int    `json:"ttl"`
	Type string `json:"type"`
}

// NewGoDaddyDNS 创建 GoDaddy 客户端
func NewGoDaddyDNS(apiKey string, apiSecret string) *GoDaddyDNS {
	return &GoDaddyDNS{
		APIKey:    apiKey,
		APISecret: apiSecret,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
			Transport: &http.Transport{
				Proxy: nil,
			},
		},
	}
}

// SetRecord 设置一条记录
// GoDaddy 的 PUT 接口直接覆盖指定 type+name 的记录,无需先查询
func (g *GoDaddyDNS) SetRecord(domain string, subDomain string, recordType model.DNSRecordType, ipAddr string, ttl int, proxied bool) error {
	rr := subDomain
	if rr == "" {
		rr = "@"
	}

	records := []godaddyRecord{{
		Data: ipAddr,
		Name: rr,
		TTL:  ttl,
		Type: string(recordType),
	}}

	body, err := json.Marshal(records)
	if err != nil {
		return err
	}

	apiURL := fmt.Sprintf("%s/v1/domains/%s/records/%s/%s", godaddyEndpoint, domain, string(recordType), rr)
	req, err := http.NewRequest(http.MethodPut, apiURL, bytes.NewBuffer(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", fmt.Sprintf("sso-key %s:%s", g.APIKey, g.APISecret))
	req.Header.Set("Content-Type", "application/json")

	resp, err := g.httpClient.Do(req)
	if err != nil {
		return err
	}
	// GoDaddy 成功返回空 body,只需检查状态码
	if _, err := httpUtils.GetHTTPResponseBody(resp); err != nil {
		return fmt.Errorf("设置记录失败: %w", err)
	}
	return nil
}
