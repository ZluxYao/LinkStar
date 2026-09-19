package ddns

import (
	"fmt"
	"net"
	"strings"

	"linkstar/modules/ddns/model"
)

// EnsureLandingRecord 给落地域名补一条 DDNS 记录，让它自己跟着公网 IP 走。
//
// 入口重定向只管「入口域名 → 落地域名:端口」这一跳；落地域名解析到哪台机器，
// 规则管不着。家宽的公网 IP 说变就变，没人维护的话规则依旧显示「同步成功」，
// 只是把所有人送到一个早就换了主人的旧 IP——两头都不报错，只有访问的人知道打不开。
//
// 已经有记录管着这个域名就原样不动：用户自己配的那条，比这里猜出来的可信。
func (r *DDNSRuntime) EnsureLandingRecord(providerID uint, zoneDomain, host string) (created bool, err error) {
	host = strings.TrimSuffix(strings.TrimSpace(host), ".")
	if host == "" {
		return false, nil
	}
	if net.ParseIP(host) != nil {
		return false, nil // 服务没填域名，对外就是裸 IP，没有域名要维护
	}
	if providerID == 0 {
		return false, nil // 上层会先报「请先选一个 DNS 服务商」，这里不重复
	}

	zone := strings.TrimSuffix(strings.TrimSpace(zoneDomain), ".")
	sub, inZone := splitZone(host, zone)
	if !inZone {
		// 入口域名和落地域名可以分属两个主域名，这条记录归落地域名自己那个
		zone = guessZone(host)
		sub, inZone = splitZone(host, zone)
	}
	if !inZone {
		return false, fmt.Errorf(
			"看不出落地域名 %s 的主域名是哪个，去 DDNS 里手动加一条指向它的 A 记录", host)
	}

	if _, ok := r.FindRecordByHost(host); ok {
		return false, nil // 已经有人管着这个域名了
	}

	_, err = r.AddRecord(RecordInput{
		Enabled:      true,
		ProviderID:   providerID,
		Name:         host + " 外网地址",
		Domain:       zone,
		SubDomain:    sub,
		RecordType:   model.DNSRecordTypeA,
		IPSourceType: model.IPSourceSTUN,
		TTL:          1,
		// 小黄云必须关着：Cloudflare 的代理只转发 80/443 那几个端口，
		// 而这里对外的是 28262 这种打洞打出来的端口，开了就连不上。
		Proxied: false,
		// 打上记号，服务删掉时才知道这条是自己补的、可以跟着删
		AutoCreated: true,
	})
	if err != nil {
		return false, fmt.Errorf("自动添加落地域名解析记录失败: %w", err)
	}
	return true, nil
}

// ReleaseLandingRecord 服务没了，把当初替它补的那条记录也收回去。
//
// 不收的话，DDNS 列表里会一直躺着一条谁也不认识的记录，每轮照常往服务商那边
// 推 IP，维护着一个早就没有服务在听的域名。用户既不知道它是谁加的，
// 也不知道能不能删。
//
// 只删自己加的那条：用户手动建的，或者建完又被他改过的（UpdateRecord 会把
// AutoCreated 清掉），一律不动——同一个域名他可能还拿来干别的。
func (r *DDNSRuntime) ReleaseLandingRecord(host string) (removed bool, err error) {
	rec, ok := r.FindRecordByHost(host)
	if !ok || !rec.AutoCreated {
		return false, nil
	}
	if err := r.DeleteRecord(rec.ID); err != nil {
		return false, err
	}
	return true, nil
}

// FindRecordByHost 哪条 DDNS 记录管着这个域名的 A 记录。
//
// 「有没有人管」这件事有两处要用：建之前得知道别重复建，界面上也得把
// 「这个域名归谁维护、上次同步成功没有」摆出来。两处用同一套匹配，
// 免得一边认为有、另一边认为没有。
func (r *DDNSRuntime) FindRecordByHost(host string) (model.DDNSRecord, bool) {
	host = strings.TrimSuffix(strings.TrimSpace(host), ".")
	if host == "" {
		return model.DDNSRecord{}, false
	}
	for _, rec := range r.Snapshot().Records {
		if rec.RecordType != model.DNSRecordTypeA {
			continue
		}
		if strings.EqualFold(recordFQDN(rec.Domain, rec.SubDomain), host) {
			return rec, true
		}
	}
	return model.DDNSRecord{}, false
}

// guessZone 主域名没填时取后两段：a.b.example.com → example.com。
// 猜不对的（example.co.uk 这种）在配置里把主域名填上。
func guessZone(host string) string {
	parts := strings.Split(strings.Trim(host, "."), ".")
	if len(parts) < 2 {
		return host
	}
	return strings.Join(parts[len(parts)-2:], ".")
}

// splitZone 把 host 拆成「主域名下的那一截」。
// host 就是主域名本身时返回 "@"；不在这个主域名下面时返回 false。
func splitZone(host, zone string) (string, bool) {
	if strings.EqualFold(host, zone) {
		return "@", true
	}
	suffix := "." + zone
	if len(host) > len(suffix) && strings.EqualFold(host[len(host)-len(suffix):], suffix) {
		return host[:len(host)-len(suffix)], true
	}
	return "", false
}

// recordFQDN 把一条记录的主域名 + 子域名拼回完整域名，拼法和各服务商 SetRecord 一致
func recordFQDN(domain, sub string) string {
	domain = strings.TrimSuffix(strings.TrimSpace(domain), ".")
	sub = strings.TrimSuffix(strings.TrimSpace(sub), ".")
	if sub == "" || sub == "@" {
		return domain
	}
	return sub + "." + domain
}
