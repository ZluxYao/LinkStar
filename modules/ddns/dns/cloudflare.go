package dns

import (
	"bytes"
	"encoding/json"
	"fmt"
	"linkstar/modules/ddns/model"
	httpUtils "linkstar/utils/http_utils"
	"net/http"
	"net/url"
	"time"
)

// zonesAPI Cloudflare 的 zone 接口根地址。
// 是变量不是常量，只为一件事：测试里换成本地的假服务器。运行时不会被改。
var zonesAPI = "https://api.cloudflare.com/client/v4/zones"

// Cloudflare
type Cloudflare struct {
	APIToken   string
	httpClient *http.Client
}

// Cloudflare 支持 ACME DNS-01
var _ ACMEDNSProvider = (*Cloudflare)(nil)

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
		APIToken: apiToken,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
			Transport: &http.Transport{
				Proxy: nil,
			},
		},
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
func (cf *Cloudflare) SetRecord(domain string, subDomain string, recordType model.DNSRecordType, ipAddr string, ttl int, proxied bool) error {
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
	params.Set("type", string(recordType))
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

	// 3.存在 -> 更新 (完全相同则跳过)
	if len(records.Result) > 0 {
		return cf.modify(records.Result[0], zoneID, ipAddr, ttl, proxied)
	}
	return cf.create(zoneID, fullName, recordType, ipAddr, ttl, proxied)

}

// 新增
func (cf *Cloudflare) create(zoneID string, fullName string, recordType model.DNSRecordType, ipAddr string, ttl int, proxied bool) error {
	record := CloudflareRecord{
		Name:    fullName,
		Type:    string(recordType),
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
	// 完全相同则跳过
	if record.Content == ipAddr &&
		record.TTL == ttl &&
		record.Proxied == proxied {
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

// cloudflareTXTRecord TXT 记录的创建载荷。
// 刻意不复用 CloudflareRecord：它带 proxied 字段，而 TXT 不可代理，
// Cloudflare 对 TXT 记录带 proxied 会报错。
type cloudflareTXTRecord struct {
	Name    string `json:"name"`
	Type    string `json:"type"`
	Content string `json:"content"`
	TTL     int    `json:"ttl"`
	Comment string `json:"comment,omitempty"`
}

// acmeTXTTTL ACME 挑战记录用最短 TTL，缩短传播与清理时间
const acmeTXTTTL = 60

// AddTXTRecord 追加一条 TXT 记录（ACME DNS-01 用）。
//
// 注意这里是无条件 create，不做「查到就改」——同一个 _acme-challenge 名下
// 可能需要并存多条挑战值（如 example.com 与 *.example.com 同单签发）。
func (cf *Cloudflare) AddTXTRecord(domain string, fqdn string, value string) error {
	zoneID, err := cf.zoneID(domain)
	if err != nil {
		return err
	}

	record := cloudflareTXTRecord{
		Name:    fqdn,
		Type:    "TXT",
		Content: value,
		TTL:     acmeTXTTTL,
		Comment: "LinkStar ACME challenge",
	}

	var status CloudflareStatus
	err = cf.request(
		"POST",
		fmt.Sprintf(zonesAPI+"/%s/dns_records", zoneID),
		record,
		&status,
	)
	if err != nil {
		return fmt.Errorf("新建 TXT 记录失败: %w", err)
	}
	if !status.Success {
		return fmt.Errorf("新建 TXT 记录返回失败: %v", status.Messages)
	}
	return nil
}

// RemoveTXTRecord 按 name + content 精确删除 TXT 记录。
// 按值匹配是为了不误删同名下另一条仍在验证中的挑战记录。
func (cf *Cloudflare) RemoveTXTRecord(domain string, fqdn string, value string) error {
	zoneID, err := cf.zoneID(domain)
	if err != nil {
		return err
	}

	params := url.Values{}
	params.Set("type", "TXT")
	params.Set("name", fqdn)
	params.Set("content", value)
	params.Set("per_page", "50")

	var records CloudflareRecordsResp
	err = cf.request(
		"GET",
		fmt.Sprintf(zonesAPI+"/%s/dns_records?%s", zoneID, params.Encode()),
		nil,
		&records,
	)
	if err != nil {
		return fmt.Errorf("查询 TXT 记录失败: %w", err)
	}
	if !records.Success {
		return fmt.Errorf("查询 TXT 记录返回失败: %v", records.Messages)
	}

	for _, rec := range records.Result {
		// 服务端过滤之外再自查一遍，避免误删
		if rec.Content != value {
			continue
		}
		var status CloudflareStatus
		if err := cf.request(
			"DELETE",
			fmt.Sprintf(zonesAPI+"/%s/dns_records/%s", zoneID, rec.ID),
			nil,
			&status,
		); err != nil {
			return fmt.Errorf("删除 TXT 记录失败: %w", err)
		}
		if !status.Success {
			return fmt.Errorf("删除 TXT 记录返回失败: %v", status.Messages)
		}
	}
	return nil
}

// zoneID 查出主域名对应的 zone ID
func (cf *Cloudflare) zoneID(domain string) (string, error) {
	zones, err := cf.getZones(domain)
	if err != nil {
		return "", fmt.Errorf("查询 zone 失败: %w", err)
	}
	if !zones.Success {
		return "", fmt.Errorf("查询 zone 返回失败: %v", zones.Messages)
	}
	if len(zones.Result) == 0 {
		return "", fmt.Errorf("未找到域名 %s 对应的 zone", domain)
	}
	return zones.Result[0].ID, nil
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
