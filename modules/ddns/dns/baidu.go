package dns

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"linkstar/modules/ddns/model"
	httpUtils "linkstar/utils/http_utils"
	"net/http"
	"strings"
	"time"
)

// 百度云 DNS(BCD)
// 文档: https://cloud.baidu.com/doc/BCD/s/4jwvymhs7
const (
	baiduEndpoint       = "https://bcd.baidubce.com"
	baiduHost           = "bcd.baidubce.com"
	baiduDateFormat     = "2006-01-02T15:04:05Z"
	baiduExpirationSecs = "1800"
	baiduListPath       = "/v1/domain/resolve/list"
	baiduAddPath        = "/v1/domain/resolve/add"
	baiduEditPath       = "/v1/domain/resolve/edit"
)

// BaiduCloud 百度云 DNS
type BaiduCloud struct {
	AccessKeyID     string
	AccessKeySecret string
	httpClient      *http.Client
}

// BaiduRecord 单条解析记录
type BaiduRecord struct {
	RecordId uint   `json:"recordId"`
	Domain   string `json:"domain"`
	View     string `json:"view"`
	Rdtype   string `json:"rdtype"`
	TTL      int    `json:"ttl"`
	Rdata    string `json:"rdata"`
	ZoneName string `json:"zoneName"`
	Status   string `json:"status"`
}

// BaiduRecordsResp 解析列表返回结果
type BaiduRecordsResp struct {
	TotalCount int           `json:"totalCount"`
	Result     []BaiduRecord `json:"result"`
}

// BaiduListRequest 查询列表请求体
type BaiduListRequest struct {
	Domain   string `json:"domain"`
	PageNum  int    `json:"pageNum"`
	PageSize int    `json:"pageSize"`
}

// BaiduCreateRequest 创建请求体
type BaiduCreateRequest struct {
	Domain   string `json:"domain"`
	RdType   string `json:"rdType"`
	TTL      int    `json:"ttl"`
	Rdata    string `json:"rdata"`
	ZoneName string `json:"zoneName"`
}

// BaiduModifyRequest 修改请求体
type BaiduModifyRequest struct {
	RecordId uint   `json:"recordId"`
	Domain   string `json:"domain"`
	View     string `json:"view"`
	RdType   string `json:"rdType"`
	TTL      int    `json:"ttl"`
	Rdata    string `json:"rdata"`
	ZoneName string `json:"zoneName"`
}

// NewBaiduCloud 创建百度云 DNS 客户端
func NewBaiduCloud(accessKeyID string, accessKeySecret string) *BaiduCloud {
	return &BaiduCloud{
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
func (bd *BaiduCloud) SetRecord(domain string, subDomain string, recordType model.DNSRecordType, ipAddr string, ttl int, proxied bool) error {
	rr := subDomain
	if rr == "" {
		rr = "@"
	}

	// 1. 查询记录列表
	var records BaiduRecordsResp
	listReq := BaiduListRequest{Domain: domain, PageNum: 1, PageSize: 1000}
	if err := bd.request(baiduListPath, listReq, &records); err != nil {
		return fmt.Errorf("查询记录失败: %w", err)
	}

	// 2. 子域名匹配 -> 更新(相同则跳过); 不存在 -> 新增
	for _, record := range records.Result {
		if record.Domain == rr && record.Rdtype == string(recordType) {
			return bd.modify(record, recordType, ipAddr)
		}
	}
	return bd.create(domain, rr, recordType, ipAddr, ttl)
}

// 新增
func (bd *BaiduCloud) create(domain string, rr string, recordType model.DNSRecordType, ipAddr string, ttl int) error {
	req := BaiduCreateRequest{
		Domain:   rr,
		RdType:   string(recordType),
		TTL:      ttl,
		Rdata:    ipAddr,
		ZoneName: domain,
	}
	var result BaiduRecordsResp
	if err := bd.request(baiduAddPath, req, &result); err != nil {
		return fmt.Errorf("新增记录失败: %w", err)
	}
	return nil
}

// 修改
func (bd *BaiduCloud) modify(record BaiduRecord, recordType model.DNSRecordType, ipAddr string) error {
	// 完全相同则跳过
	if record.Rdata == ipAddr {
		return nil
	}
	req := BaiduModifyRequest{
		RecordId: record.RecordId,
		Domain:   record.Domain,
		View:     record.View,
		RdType:   string(recordType),
		TTL:      record.TTL,
		Rdata:    ipAddr,
		ZoneName: record.ZoneName,
	}
	var result BaiduRecordsResp
	if err := bd.request(baiduEditPath, req, &result); err != nil {
		return fmt.Errorf("更新记录失败: %w", err)
	}
	return nil
}

// request 统一请求接口
func (bd *BaiduCloud) request(path string, data interface{}, result interface{}) error {
	jsonStr := make([]byte, 0)
	if data != nil {
		b, err := json.Marshal(data)
		if err != nil {
			return err
		}
		jsonStr = b
	}

	req, err := http.NewRequest(http.MethodPost, baiduEndpoint+path, bytes.NewBuffer(jsonStr))
	if err != nil {
		return err
	}
	req.Header.Set("content-type", "application/json")
	req.Header.Set("Authorization", bd.sign(http.MethodPost, path))

	resp, err := bd.httpClient.Do(req)
	if err != nil {
		return err
	}
	return httpUtils.GetHTTPResponse(resp, result)
}

// sign 百度云 BCE 签名,返回 Authorization 头
// https://cloud.baidu.com/doc/Reference/s/Njwvz1wot
func (bd *BaiduCloud) sign(method string, path string) string {
	// authStringPrefix: bce-auth-v1/{ak}/{timestamp}/{expire}
	authStringPrefix := "bce-auth-v1/" + bd.AccessKeyID + "/" +
		time.Now().UTC().Format(baiduDateFormat) + "/" + baiduExpirationSecs

	// CanonicalURI: 对每个路径段做 RFC3986 转义
	canonicalURI := baiduCanonicalURI(path)

	// CanonicalRequest = Method\nURI\nQuery\nHeaders
	// 仅调用固定 POST 接口,Query 为空,Headers 写死 host
	canonicalReq := method + "\n" + canonicalURI + "\n\n" + "host:" + baiduHost

	signingKey := hmacSHA256Hex(bd.AccessKeySecret, authStringPrefix)
	signature := hmacSHA256Hex(signingKey, canonicalReq)

	return authStringPrefix + "/host/" + signature
}

// baiduCanonicalURI 逐段转义路径,保留 '/' 分隔符
func baiduCanonicalURI(path string) string {
	segs := strings.Split(path, "/")
	for i, v := range segs {
		segs[i] = signEscape(v)
	}
	return strings.Join(segs, "/")
}

func hmacSHA256Hex(secret string, message string) string {
	h := hmac.New(sha256.New, []byte(secret))
	h.Write([]byte(message))
	return hex.EncodeToString(h.Sum(nil))
}
