package dns

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"linkstar/modules/ddns/model"
	httpUtils "linkstar/utils/http_utils"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

// 华为云 DNS
// 文档: https://support.huaweicloud.com/api-dns/dns_api_64001.html
const (
	huaweiEndpoint   = "https://dns.myhuaweicloud.com"
	huaweiDateFormat = "20060102T150405Z"
	huaweiAlgorithm  = "SDK-HMAC-SHA256"
)

// Huaweicloud 华为云 DNS
type Huaweicloud struct {
	AccessKeyID     string
	AccessKeySecret string
	httpClient      *http.Client
}

// HuaweicloudZonesResp zones 返回结果
type HuaweicloudZonesResp struct {
	Zones []struct {
		ID   string
		Name string
	}
}

// HuaweicloudRecordsResp 记录返回结果
type HuaweicloudRecordsResp struct {
	Recordsets []HuaweicloudRecordsets
}

// HuaweicloudRecordsets 记录
type HuaweicloudRecordsets struct {
	ID      string
	Name    string `json:"name"`
	ZoneID  string `json:"zone_id"`
	Status  string
	Type    string   `json:"type"`
	TTL     int      `json:"ttl"`
	Records []string `json:"records"`
	Weight  int      `json:"weight"`
}

// NewHuaweicloud 创建华为云 DNS 客户端
func NewHuaweicloud(accessKeyID string, accessKeySecret string) *Huaweicloud {
	return &Huaweicloud{
		AccessKeyID:     accessKeyID,
		AccessKeySecret: accessKeySecret,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
			Transport: &http.Transport{
				Proxy: nil,
			},
		},
	}
}

// SetRecord 设置一条记录
func (hw *Huaweicloud) SetRecord(domain string, subDomain string, recordType model.DNSRecordType, ipAddr string, ttl int, proxied bool) error {
	// 完整域名:子域名为空或 @ 即主域名本身
	fullName := subDomain + "." + domain
	if subDomain == "" || subDomain == "@" {
		fullName = domain
	}

	// 1. 查询记录(华为云默认模糊匹配,需按 name 精确比对)
	params := url.Values{}
	params.Set("name", fullName)
	params.Set("type", string(recordType))

	var records HuaweicloudRecordsResp
	if err := hw.request("GET", huaweiEndpoint+"/v2.1/recordsets", params, &records); err != nil {
		return fmt.Errorf("查询记录失败: %w", err)
	}

	for _, record := range records.Recordsets {
		if record.Name == fullName+"." {
			return hw.modify(record, ipAddr, ttl)
		}
	}
	return hw.create(domain, fullName, recordType, ipAddr, ttl)
}

// 新增
func (hw *Huaweicloud) create(domain string, fullName string, recordType model.DNSRecordType, ipAddr string, ttl int) error {
	// 先查 zone 拿到 zoneID
	var zones HuaweicloudZonesResp
	if err := hw.request("GET", huaweiEndpoint+"/v2/zones", url.Values{"name": []string{domain}}, &zones); err != nil {
		return fmt.Errorf("查询 zone 失败: %w", err)
	}
	if len(zones.Zones) == 0 {
		return fmt.Errorf("未找到域名 %s 对应的 zone", domain)
	}
	zoneID := zones.Zones[0].ID
	for _, z := range zones.Zones {
		if z.Name == domain+"." {
			zoneID = z.ID
			break
		}
	}

	record := HuaweicloudRecordsets{
		Type:    string(recordType),
		Name:    fullName + ".",
		Records: []string{ipAddr},
		TTL:     ttl,
		Weight:  1,
	}
	var result HuaweicloudRecordsets
	if err := hw.request("POST", fmt.Sprintf(huaweiEndpoint+"/v2.1/zones/%s/recordsets", zoneID), record, &result); err != nil {
		return fmt.Errorf("新增记录失败: %w", err)
	}
	if len(result.Records) == 0 || result.Records[0] != ipAddr {
		return fmt.Errorf("新增记录返回失败: %s", result.Status)
	}
	return nil
}

