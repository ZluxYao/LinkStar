package dns

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"sync"
)

// Cloudflare 支持入口重定向规则同步
var _ RedirectRuleProvider = (*Cloudflare)(nil)

// redirectPhase 单一重定向（Single Redirects）所在的规则阶段。
//
// 它在回源之前执行，所以入口域名的 A 记录指向哪个 IP 都无所谓，192.0.2.1 这种
// 占位地址即可——但那条记录必须是「已代理」（小黄云）。灰云是纯 DNS，
// 请求根本不经过 Cloudflare，这个阶段自然轮不到执行，规则写了也不会生效。
const redirectPhase = "http_request_dynamic_redirect"

// redirectStatusCode 只用 307。
//
// 绝对不能用 301/308：STUN 给的端口会漂，而永久重定向会被浏览器永久缓存，
// 端口一变用户就只能靠清浏览器数据才能恢复。和 routers/portal.go 里 307 的理由一致。
const redirectStatusCode = 307

// cfRule 一条规则的原始 JSON。
//
// 刻意用 map[string]json.RawMessage 而不是结构体：用户在 Cloudflare 后台
// 手写的规则里可能有 LinkStar 根本不认识的字段，读-改-写时必须原样放回去。
// 用结构体解析就会把不认识的字段悄悄吃掉——那等于替用户删了他的配置。
type cfRule map[string]json.RawMessage

// cfReadOnlyRuleFields 由 Cloudflare 生成、写回时不该带上的字段
var cfReadOnlyRuleFields = []string{"version", "last_updated"}

type cfRulesetResp struct {
	CloudflareStatus
	Result struct {
		ID    string   `json:"id"`
		Rules []cfRule `json:"rules"`
	} `json:"result"`
}

type cfRulesetPut struct {
	Rules []cfRule `json:"rules"`
}

// redirectRulesetLocks 同一个 zone 的规则集，一次只让一个人改。
//
// Cloudflare 没有「只改一条规则」的接口：一个 zone 的重定向规则是一整份
// ruleset，改任何一条都得整份读回来、改完整份写回去。两个服务同时同步时，
// 各自读到的都是对方写之前那一份，后写的那份里带着前一个的旧端口，
// 把刚写好的按了回去。
//
// 被按回去的那一边拿到的是 200，于是记下「服务商那边已经是新端口了」，
// 之后地址没变就再也不同步（scheduler.go 的 shouldSyncRedirect），
// 界面显示新端口、Cloudflare 那边是旧端口，而且永远不会自己纠正。
// 开机时所有洞几乎同时打通，这个必撞——而且撞完两边都不报错。
//
// 按 zone 分锁：不同账号、不同域名之间没有共享的规则集，不用互相等。
var redirectRulesetLocks sync.Map // zoneID -> *sync.Mutex

// lockRedirectRuleset 锁住这个 zone 的规则集，返回解锁函数
func lockRedirectRuleset(zoneID string) func() {
	v, _ := redirectRulesetLocks.LoadOrStore(zoneID, &sync.Mutex{})
	mu := v.(*sync.Mutex)
	mu.Lock()
	return mu.Unlock
}

// SyncRedirectRule 把一条入口重定向写进 zone 的重定向规则集。
//
// 返回的 keepPath 表示是否用上了「保留原始路径」的动态目标。
// Cloudflare 各套餐对动态表达式的支持程度无法在本地断言，被拒就降级成
// 静态目标（用户从子路径进来会落到根路径），由调用方提示用户。
//
// entryWarn 是「规则写好了，但入口域名那条 DNS 记录没能确认」。规则本身是成功的，
// 所以不当失败处理；但也绝不能咽下去——少了那条记录，访问就是不通。
func (cf *Cloudflare) SyncRedirectRule(zoneDomain, ruleKey, ruleLabel, entryHost, targetURL string) (bool, string, error) {
	if err := guardRedirectLoop(entryHost, targetURL); err != nil {
		return false, "", err
	}

	zoneID, err := cf.zoneID(zoneDomain)
	if err != nil {
		return false, "", err
	}

	// 规则和记录得一起到位，少一个都是「规则写进去了但访问不了」，
	// 而这种失败从 LinkStar 这侧完全看不出来，只能靠人去 Cloudflare 后台翻。
	entryWarn, err := cf.ensureEntryRecord(zoneID, entryHost)
	if err != nil {
		return false, "", err
	}

	// 从这里到函数结束是一次完整的读-改-写，中间不能让第二个服务插进来：
	// 它读到的会是本次写入之前那一份，写回去就等于把这次的端口按回旧值。
	unlock := lockRedirectRuleset(zoneID)
	defer unlock()

	existing, err := cf.readRedirectRules(zoneID)
	if err != nil {
		return false, entryWarn, err
	}

	// 先试保留路径的动态写法
	dynamic, err := buildRedirectRule(ruleKey, ruleLabel, entryHost, targetURL, true)
	if err != nil {
		return false, entryWarn, err
	}
	dynErr := cf.writeRedirectRules(zoneID, mergeRedirectRule(existing, ruleKey, dynamic))
	if dynErr == nil {
		return true, entryWarn, nil
	}

	// 失败原因无法可靠区分（套餐限制？表达式语法？权限？），
	// 所以一律退回静态写法再试一次：能用总比整条入口不可用强。
	static, err := buildRedirectRule(ruleKey, ruleLabel, entryHost, targetURL, false)
	if err != nil {
		return false, entryWarn, err
	}
	if err := cf.writeRedirectRules(zoneID, mergeRedirectRule(existing, ruleKey, static)); err != nil {
		return false, entryWarn, err
	}
	return false, entryWarn, nil
}

