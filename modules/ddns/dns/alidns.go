package dns

import (
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base64"
	"fmt"
	"linkstar/modules/ddns/model"
	httpUtils "linkstar/utils/http_utils"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// 阿里云 DNS OpenAPI 入口
// 文档: https://help.aliyun.com/document_detail/29776.html
const alidnsEndpoint = "https://alidns.aliyuncs.com/"

// Alidns 阿里云 DNS
type Alidns struct {
	AccessKeyID     string
	AccessKeySecret string
	httpClient      *http.Client
}

// AlidnsRecord 记录实体
type AlidnsRecord struct {
	DomainName string
	RecordID   string
	Value      string
}

// AlidnsSubDomainRecords 查询子域名记录返回结果
type AlidnsSubDomainRecords struct {
	TotalCount    int
	DomainRecords struct {
		Record []AlidnsRecord
	}
}

// AlidnsResp 新增/更新返回结果
type AlidnsResp struct {
	RecordID  string
	RequestID string
}

// NewAlidns 创建阿里云 DNS 客户端
func NewAlidns(accessKeyID string, accessKeySecret string) *Alidns {
	return &Alidns{
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
func (ali *Alidns) SetRecord(domain string, subDomain string, recordType model.DNSRecordType, ipAddr string, ttl int, proxied bool) error {
	// RR(主机记录):为空或 @ 表示根域名
	rr := subDomain
	if rr == "" {
		rr = "@"
	}
	// 完整域名,用于查询
	fullDomain := rr + "." + domain
	if rr == "@" {
		fullDomain = domain
	}

	// 1. 查询这条记录存不存在
	params := url.Values{}
	params.Set("Action", "DescribeSubDomainRecords")
	params.Set("DomainName", domain)
	params.Set("SubDomain", fullDomain)
	params.Set("Type", string(recordType))

	var records AlidnsSubDomainRecords
	if err := ali.request(params, &records); err != nil {
		return fmt.Errorf("查询记录失败: %w", err)
	}

	// 2. 存在 -> 更新(相同则跳过); 不存在 -> 新增
	if records.TotalCount > 0 {
		return ali.modify(records.DomainRecords.Record[0], rr, recordType, ipAddr, ttl)
	}
	return ali.create(domain, rr, recordType, ipAddr, ttl)
}

// 新增
func (ali *Alidns) create(domain string, rr string, recordType model.DNSRecordType, ipAddr string, ttl int) error {
	params := url.Values{}
	params.Set("Action", "AddDomainRecord")
	params.Set("DomainName", domain)
	params.Set("RR", rr)
	params.Set("Type", string(recordType))
	params.Set("Value", ipAddr)
	params.Set("TTL", strconv.Itoa(ttl))

	var result AlidnsResp
	if err := ali.request(params, &result); err != nil {
		return fmt.Errorf("新增记录失败: %w", err)
	}
	if result.RecordID == "" {
		return fmt.Errorf("新增记录返回失败: RecordId 为空")
	}
	return nil
}

// 修改
func (ali *Alidns) modify(record AlidnsRecord, rr string, recordType model.DNSRecordType, ipAddr string, ttl int) error {
	// 完全相同则跳过
	if record.Value == ipAddr {
		return nil
	}

	params := url.Values{}
	params.Set("Action", "UpdateDomainRecord")
	params.Set("RecordId", record.RecordID)
	params.Set("RR", rr)
	params.Set("Type", string(recordType))
	params.Set("Value", ipAddr)
	params.Set("TTL", strconv.Itoa(ttl))

	var result AlidnsResp
	if err := ali.request(params, &result); err != nil {
		return fmt.Errorf("更新记录失败: %w", err)
	}
	if result.RecordID == "" {
		return fmt.Errorf("更新记录返回失败: RecordId 为空")
	}
	return nil
}

// request 统一请求接口
func (ali *Alidns) request(params url.Values, result interface{}) error {
	method := http.MethodGet
	ali.sign(&params, method)

	req, err := http.NewRequest(method, alidnsEndpoint, nil)
	if err != nil {
		return err
	}
	req.URL.RawQuery = params.Encode()

	resp, err := ali.httpClient.Do(req)
	if err != nil {
		return err
	}
	return httpUtils.GetHTTPResponse(resp, result)
}

// sign 阿里云 POP 签名(HMAC-SHA1),会写入公共参数与 Signature
func (ali *Alidns) sign(params *url.Values, method string) {
	// 公共参数
	params.Set("SignatureMethod", "HMAC-SHA1")
	params.Set("SignatureNonce", strconv.FormatInt(time.Now().UnixNano(), 10))
	params.Set("AccessKeyId", ali.AccessKeyID)
	params.Set("SignatureVersion", "1.0")
	params.Set("Timestamp", time.Now().UTC().Format("2006-01-02T15:04:05Z"))
	params.Set("Format", "JSON")
	params.Set("Version", "2015-01-09")

	// 待签名串: METHOD & 编码("/") & 编码(排序后的参数)
	strToSign := method + "&" + specialURLEncode("/") + "&" + specialURLEncode(params.Encode())

	mac := hmac.New(sha1.New, []byte(ali.AccessKeySecret+"&"))
	mac.Write([]byte(strToSign))
	params.Set("Signature", base64.StdEncoding.EncodeToString(mac.Sum(nil)))
}

// specialURLEncode 阿里云签名要求的特殊编码:
// 对已编码串再次编码 & = / *,空格(+)转 %20,%7E 还原为 ~
func specialURLEncode(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); {
		ch := s[i]
		switch ch {
		case '%':
			if i+3 <= len(s) && s[i:i+3] == "%7E" {
				b.WriteByte('~')
				i += 3
				continue
			}
			fmt.Fprintf(&b, "%%%02X", ch)
		case '*', '/', '&', '=':
			fmt.Fprintf(&b, "%%%02X", ch)
		case '+':
			b.WriteString("%20")
		default:
			b.WriteByte(ch)
		}
		i++
	}
	return b.String()
}