// 修改
func (hw *Huaweicloud) modify(record HuaweicloudRecordsets, ipAddr string, ttl int) error {
	// 完全相同则跳过
	if len(record.Records) > 0 && record.Records[0] == ipAddr {
		return nil
	}
	body := map[string]interface{}{
		"name":    record.Name,
		"type":    record.Type,
		"records": []string{ipAddr},
		"ttl":     ttl,
	}
	var result HuaweicloudRecordsets
	if err := hw.request("PUT", fmt.Sprintf(huaweiEndpoint+"/v2.1/zones/%s/recordsets/%s", record.ZoneID, record.ID), body, &result); err != nil {
		return fmt.Errorf("更新记录失败: %w", err)
	}
	if len(result.Records) == 0 || result.Records[0] != ipAddr {
		return fmt.Errorf("更新记录返回失败: %s", result.Status)
	}
	return nil
}

// request 统一请求接口
func (hw *Huaweicloud) request(method string, urlStr string, data interface{}, result interface{}) error {
	var body []byte
	var req *http.Request
	var err error

	if method == "GET" {
		req, err = http.NewRequest(method, urlStr, nil)
		if err != nil {
			return err
		}
		if v, ok := data.(url.Values); ok {
			req.URL.RawQuery = v.Encode()
		}
	} else {
		if data != nil {
			body, err = json.Marshal(data)
			if err != nil {
				return err
			}
		}
		req, err = http.NewRequest(method, urlStr, bytes.NewBuffer(body))
		if err != nil {
			return err
		}
	}

	hw.sign(req, body)
	req.Header.Set("content-type", "application/json")

	resp, err := hw.httpClient.Do(req)
	if err != nil {
		return err
	}
	return httpUtils.GetHTTPResponse(resp, result)
}

// sign 华为云 SDK-HMAC-SHA256 签名,签名头仅 x-sdk-date
// https://support.huaweicloud.com/devg-apisign/api-sign-algorithm.html
func (hw *Huaweicloud) sign(req *http.Request, payload []byte) {
	date := time.Now().UTC().Format(huaweiDateFormat)
	req.Header.Set("X-Sdk-Date", date)

	signedHeaders := "x-sdk-date"
	canonicalHeaders := "x-sdk-date:" + date + "\n"

	canonicalRequest := req.Method + "\n" +
		huaweiCanonicalURI(req.URL.Path) + "\n" +
		huaweiCanonicalQuery(req.URL) + "\n" +
		canonicalHeaders + "\n" +
		signedHeaders + "\n" +
		sha256Hex(string(payload))

	stringToSign := huaweiAlgorithm + "\n" + date + "\n" + sha256Hex(canonicalRequest)

	h := hmac.New(sha256.New, []byte(hw.AccessKeySecret))
	h.Write([]byte(stringToSign))
	signature := fmt.Sprintf("%x", h.Sum(nil))

	req.Header.Set("Authorization", fmt.Sprintf("%s Access=%s, SignedHeaders=%s, Signature=%s",
		huaweiAlgorithm, hw.AccessKeyID, signedHeaders, signature))
}

// huaweiCanonicalURI 逐段转义,保证以 / 结尾
func huaweiCanonicalURI(path string) string {
	segs := strings.Split(path, "/")
	for i, v := range segs {
		segs[i] = signEscape(v)
	}
	uri := strings.Join(segs, "/")
	if len(uri) == 0 || uri[len(uri)-1] != '/' {
		uri += "/"
	}
	return uri
}

// huaweiCanonicalQuery 按 key 排序后转义拼接
func huaweiCanonicalQuery(u *url.URL) string {
	query := u.Query()
	keys := make([]string, 0, len(query))
	for k := range query {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var pairs []string
	for _, k := range keys {
		values := query[k]
		sort.Strings(values)
		for _, v := range values {
			pairs = append(pairs, signEscape(k)+"="+signEscape(v))
		}
	}
	return strings.Join(pairs, "&")
}