// RemoveRedirectRule 删掉 LinkStar 自己写的那条规则，其余规则不动
func (cf *Cloudflare) RemoveRedirectRule(zoneDomain, ruleKey string) error {
	zoneID, err := cf.zoneID(zoneDomain)
	if err != nil {
		return err
	}

	// 删也是整份写回去，和同步抢的是同一份规则集
	unlock := lockRedirectRuleset(zoneID)
	defer unlock()

	existing, err := cf.readRedirectRules(zoneID)
	if err != nil {
		return err
	}

	rules, removed := removeRedirectRule(existing, ruleKey)
	if !removed {
		return nil // 本来就没有，不用白打一次 API
	}
	return cf.writeRedirectRules(zoneID, rules)
}

func (cf *Cloudflare) readRedirectRules(zoneID string) ([]cfRule, error) {
	var resp cfRulesetResp
	err := cf.request("GET", rulesetEntrypointURL(zoneID), nil, &resp)
	if err != nil {
		// 这个 zone 还没有任何重定向规则时，入口规则集压根不存在。
		// 那不是错误，是「空」——后面的 PUT 会把它创建出来。
		if isNotFound(err) {
			return nil, nil
		}
		if isForbidden(err) {
			return nil, errRulesetPermission
		}
		return nil, fmt.Errorf("读取重定向规则失败: %w", err)
	}
	if !resp.Success {
		return nil, fmt.Errorf("读取重定向规则返回失败: %v", resp.Messages)
	}
	return resp.Result.Rules, nil
}

func (cf *Cloudflare) writeRedirectRules(zoneID string, rules []cfRule) error {
	if rules == nil {
		rules = []cfRule{}
	}
	var resp cfRulesetResp
	// PUT 是整体替换：rules 里少了谁，Cloudflare 那边就没了谁。
	// 所以传进来的必须是「全量」——合并逻辑见 mergeRedirectRule。
	if err := cf.request("PUT", rulesetEntrypointURL(zoneID), cfRulesetPut{Rules: rules}, &resp); err != nil {
		if isForbidden(err) {
			return errRulesetPermission
		}
		return fmt.Errorf("写入重定向规则失败: %w", err)
	}
	if !resp.Success {
		return fmt.Errorf("写入重定向规则返回失败: %v", resp.Messages)
	}
	return nil
}

func rulesetEntrypointURL(zoneID string) string {
	return fmt.Sprintf("%s/%s/rulesets/phases/%s/entrypoint", zonesAPI, zoneID, redirectPhase)
}

func isNotFound(err error) bool {
	return err != nil && strings.Contains(err.Error(), "返回状态码:404")
}

func isForbidden(err error) bool {
	return err != nil && strings.Contains(err.Error(), "返回状态码:403")
}

// errRulesetPermission 缺权限时 Cloudflare 回的原文是
// 403 + {"code":10000,"message":"Authentication error"}。
//
// 「Authentication error」字面上像 Token 填错了，照着字面去重新贴一遍 Token
// 只会贴出一个一样报错的——真实原因是权限范围不够：DDNS 解析只要
// Zone → DNS → Edit，而重定向规则走的是 Rulesets，要 Zone → Dynamic Redirect → Edit。
// 同一个 Token 于是出现「域名解析好好的，重定向就是 403」这种看着没道理的现象。
var errRulesetPermission = errors.New(
	"Cloudflare Token 权限不够：域名解析只要 DNS 权限，改重定向规则还要 Zone → Dynamic Redirect → Edit。" +
		"去 Cloudflare 后台「我的个人资料 → API 令牌」给这个 Token 补上这项权限（部分账号还需要 Account → Account Rulesets → Edit），" +
		"改完不用重贴 Token，直接再点一次同步",
)

