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
// 只认领 description 等于 ruleKey 的那一条。
type RedirectRuleProvider interface {
	// SyncRedirectRule 让 entryHost 重定向到 targetURL。
	// keepPath 表示是否用上了保留原始路径的写法；服务商不支持时降级为 false。
	SyncRedirectRule(zoneDomain, ruleKey, entryHost, targetURL string) (keepPath bool, err error)
	// RemoveRedirectRule 删除 ruleKey 对应的规则；规则本来就不存在时返回 nil
	RemoveRedirectRule(zoneDomain, ruleKey string) error
}
