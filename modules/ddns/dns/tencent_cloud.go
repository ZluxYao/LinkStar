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
	"strconv"
	"strings"
	"time"
)

// 腾讯云 DNSPod API 3.0 入口
// 文档: https://cloud.tencent.com/document/api/1427/56193
const (
	tencentCloudEndpoint = "https://dnspod.tencentcloudapi.com"
	tencentCloudHost     = "dnspod.tencentcloudapi.com"
	tencentCloudService  = "dnspod"
	tencentCloudVersion  = "2021-03-23"
)

// TencentCloud 腾讯云 DNSPod
type TencentCloud struct {
	SecretID   string
	SecretKey  string
	httpClient *http.Client
}

// TencentCloudRecord 腾讯云记录
type TencentCloudRecord struct {
	Domain string `json:"Domain"`
	// DescribeRecordList 用 SubDomain
	SubDomain string `json:"SubDomain,omitempty"`
	// CreateRecord/ModifyRecord 用 Subdomain
	Subdomain  string `json:"Subdomain,omitempty"`
	RecordType string `json:"RecordType"`
	RecordLine string `json:"RecordLine"`
	Value      string `json:"Value,omitempty"`
	RecordId   int64  `json:"RecordId,omitempty"`
	TTL        int    `json:"TTL,omitempty"`
}

// TencentCloudRecordListsResp 解析记录列表返回结果
type TencentCloudRecordListsResp struct {
	Response struct {
		Error struct {
			Code    string
			Message string
		}
		RecordCountInfo struct {
			TotalCount int `json:"TotalCount"`
		} `json:"RecordCountInfo"`
		RecordList []TencentCloudRecord `json:"RecordList"`
	}
}

// TencentCloudStatus 腾讯云返回状态
type TencentCloudStatus struct {
	Response struct {
		Error struct {
			Code    string
			Message string
		}
	}
}

// NewTencentCloud 创建腾讯云 DNSPod 客户端
func NewTencentCloud(secretID string, secretKey string) *TencentCloud {
	return &TencentCloud{
		SecretID:  secretID,
		SecretKey: secretKey,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
			Transport: &http.Transport{
				Proxy: nil,
			},
		},
	}
}

// SetRecord 设置一条记录
func (tc *TencentCloud) SetRecord(domain string, subDomain string, recordType model.DNSRecordType, ipAddr string, ttl int, proxied bool) error {
	// 主机记录:为空表示根域名 @
	rr := subDomain
	if rr == "" {
		rr = "@"
	}

	// 1. 查询这条记录存不存在
	list := TencentCloudRecord{
		Domain:     domain,
		SubDomain:  rr,
		RecordType: string(recordType),
		RecordLine: "默认",
	}
	var records TencentCloudRecordListsResp
	if err := tc.request("DescribeRecordList", list, &records); err != nil {
		return fmt.Errorf("查询记录失败: %w", err)
	}
	// 记录不存在时腾讯云会返回 ResourceNotFound.NoDataOfRecord,视为需要新增
	if records.Response.Error.Code != "" && records.Response.Error.Code != "ResourceNotFound.NoDataOfRecord" {
		return fmt.Errorf("查询记录返回失败: %s", records.Response.Error.Message)
	}

	// 2. 存在 -> 更新(相同则跳过); 不存在 -> 新增
	if records.Response.RecordCountInfo.TotalCount > 0 {
		return tc.modify(records.Response.RecordList[0], domain, rr, recordType, ipAddr, ttl)
	}
	return tc.create(domain, rr, recordType, ipAddr, ttl)
}

// 新增
func (tc *TencentCloud) create(domain string, rr string, recordType model.DNSRecordType, ipAddr string, ttl int) error {
	record := TencentCloudRecord{
		Domain:     domain,
		Subdomain:  rr,
		RecordType: string(recordType),
		RecordLine: "默认",
		Value:      ipAddr,
		TTL:        ttl,
	}
	var status TencentCloudStatus
	if err := tc.request("CreateRecord", record, &status); err != nil {
		return fmt.Errorf("新增记录失败: %w", err)
	}
	if status.Response.Error.Code != "" {
		return fmt.Errorf("新增记录返回失败: %s", status.Response.Error.Message)
	}
	return nil
}