// ==== 入口域名的那条 DNS 记录 ====

// entryRecordIP 入口域名 A 记录填的占位地址（RFC 5737 文档保留段，永远不会指向真机器）。
//
// 重定向在回源之前就执行完了，这条记录指哪儿都无所谓；它唯一的作用是让请求先进
// Cloudflare。填个保留地址而不是随便一个 IP，是为了万一小黄云被关掉，访问直接失败，
// 而不是悄悄打到别人的机器上。
const entryRecordIP = "192.0.2.1"

// entryRecordComment 标记这条记录是 LinkStar 建的。
//
// 用户自己手建的同名记录可能指着真机器，改它之前得先认出来哪条不是自己的。
const entryRecordComment = "linkstar:entry"

// entryRecordTTL 已代理的记录必须是自动 TTL，Cloudflare 不接受具体秒数
const entryRecordTTL = 1

// cfEntryRecord 建记录的载荷。
// 不复用 CloudflareRecord：那个带 id 字段，新建时传空 id 是多余的。
type cfEntryRecord struct {
	Name    string `json:"name"`
	Type    string `json:"type"`
	Content string `json:"content"`
	Proxied bool   `json:"proxied"`
	TTL     int    `json:"ttl"`
	Comment string `json:"comment,omitempty"`
}

// errEntryRecordPermission 想替用户建记录但 Token 只有重定向权限时的说明
var errEntryRecordPermission = errors.New(
	"入口域名还没有 DNS 记录，LinkStar 想替你建但没权限：这个 Token 还需要 Zone → DNS → 编辑。" +
		"要么去 Cloudflare 后台给 Token 补上这项，要么自己在 DNS 里加一条 A 记录指向 " +
		entryRecordIP + " 并开小黄云",
)

func errEntryRecordGrey(entryHost string) error {
	return fmt.Errorf(
		"%s 这条记录没开小黄云，请求不经过 Cloudflare，重定向规则再对也不会执行。"+
			"这条记录是你自己加的（不是 LinkStar 建的），所以没敢替你改——"+
			"去 Cloudflare DNS 里把它的代理状态改成「已代理」", entryHost)
}

// ensureEntryRecord 保证入口域名有一条「已代理」的记录。
//
// 没有这条记录，Cloudflare 那边只会安静地什么都不做：规则列表里明明躺着一条对的规则，
// 访问就是不通，两侧都不报错。这一步就是为了不把这个只能靠猜的坑留给用户。
//
// 返回的 warn 是「记录这一步没做成，但规则该写还得写」。查不到记录不当硬失败：
// 用户完全可能已经自己在 Cloudflare 后台建好了，硬拦等于把本来能用的功能拦下来。
// 但也绝不能像以前那样默默返回成功——那会让「域名压根不存在」长得和一切正常一模一样。
func (cf *Cloudflare) ensureEntryRecord(zoneID, entryHost string) (string, error) {
	entryHost = strings.TrimSuffix(strings.TrimSpace(entryHost), ".")
	if entryHost == "" {
		return "", nil // 上层已经拦过空值，这里不重复报错
	}

	rec, warn, err := cf.findEntryRecord(zoneID, entryHost)
	if err != nil {
		return "", err
	}
	if warn != "" {
		return warn, nil
	}
	if rec == nil {
		return "", cf.createEntryRecord(zoneID, entryHost)
	}
	if rec.Proxied {
		return "", nil // 已经是要的样子
	}
	if rec.Content != entryRecordIP && rec.Comment != entryRecordComment {
		return "", errEntryRecordGrey(entryHost) // 用户自己的记录，不动
	}
	// 这条是 LinkStar 建的（或者内容就是那个占位地址），可以替他开上
	if err := cf.modify(*rec, zoneID, rec.Content, entryRecordTTL, true); err != nil {
		if isForbidden(err) {
			return "", errEntryRecordPermission
		}
		return "", fmt.Errorf("给 %s 打开小黄云失败: %w", entryHost, err)
	}
	return "", nil
}

