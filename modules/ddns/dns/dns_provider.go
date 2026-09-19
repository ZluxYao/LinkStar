package dns

import "linkstar/modules/ddns/model"

// DNSProvider 所有DSN服务器实现这个接口

type DNSProvider interface {
	SetRecord(domain string, subDomain string, recordType model.DNSRecordType, ip string, ttl int, proxied bool) error
}

// ACMEDNSProvider 可选接口：服务商额外支持 ACME DNS-01 所需的 TXT 增删。
//
// 刻意不并入 DNSProvider，否则七家服务商都得改。未实现的服务商在类型断言
// 处自然失败，上层据此提示「该服务商暂不支持 DNS-01」。
//
// 语义要求：AddTXTRecord 必须是「追加」而非「覆盖」——签发 example.com 与
// *.example.com 时，两条 challenge 都落在 _acme-challenge.example.com，
// 必须同时存在两条 TXT，覆盖会导致验证失败。
type ACMEDNSProvider interface {
	// AddTXTRecord 在 fqdn 上新增一条值为 value 的 TXT 记录
	AddTXTRecord(domain string, fqdn string, value string) error
	// RemoveTXTRecord 删除 fqdn 上值为 value 的 TXT 记录（按值精确匹配）
	RemoveTXTRecord(domain string, fqdn string, value string) error
}

// RedirectRuleProvider 可选接口：服务商支持在边缘下发重定向规则。
//
// 用途只有一个——CGNAT 下 STUN 给的是随机高位端口，人在外面根本不知道它是多少，
// 而 Cloudflare 橙云只代理 443/80，到不了那个端口。于是让橙云域名发一条 307，
// 把当前端口补在 Location 里。端口一漂，规则就得跟着改。
//
// 和 ACMEDNSProvider 一样刻意不并入 DNSProvider：未实现的服务商在类型断言处
// 自然失败，上层据此提示「该服务商暂不支持入口重定向」。
//
// 语义要求：实现必须是「读-改-写」并逐字保留用户自己写的规则，
// 只认领 ruleKey 对应的那一条。规则名允许在 ruleKey 后面再挂一截给人看的文字，
// 但认领只能看 ruleKey 那一段——否则服务一改名，上一条规则就成了没人认领的孤儿。
type RedirectRuleProvider interface {
	// SyncRedirectRule 让 entryHost 重定向到 targetURL。
	// ruleLabel 是规则名里给人看的那半截（一般是服务名），可以为空，不参与认领。
	// keepPath 表示是否用上了保留原始路径的写法；服务商不支持时降级为 false。
	// entryWarn 非空表示规则写好了，但入口域名那条 DNS 记录没能确认——
	// 规则本身没失败，可少了那条记录访问依旧不通，上层必须把这句话摆到用户面前。
	SyncRedirectRule(zoneDomain, ruleKey, ruleLabel, entryHost, targetURL string) (keepPath bool, entryWarn string, err error)
	// RemoveRedirectRule 删除 ruleKey 对应的规则；规则本来就不存在时返回 nil
	RemoveRedirectRule(zoneDomain, ruleKey string) error
	// InspectEntryRecord 只读地看一眼入口域名那条记录现在什么样，不改任何东西
	InspectEntryRecord(zoneDomain, entryHost string) (EntryRecordState, error)
}

// EntryRecordState 入口域名那条解析记录的现状，给界面照实摆出来用。
//
// 这条记录和 DDNS 那些记录不是一回事：它的内容是个永远不变的占位地址，
// 没人需要维护它。但它在不在、是不是橙云，决定了重定向规则到底执不执行，
// 而这件事从规则那边一点都看不出来——所以才要单独查一次摆在人眼前。
type EntryRecordState struct {
	// Host 入口域名
	Host string `json:"host"`
	// Found 这条记录存在
	Found bool `json:"found"`
	// Type A / AAAA / CNAME
	Type string `json:"type"`
	// Content 记录指向哪
	Content string `json:"content"`
	// Proxied 橙云（已代理）。灰云的话请求根本不经过 Cloudflare，重定向不会执行
	Proxied bool `json:"proxied"`
	// ByLinkStar 这条是 LinkStar 自己建的（注释对得上），不是用户手加的
	ByLinkStar bool `json:"byLinkStar"`
	// WantIP LinkStar 建这条记录时用的占位地址，给界面当「应该长这样」的参照
	WantIP string `json:"wantIP"`
	// Warn 查不了这条记录时的说明（多半是 Token 没有 DNS 权限）
	Warn string `json:"warn"`
}