// 修改
func (tc *TencentCloud) modify(record TencentCloudRecord, domain string, rr string, recordType model.DNSRecordType, ipAddr string, ttl int) error {
	// 完全相同则跳过
	if record.Value == ipAddr {
		return nil
	}
	req := TencentCloudRecord{
		Domain:     domain,
		Subdomain:  rr,
		RecordType: string(recordType),
		RecordLine: "默认",
		Value:      ipAddr,
		RecordId:   record.RecordId,
		TTL:        ttl,
	}
	var status TencentCloudStatus
	if err := tc.request("ModifyRecord", req, &status); err != nil {
		return fmt.Errorf("更新记录失败: %w", err)
	}
	if status.Response.Error.Code != "" {
		return fmt.Errorf("更新记录返回失败: %s", status.Response.Error.Message)
	}
	return nil
}

// request 统一请求接口
func (tc *TencentCloud) request(action string, data interface{}, result interface{}) error {
	jsonStr := make([]byte, 0)
	if data != nil {
		b, err := json.Marshal(data)
		if err != nil {
			return err
		}
		jsonStr = b
	}

	req, err := http.NewRequest(http.MethodPost, tencentCloudEndpoint, bytes.NewBuffer(jsonStr))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-TC-Version", tencentCloudVersion)
	tc.sign(req, action, string(jsonStr))

	resp, err := tc.httpClient.Do(req)
	if err != nil {
		return err
	}
	return httpUtils.GetHTTPResponse(resp, result)
}

// sign 腾讯云 API 3.0 签名(TC3-HMAC-SHA256)
// https://cloud.tencent.com/document/api/1427/56189
func (tc *TencentCloud) sign(req *http.Request, action string, payload string) {
	const algorithm = "TC3-HMAC-SHA256"
	timestamp := time.Now().Unix()
	timestampStr := strconv.FormatInt(timestamp, 10)

	// step 1: 拼接规范请求串
	canonicalHeaders := "content-type:application/json\nhost:" + tencentCloudHost +
		"\nx-tc-action:" + strings.ToLower(action) + "\n"
	signedHeaders := "content-type;host;x-tc-action"
	hashedPayload := sha256Hex(payload)
	canonicalRequest := "POST\n/\n\n" + canonicalHeaders + "\n" + signedHeaders + "\n" + hashedPayload

	// step 2: 拼接待签名字符串
	date := time.Unix(timestamp, 0).UTC().Format("2006-01-02")
	credentialScope := date + "/" + tencentCloudService + "/tc3_request"
	string2sign := algorithm + "\n" + timestampStr + "\n" + credentialScope + "\n" + sha256Hex(canonicalRequest)

	// step 3: 计算签名
	secretDate := hmacSHA256(date, "TC3"+tc.SecretKey)
	secretService := hmacSHA256(tencentCloudService, secretDate)
	secretSigning := hmacSHA256("tc3_request", secretService)
	signature := hex.EncodeToString([]byte(hmacSHA256(string2sign, secretSigning)))

	// step 4: 拼接 Authorization
	authorization := algorithm + " Credential=" + tc.SecretID + "/" + credentialScope +
		", SignedHeaders=" + signedHeaders + ", Signature=" + signature

	req.Header.Set("Authorization", authorization)
	req.Header.Set("Host", tencentCloudHost)
	req.Header.Set("X-TC-Action", action)
	req.Header.Set("X-TC-Timestamp", timestampStr)
}

func sha256Hex(s string) string {
	b := sha256.Sum256([]byte(s))
	return hex.EncodeToString(b[:])
}

func hmacSHA256(s string, key string) string {
	h := hmac.New(sha256.New, []byte(key))
	h.Write([]byte(s))
	return string(h.Sum(nil))
}
