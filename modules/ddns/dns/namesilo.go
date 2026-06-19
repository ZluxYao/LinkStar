package dns

import (
	"encoding/xml"
	"fmt"
	"io"
	"linkstar/modules/ddns/model"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// NameSilo DNS
// 文档: https://www.namesilo.com/api-reference
const namesiloEndpoint = "https://www.namesilo.com/api"

// NameSilo NameSilo
type NameSilo struct {
	APIKey     string
	httpClient *http.Client
}

// NameSiloListResp dnsListRecords 返回结果
type NameSiloListResp struct {
	XMLName xml.Name `xml:"namesilo"`
	Reply   struct {
		Code          int              `xml:"code"`
		Detail        string           `xml:"detail"`
		ResourceItems []NameSiloRecord `xml:"resource_record"`
	} `xml:"reply"`
}

// NameSiloRecord 单条记录
type NameSiloRecord struct {
	RecordID string `xml:"record_id"`
	Type     string `xml:"type"`
	Host     string `xml:"host"`
	Value    string `xml:"value"`
	TTL      int    `xml:"ttl"`
}

// NameSiloModifyResp dnsAddRecord/dnsUpdateRecord 返回结果
type NameSiloModifyResp struct {
	XMLName xml.Name `xml:"namesilo"`
	Reply   struct {
		Code     int    `xml:"code"`
		Detail   string `xml:"detail"`
		RecordID string `xml:"record_id"`
	} `xml:"reply"`
}

// NewNameSilo 创建 NameSilo 客户端
func NewNameSilo(apiKey string) *NameSilo {
	return &NameSilo{
		APIKey: apiKey,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
			Transport: &http.Transport{
				Proxy: nil,
			},
		},
	}
}

// SetRecord 设置一条记录
func (ns *NameSilo) SetRecord(domain string, subDomain string, recordType model.DNSRecordType, ipAddr string, ttl int, proxied bool) error {
	host := subDomain
	if host == "" {
		host = "@"
	}

	// 1. 查询记录列表
	var list NameSiloListResp
	listParams := url.Values{}
	listParams.Set("version", "1")
	listParams.Set("type", "xml")
	listParams.Set("key", ns.APIKey)
	listParams.Set("domain", domain)
	if err := ns.request("dnsListRecords", listParams, &list); err != nil {
		return fmt.Errorf("查询记录失败: %w", err)
	}
	if list.Reply.Code != 300 {
		return fmt.Errorf("查询记录返回失败: %s", list.Reply.Detail)
	}

	// 2. 匹配 host + type
	var found *NameSiloRecord
	for i := range list.Reply.ResourceItems {
		r := &list.Reply.ResourceItems[i]
		// NameSilo 返回的 host 是完整域名(子域名.主域名 或 主域名)
		if r.Type == string(recordType) && (r.Host == fullHost(host, domain) || r.Host == domain && host == "@") {
			found = r
			break
		}
	}

	// 3. 存在 -> 更新(相同则跳过); 不存在 -> 新增
	if found != nil {
		if found.Value == ipAddr {
			return nil
		}
		return ns.update(domain, host, found.RecordID, ipAddr, ttl)
	}
	return ns.create(domain, host, recordType, ipAddr, ttl)
}

// 新增
func (ns *NameSilo) create(domain string, host string, recordType model.DNSRecordType, ipAddr string, ttl int) error {
	params := url.Values{}
	params.Set("version", "1")
	params.Set("type", "xml")
	params.Set("key", ns.APIKey)
	params.Set("domain", domain)
	params.Set("rrhost", host)
	params.Set("rrtype", string(recordType))
	params.Set("rrvalue", ipAddr)
	params.Set("rrttl", strconv.Itoa(ttl))

	var result NameSiloModifyResp
	if err := ns.request("dnsAddRecord", params, &result); err != nil {
		return fmt.Errorf("新增记录失败: %w", err)
	}
	if result.Reply.Code != 300 {
		return fmt.Errorf("新增记录返回失败: %s", result.Reply.Detail)
	}
	return nil
}

// 修改
func (ns *NameSilo) update(domain string, host string, recordID string, ipAddr string, ttl int) error {
	params := url.Values{}
	params.Set("version", "1")
	params.Set("type", "xml")
	params.Set("key", ns.APIKey)
	params.Set("domain", domain)
	params.Set("rrid", recordID)
	params.Set("rrhost", host)
	params.Set("rrvalue", ipAddr)
	params.Set("rrttl", strconv.Itoa(ttl))

	var result NameSiloModifyResp
	if err := ns.request("dnsUpdateRecord", params, &result); err != nil {
		return fmt.Errorf("更新记录失败: %w", err)
	}
	if result.Reply.Code != 300 {
		return fmt.Errorf("更新记录返回失败: %s", result.Reply.Detail)
	}
	return nil
}

// request 统一请求接口(GET,XML 响应)
func (ns *NameSilo) request(action string, params url.Values, result interface{}) error {
	apiURL := namesiloEndpoint + "/" + action + "?" + params.Encode()
	req, err := http.NewRequest(http.MethodGet, apiURL, http.NoBody)
	if err != nil {
		return err
	}

	resp, err := ns.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	return xml.Unmarshal(data, result)
}

// fullHost 拼接 NameSilo 返回的完整 host
func fullHost(host string, domain string) string {
	if host == "@" {
		return domain
	}
	return host + "." + domain
}