// findEntryRecord 查入口域名下和「入口」有关的那条记录，不改任何东西。
//
// 返回三种结果，调用方分别处理：记录（有）、warn（查不了，多半是 Token 没 DNS 权限）、
// 两个都空（能查，确实没有这条记录）。
func (cf *Cloudflare) findEntryRecord(zoneID, entryHost string) (*CloudflareRecord, string, error) {
	params := url.Values{}
	params.Set("name", entryHost)
	params.Set("per_page", "50")

	var records CloudflareRecordsResp
	err := cf.request("GET",
		fmt.Sprintf("%s/%s/dns_records?%s", zonesAPI, zoneID, params.Encode()),
		nil, &records)
	switch {
	case isForbidden(err):
		// 这个最常见，也最容易被当成别的问题：Token 有重定向权限、没有 DNS 权限，
		// 于是规则写得进去、记录一条都读不到，单看结果和「一切正常」长得一样
		return nil, warnEntryRecordNoPerm(entryHost), nil
	case err != nil:
		return nil, warnEntryRecordUnchecked(entryHost, err.Error()), nil
	case !records.Success:
		return nil, warnEntryRecordUnchecked(entryHost, fmt.Sprint(records.Messages)), nil
	}

	for _, rec := range records.Result {
		// 同名下可能还挂着 TXT（比如 ACME 挑战），那些和入口无关
		switch rec.Type {
		case "A", "AAAA", "CNAME":
			return &rec, "", nil
		}
	}
	return nil, "", nil
}

// RemoveEntryRecord 服务删了，把当初替入口域名建的那条占位记录也收回去。
//
// 不收的话，DNS 里会剩一条指向 192.0.2.1 的已代理记录：规则已经没了，
// 访问这个域名的人撞上的是 Cloudflare 的错误页，而 LinkStar 这边什么都不显示——
// 因为这个服务在它眼里早就不存在了。
//
// 这里比 ensureEntryRecord 严格一档：那边看见内容是占位地址就肯替用户把小黄云打开，
// 这边必须备注也对得上才删。按权限报错里的提示自己手建那条记录的用户，内容同样是
// 192.0.2.1 却没有备注，那是他的东西（何况那种情况下 Token 本来就没有 DNS 权限，
// 也删不动）。删 DNS 记录没有撤销，宁可留一条没用的让他自己删。
func (cf *Cloudflare) RemoveEntryRecord(zoneDomain, entryHost string) (bool, error) {
	entryHost = strings.TrimSuffix(strings.TrimSpace(entryHost), ".")
	if entryHost == "" {
		return false, nil
	}

	zoneID, err := cf.zoneID(zoneDomain)
	if err != nil {
		return false, err
	}

	rec, warn, err := cf.findEntryRecord(zoneID, entryHost)
	if err != nil {
		return false, err
	}
	if warn != "" {
		// 记录查不了（多半是 Token 没有 DNS 权限）。查不清楚就别动手
		return false, errors.New(warn)
	}
	if rec == nil || rec.Comment != entryRecordComment {
		return false, nil
	}

	var status CloudflareStatus
	if err := cf.request("DELETE",
		fmt.Sprintf("%s/%s/dns_records/%s", zonesAPI, zoneID, rec.ID), nil, &status); err != nil {
		return false, fmt.Errorf("删除入口域名 %s 的记录失败: %w", entryHost, err)
	}
	if !status.Success {
		return false, fmt.Errorf("删除入口域名 %s 的记录返回失败: %v", entryHost, status.Messages)
	}
	return true, nil
}

// InspectEntryRecord 只看不改：入口域名那条记录现在到底在不在、开没开小黄云。
//
// 这条记录不归 DDNS 管（内容永远是那个占位地址，不会变），所以别处没人盯着它；
// 可它一旦没了或者被关成灰云，重定向就彻底不执行，而规则列表里那条规则看着完全正常。
func (cf *Cloudflare) InspectEntryRecord(zoneDomain, entryHost string) (EntryRecordState, error) {
	entryHost = strings.TrimSuffix(strings.TrimSpace(entryHost), ".")
	st := EntryRecordState{Host: entryHost, WantIP: entryRecordIP}
	if entryHost == "" {
		return st, errors.New("入口域名为空")
	}

	zoneID, err := cf.zoneID(zoneDomain)
	if err != nil {
		return st, err
	}
	rec, warn, err := cf.findEntryRecord(zoneID, entryHost)
	if err != nil {
		return st, err
	}
	st.Warn = warn
	if rec == nil {
		return st, nil
	}
	st.Found = true
	st.Type = rec.Type
	st.Content = rec.Content
	st.Proxied = rec.Proxied
	st.ByLinkStar = rec.Comment == entryRecordComment
	return st, nil
}

