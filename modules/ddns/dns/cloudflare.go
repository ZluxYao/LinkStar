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

// CloudflareRecordsResp records
type CloudflareRecordsResp struct {
	CloudflareStatus
	Result []CloudflareRecord
}

// CloudflareRecord 记录实体
type CloudflareRecord struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Type    string `json:"type"`
	Content string `json:"content"`
	Proxied bool   `json:"proxied"`
	TTL     int    `json:"ttl"`
	Comment string `json:"comment"`
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

// SetRecord 设置一条记录
func (cf *Cloudflare) SetRecord(domain, subDomain, recordType, ipAddr string, ttl int, proxied bool) error {
	// 拼接完整域名:子域名为空或者@ 就在主域名本身
	fullName := subDomain + "." + domain
	if subDomain == "" || subDomain == "@" {
		fullName = domain
	}

	// 1. 查询zone
	zones, err := cf.getZones(domain)
	if err != nil {
		return fmt.Errorf("查询 zone 失败:%w", err)
	}
	if !zones.Success {
		return fmt.Errorf("查询 zone 返回失败: %v", zones.Messages)
	}
	if len(zones.Result) == 0 {
		return fmt.Errorf("未找到域名 %s 对应的 zone", domain)
	}
	zoneID := zones.Result[0].ID
	fmt.Printf("Debug:zoneID:%s \n", zoneID)

	// 2. 查询这条记录存不存在
	params := url.Values{}
	params.Set("type", recordType)
	params.Set("name", fullName)
	params.Set("per_page", "50")

	var records CloudflareRecordsResp
	err = cf.request(
		"GET",
		fmt.Sprintf(zonesAPI+"/%s/dns_records?%s", zoneID, params.Encode()),
		nil,
		&records,
	)
	if err != nil {
		return fmt.Errorf("查询记录失败: %w", err)
	}
	if !records.Success {
		return fmt.Errorf("查询记录返回失败: %v", records.Messages)
	}
	fmt.Printf("Debug:records:%v \n", records)

	// 3.存在 -> 更新 (ip相同则跳过)
	if len(records.Result) > 0 {
		return cf.modify(records.Result[0], zoneID, ipAddr, ttl, proxied)
	}
	return cf.create(zoneID, fullName, recordType, ipAddr, ttl, proxied)

}

// 新增
func (cf *Cloudflare) create(zoneID, fullName, recordType, ipAddr string, ttl int, proxied bool) error {
	record := CloudflareRecord{
		Name:    fullName,
		Type:    recordType,
		Content: ipAddr,
		Proxied: proxied,
		TTL:     ttl,
	}
	var status CloudflareStatus
	err := cf.request(
		"POST",
		fmt.Sprintf(zonesAPI+"/%s/dns_records", zoneID),
		record,
		&status,
	)
	if err != nil {
		return fmt.Errorf("新建记录失败: %w", err)
	}
	if !status.Success {
		return fmt.Errorf("新建记录返回失败: %v", status.Messages)
	}
	return nil
}

// 修改
func (cf *Cloudflare) modify(record CloudflareRecord, zoneID string, ipAddr string, ttl int, proxied bool) error {
	if record.Content == ipAddr {
		// ip 没变无需更新
		return nil
	}
	record.Content = ipAddr
	record.TTL = ttl
	record.Proxied = proxied

	var Status CloudflareStatus
	err := cf.request(
		"PUT",
		fmt.Sprintf(zonesAPI+"/%s/dns_records/%s", zoneID, record.ID),
		record,
		&Status,
	)
	if err != nil {
		return fmt.Errorf("更新记录失败：%w", err)
	}
	if !Status.Success {
		return fmt.Errorf("更新记录返回失败：%v", Status.Messages)
	}
	return nil
}

// 获取域名 zone 信息
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
