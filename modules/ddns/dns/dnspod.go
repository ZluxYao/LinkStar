package dns

import (
	"fmt"
	"linkstar/modules/ddns/model"
	httpUtils "linkstar/utils/http_utils"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// DNSPod 老版 API(dnsapi.cn),使用 login_token 认证
// 文档: https://cloud.tencent.com/document/api/302/8516
const (
	dnspodRecordListAPI   = "https://dnsapi.cn/Record.List"
	dnspodRecordCreateAPI = "https://dnsapi.cn/Record.Create"
	dnspodRecordModifyAPI = "https://dnsapi.cn/Record.Modify"
)

// Dnspod DNSPod 老版
type Dnspod struct {
	ID         string // API Token 的 ID
	Token      string // API Token
	httpClient *http.Client
}

// DnspodRecord 记录实体
type DnspodRecord struct {
	ID    string
	Name  string
	Type  string
	Value string
}

// DnspodRecordListResp Record.List 返回结果
type DnspodRecordListResp struct {
	DnspodStatus
	Records []DnspodRecord
}

// DnspodStatus 公共状态
type DnspodStatus struct {
	Status struct {
		Code    string
		Message string
	}
}

// NewDnspod 创建 DNSPod(老版)客户端
func NewDnspod(id string, token string) *Dnspod {
	return &Dnspod{
		ID:    id,
		Token: token,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
			Transport: &http.Transport{
				Proxy: nil,
			},
		},
	}
}

// SetRecord 设置一条记录
func (dp *Dnspod) SetRecord(domain string, subDomain string, recordType model.DNSRecordType, ipAddr string, ttl int, proxied bool) error {
	rr := subDomain
	if rr == "" {
		rr = "@"
	}

	// 1. 查询记录列表
	listParams := url.Values{}
	listParams.Set("login_token", dp.ID+","+dp.Token)
	listParams.Set("domain", domain)
	listParams.Set("record_type", string(recordType))
	listParams.Set("sub_domain", rr)
	listParams.Set("format", "json")

	var list DnspodRecordListResp
	if err := dp.request(dnspodRecordListAPI, listParams, &list); err != nil {
		return fmt.Errorf("查询记录失败: %w", err)
	}
	// 记录不存在时 Code 为 10(无记录),其余非 1 视为错误
	if list.Status.Code != "1" && list.Status.Code != "10" {
		return fmt.Errorf("查询记录返回失败: %s", list.Status.Message)
	}

	// 2. 存在 -> 更新(相同则跳过); 不存在 -> 新增
	if len(list.Records) > 0 {
		return dp.modify(list.Records[0], domain, rr, recordType, ipAddr, ttl)
	}
	return dp.create(domain, rr, recordType, ipAddr, ttl)
}

// 新增
func (dp *Dnspod) create(domain string, rr string, recordType model.DNSRecordType, ipAddr string, ttl int) error {
	params := url.Values{}
	params.Set("login_token", dp.ID+","+dp.Token)
	params.Set("domain", domain)
	params.Set("sub_domain", rr)
	params.Set("record_type", string(recordType))
	params.Set("record_line", "默认")
	params.Set("value", ipAddr)
	params.Set("ttl", strconv.Itoa(ttl))
	params.Set("format", "json")

	var status DnspodStatus
	if err := dp.request(dnspodRecordCreateAPI, params, &status); err != nil {
		return fmt.Errorf("新增记录失败: %w", err)
	}
	if status.Status.Code != "1" {
		return fmt.Errorf("新增记录返回失败: %s", status.Status.Message)
	}
	return nil
}

// 修改
func (dp *Dnspod) modify(record DnspodRecord, domain string, rr string, recordType model.DNSRecordType, ipAddr string, ttl int) error {
	// 完全相同则跳过
	if record.Value == ipAddr {
		return nil
	}
	params := url.Values{}
	params.Set("login_token", dp.ID+","+dp.Token)
	params.Set("domain", domain)
	params.Set("sub_domain", rr)
	params.Set("record_type", string(recordType))
	params.Set("record_line", "默认")
	params.Set("value", ipAddr)
	params.Set("ttl", strconv.Itoa(ttl))
	params.Set("format", "json")
	params.Set("record_id", record.ID)

	var status DnspodStatus
	if err := dp.request(dnspodRecordModifyAPI, params, &status); err != nil {
		return fmt.Errorf("更新记录失败: %w", err)
	}
	if status.Status.Code != "1" {
		return fmt.Errorf("更新记录返回失败: %s", status.Status.Message)
	}
	return nil
}

// request 统一请求接口(POST form)
func (dp *Dnspod) request(apiAddr string, values url.Values, result interface{}) error {
	resp, err := dp.httpClient.PostForm(apiAddr, values)
	if err != nil {
		return err
	}
	return httpUtils.GetHTTPResponse(resp, result)
}