// warnEntryRecordNoPerm Token 读不了 DNS 记录时的说明。
//
// 这条警告存在的全部意义：规则照样写得进去，记录却一条都建不了，
// 于是访问入口域名直接「域名不存在」，而 LinkStar 这侧显示的是「已同步」。
func warnEntryRecordNoPerm(entryHost string) string {
	return fmt.Sprintf(
		"重定向规则写好了，但这个 Token 没有 DNS 权限，%s 这条解析记录建不了——"+
			"现在访问它多半会提示域名不存在。两条路选一条："+
			"去 Cloudflare「我的个人资料 → API 令牌」给它补上 Zone → DNS → 编辑，再点一次同步；"+
			"或者自己在 DNS 里加一条 %s 的 A 记录指向 %s 并开小黄云",
		entryHost, entryHost, entryRecordIP)
}

// warnEntryRecordUnchecked 除了没权限之外的原因导致查不到记录时的说明。
//
// 原因五花八门（网络不通、接口改了、账号被限流），没法逐个给建议，
// 但至少得让人知道「这一步没做成」，而不是把它算进「已同步」里。
func warnEntryRecordUnchecked(entryHost, cause string) string {
	return fmt.Sprintf(
		"重定向规则写好了，但没能确认 %s 有没有解析记录（%s）。"+
			"要是访问它提示域名不存在，就去 Cloudflare DNS 里加一条 A 记录指向 %s 并开小黄云",
		entryHost, shortCause(cause), entryRecordIP)
}

// shortCause 把服务商返回的原文压成一行短句。
//
// Cloudflare 报错会把整个响应体带回来，几百个字符照原样贴进界面，
// 真正有用的那半句就被埋了。留个头，够对着搜就行。
func shortCause(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	const max = 120
	if r := []rune(s); len(r) > max {
		return string(r[:max]) + "…"
	}
	return s
}

func (cf *Cloudflare) createEntryRecord(zoneID, entryHost string) error {
	var status CloudflareStatus
	err := cf.request("POST",
		fmt.Sprintf("%s/%s/dns_records", zonesAPI, zoneID),
		cfEntryRecord{
			Name:    entryHost,
			Type:    "A",
			Content: entryRecordIP,
			Proxied: true,
			TTL:     entryRecordTTL,
			Comment: entryRecordComment,
		},
		&status)
	if err != nil {
		if isForbidden(err) {
			return errEntryRecordPermission
		}
		return fmt.Errorf("自动创建入口域名记录失败: %w", err)
	}
	if !status.Success {
		return fmt.Errorf("自动创建入口域名记录返回失败: %v", status.Messages)
	}
	return nil
}

// ==== 以下为纯函数，不碰网络，便于测试 ====

// guardRedirectLoop 入口域名不能和落地域名是同一个。
//
// 同一个的话这条记录得同时是两种身份：入口要开小黄云，重定向规则才轮得到执行；
// 落地要关小黄云，浏览器才连得上 STUN 那个端口——Cloudflare 的代理不转发 24969 这种端口。
// 一条记录满足不了两边，配出来的结果是转一圈然后连不上，且两侧都不报错。
func guardRedirectLoop(entryHost, targetURL string) error {
	entry := strings.TrimSuffix(strings.TrimSpace(entryHost), ".")
	u, err := url.Parse(targetURL)
	if err != nil {
		return nil // 解析不了就别拦，真有问题写规则时也会报
	}
	target := strings.TrimSuffix(u.Hostname(), ".")
	if target == "" || !strings.EqualFold(entry, target) {
		return nil
	}
	return fmt.Errorf(
		"入口域名和落地域名都是 %s，这样配不通：入口域名要开小黄云才能触发重定向，"+
			"落地域名要关小黄云浏览器才连得上那个端口，一条记录做不到两头。"+
			"给入口换个名字，比如 link.%s", entry, entry)
}

