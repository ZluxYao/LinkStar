package model

import "time"

// DDSN提供商类型
type DNSProviderType string

const (
	DNSProviderCloudflare   DNSProviderType = "cloudflare"
	DNSProviderAlidns       DNSProviderType = "alidns"
	DNSProviderTencentCloud DNSProviderType = "tencentcloud"
	DNSProviderBaiduCloud   DNSProviderType = "baiducloud"
	DNSProviderHuaweiCloud  DNSProviderType = "huaweicloud"
	DNSProviderNameCheap    DNSProviderType = "namecheap"
	DNSProviderNameSilo     DNSProviderType = "namesilo"
)

// ProviderCredentialKeys 每种服务商类型所需的凭证字段 key。
// 作为后端校验与 BuildClient 取值的唯一依据；前端另有一张展示用字段表。
var ProviderCredentialKeys = map[DNSProviderType][]string{
	DNSProviderCloudflare:   {"apiToken"},
	DNSProviderAlidns:       {"accessKeyId", "accessKeySecret"},
	DNSProviderTencentCloud: {"secretId", "secretKey"},
	DNSProviderBaiduCloud:   {"accessKeyId", "accessKeySecret"},
	DNSProviderHuaweiCloud:  {"accessKeyId", "accessKeySecret"},
	DNSProviderNameCheap:    {"password"},
	DNSProviderNameSilo:     {"apiKey"},
}

// IsValidProviderType 该服务商类型是否受支持
func IsValidProviderType(t DNSProviderType) bool {
	_, ok := ProviderCredentialKeys[t]
	return ok
}

// 定义每个 DNS 记录类型
type DNSRecordType string

const (
	DNSRecordTypeA    DNSRecordType = "A"
	DNSRecordTypeAAAA DNSRecordType = "AAAA"
)

// 定义DDNS 记录状态
type DDNSRecordStatus string

const (
	// 等待执行同步
	DDNSRecordStatusPending DDNSRecordStatus = "pending"
	// 同步成功
	DDNSRecordStatusSuccess DDNSRecordStatus = "success"
	// 同步失败
	DDNSRecordStatusFailed DDNSRecordStatus = "failed"
	// 跳过同步
	DDNSRecordStatusSkipped DDNSRecordStatus = "skipped"
)

type IPSourceType string

const (
	IPSourceSTUN      IPSourceType = "stun"      // 复用 stun.Runtime.Network（PublicIP/LocalIP）
	IPSourceWeb       IPSourceType = "web"       // HTTP GET 某网站，如 https://ip.sb
	IPSourceDNS       IPSourceType = "dns"       // 解析某域名，保持一致
	IPSourceInterface IPSourceType = "interface" // 读本地网卡
)

// DDNS 配置
type DDNSConfig struct {
	IntervalSec int `json:"intervalSec"` // 全局默认同步时间300

	Providers []DDNSProvider `json:"providers"` // DNS 服务商 配置
	Records   []DDNSRecord   `json:"records"`   // DDNS 记录
	CreatedAt time.Time      `json:"createdAt"`
	UpdatedAt time.Time      `json:"updatedAt"`
}

// DNS 服务商
type DDNSProvider struct {
	ID         uint                   `json:"id"`
	Name       string                 `json:"name"` // 自定义名字
	Type       DNSProviderType        `json:"type"` // 服务商类型
	Credential map[string]interface{} `json:"credential"`

	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// DDNS 记录
type DDNSRecord struct {
	Enabled      bool          `json:"enabled"`
	ID           uint          `json:"id"`
	ProviderID   uint          `json:"providerId"`  // 服务商id
	Name         string        `json:"name"`        // DDNS 记录名字
	Domain       string        `json:"domain"`      // 主域名
	SubDomain    string        `json:"subDomain"`   // 子域名
	RecordType   DNSRecordType `json:"recordType"`  // 记录类型
	IPSourceType IPSourceType  `json:"ipSource"`    // ip来源
	IPSourceArg  string        `json:"ipSourceArg"` // web 的 URL / dns 的域名,stun/interface 留空
	TTL          int           `json:"ttl"`         // ttl
	Proxied      bool          `json:"proxied"`     // 小云朵代理

	LastIP      string           `json:"lastIP"`      // 上次成功同步的 IP
	LastStatus  DDNSRecordStatus `json:"lastStatus"`  // 上次同步状态,如 success/failed/skipped
	LastMessage string           `json:"lastMessage"` // 上次同步结果或失败原因
	LastCheckAt time.Time        `json:"lastCheckAt"` // 每次扫到就更新,用于 due 判断
	LastSyncAt  time.Time        `json:"lastSyncAt"`  // 上次同步时间

	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}
