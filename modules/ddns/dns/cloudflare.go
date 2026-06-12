package dns

import (
	"bytes"
	"encoding/json"
	"fmt"
	httpUtils "linkstar/utils/http_utils"
	"net/http"
	"net/url"
	"time"
)

const zonesAPI = "https://api.cloudflare.com/client/v4/zones"

// Cloudflare
type Cloudflare struct {
	APIToken   string
	httpClient *http.Client
}

// NewCloudflare 创建 Cloudflare 客户端
func NewCloudflare(apiToken string) *Cloudflare {
	return &Cloudflare{
		APIToken:   apiToken,
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

// CloudflareZonesResp cloudflare zones返回结果
type CloudflareZonesResp struct {
	CloudflareStatus
	Result []struct {
		ID     string
		Name   string
		Status string
		Paused bool
	}
}

// CloudflareStatus 公共状态
type CloudflareStatus struct {
	Success  bool
	Messages []string
}

// GetZone 获取域名 zone 信息
func (cf *Cloudflare) getZones(domain string) (result CloudflareZonesResp, err error) {
	params := url.Values{}
	params.Set("name", domain)
	params.Set("status", "active")
	params.Set("per_page", "50")

	err = cf.request(
		"GET",
		fmt.Sprintf(zonesAPI+"?%s", params.Encode()),
		nil,
		&result,
	)
	return result, err
}

// request 统一请求接口
func (cf *Cloudflare) request(method string, url string, data interface{}, result interface{}) error {
	jsonStr := make([]byte, 0)
	var err error
	if data != nil {
		jsonStr, err = json.Marshal(data)
		if err != nil {
			return err
		}
	}
	req, err := http.NewRequest(method, url, bytes.NewBuffer(jsonStr))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+cf.APIToken)
	req.Header.Set("content-type", "application/json")

	client := cf.httpClient
	resp, err := client.Do(req)
	if err != nil {
		return err
	}

	return httpUtils.GetHTTPResponse(resp, result)
}