// mergeRedirectRule 把 LinkStar 的规则并进现有规则集。
//
// 这是整个功能唯一有数据丢失风险的地方：PUT 会整体替换 rules 数组，
// 所以用户在 Cloudflare 后台手写的规则必须一条不少、一个字段不改地回去。
// LinkStar 只认领规则名前半截等于 ruleKey 的那一条。
func mergeRedirectRule(existing []cfRule, ruleKey string, newRule cfRule) []cfRule {
	out := make([]cfRule, 0, len(existing)+1)
	replaced := false

	for _, r := range existing {
		if ruleKeyOf(r) != ruleKey {
			out = append(out, sanitizeRule(r)) // 别人的规则，原样放回
			continue
		}
		// 自己上次写的那条：就地替换，保住它在规则集里的位置。
		// 规则是按顺序求值的，挪位置会悄悄改变用户的匹配行为。
		merged := sanitizeRule(newRule)
		if id, ok := r["id"]; ok {
			merged["id"] = id
		}
		out = append(out, merged)
		replaced = true
	}

	if !replaced {
		out = append(out, sanitizeRule(newRule))
	}
	return out
}

// removeRedirectRule 摘掉 LinkStar 自己的规则，返回是否真的摘到了
func removeRedirectRule(existing []cfRule, ruleKey string) ([]cfRule, bool) {
	out := make([]cfRule, 0, len(existing))
	removed := false
	for _, r := range existing {
		if ruleKeyOf(r) == ruleKey {
			removed = true
			continue
		}
		out = append(out, sanitizeRule(r))
	}
	return out, removed
}

// sanitizeRule 去掉 Cloudflare 生成的只读字段，其余原样保留
func sanitizeRule(r cfRule) cfRule {
	out := make(cfRule, len(r))
	for k, v := range r {
		out[k] = v
	}
	for _, k := range cfReadOnlyRuleFields {
		delete(out, k)
	}
	return out
}

func ruleDescription(r cfRule) string {
	raw, ok := r["description"]
	if !ok {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return ""
	}
	return s
}

// ruleDescSep 规则名里「认领用的 key」和「给人看的服务名」之间的分隔
const ruleDescSep = " | "

// ruleLabelMaxRunes 服务名在规则名里最多留这么长。
// Cloudflare 的 description 有长度上限，名字起得离谱也不该让整条规则写不进去。
const ruleLabelMaxRunes = 64

// composeRuleDescription 拼规则名：linkstar:1-2 | 群晖。
//
// 前半截是认领用的，一个字都不能变；后半截只给人看，在 Cloudflare 后台
// 一眼能认出哪条对应哪个服务。服务名里的分隔符和换行得洗掉，
// 不然拆回来的时候会把名字的一部分当成 key。
func composeRuleDescription(ruleKey, ruleLabel string) string {
	label := strings.Map(func(r rune) rune {
		if r == '|' || r == '\n' || r == '\r' || r == '\t' {
			return ' '
		}
		return r
	}, ruleLabel)
	label = strings.TrimSpace(strings.Join(strings.Fields(label), " "))
	if label == "" {
		return ruleKey
	}
	if runes := []rune(label); len(runes) > ruleLabelMaxRunes {
		label = string(runes[:ruleLabelMaxRunes])
	}
	return ruleKey + ruleDescSep + label
}

// ruleKeyOf 从规则名里取出认领用的那一段。
//
// 老版本写进去的规则名就是光秃秃一个 linkstar:1-2，没有分隔符，
// 整条就是 key——所以升级上来的用户不会多出一条重复规则。
func ruleKeyOf(r cfRule) string {
	desc := ruleDescription(r)
	if i := strings.Index(desc, ruleDescSep); i >= 0 {
		return desc[:i]
	}
	return desc
}

// buildRedirectRule 造一条「入口域名 → 落地地址」的 307 规则
func buildRedirectRule(ruleKey, ruleLabel, entryHost, targetURL string, keepPath bool) (cfRule, error) {
	target := map[string]any{}
	if keepPath {
		// 保留用户进来时的路径：fn.example.com/library/x → example.com:34521/library/x
		target["expression"] = fmt.Sprintf("concat(%s, http.request.uri.path)",
			strconv.Quote(strings.TrimSuffix(targetURL, "/")))
	} else {
		target["value"] = targetURL
	}

	rule := map[string]any{
		"action":      "redirect",
		"description": composeRuleDescription(ruleKey, ruleLabel),
		"enabled":     true,
		"expression":  fmt.Sprintf("(http.host eq %s)", strconv.Quote(entryHost)),
		"action_parameters": map[string]any{
			"from_value": map[string]any{
				"status_code":           redirectStatusCode,
				"target_url":            target,
				"preserve_query_string": true,
			},
		},
	}

	raw, err := json.Marshal(rule)
	if err != nil {
		return nil, err
	}
	var out cfRule
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	return out, nil
}
