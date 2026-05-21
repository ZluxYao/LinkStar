package cf

// ============ Cloudflare API 返回结构 ============
//
// 只声明本模块用到的字段，其它字段 json 反序列化时会自动忽略
// 字段含义参考官方文档：https://developers.cloudflare.com/api/operations/dns-records-for-a-zone-list-dns-records

// Record 一条 DNS 记录
type Record struct {
	ID      string `json:"id"`      // 记录唯一 ID（更新时需要用）
	Type    string `json:"type"`    // A / AAAA / CNAME 等
	Name    string `json:"name"`    // 完整域名，如 home.example.com
	Content string `json:"content"` // 当前值，A 记录就是 IPv4 字符串
	TTL     int    `json:"ttl"`     // 缓存时间，1=auto
	Proxied bool   `json:"proxied"` // 是否走 CF 橙色云朵代理
}

// listResp GET /zones/{zone_id}/dns_records 的返回结构
type listResp struct {
	Result  []Record `json:"result"`
	Success bool     `json:"success"`
	Errors  []apiErr `json:"errors"`
}

// zone 一个 CF zone 的简化字段
type zone struct {
	ID   string `json:"id"`
	Name string `json:"name"` // 如 "example.com"
}

// zoneListResp GET /zones?name=xxx 的返回结构
type zoneListResp struct {
	Result  []zone   `json:"result"`
	Success bool     `json:"success"`
	Errors  []apiErr `json:"errors"`
}

// mutateResp POST/PUT /zones/.../dns_records 的返回结构
type mutateResp struct {
	Result  Record   `json:"result"`
	Success bool     `json:"success"`
	Errors  []apiErr `json:"errors"`
}

// apiErr CF API 错误格式
type apiErr struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// ============ 业务层结果 ============

// Action 描述一次 UpdateRecord 的实际动作
type Action string

const (
	ActionSkipped Action = "skipped" // IP 没变，不需要操作
	ActionUpdated Action = "updated" // 改了已有记录的 IP
	ActionCreated Action = "created" // 之前没记录，新建了一条
)

// Result UpdateRecord 的返回结果
type Result struct {
	Action   Action // 实际做了啥
	RecordID string // 操作后的记录 ID
	PrevIP   string // 操作前 CF 上的 IP（Created 时为空）
	NewIP    string // 操作后 CF 上的 IP
}
