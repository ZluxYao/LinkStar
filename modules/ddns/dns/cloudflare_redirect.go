package dns

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// Cloudflare 支持入口重定向规则同步
var _ RedirectRuleProvider = (*Cloudflare)(nil)

// redirectPhase 单一重定向（Single Redirects）所在的规则阶段。
// 它在回源之前执行，所以入口域名底下挂什么 A 记录都无所谓——
// 橙云名只要能解析到 Cloudflare 就行，占位 IP 即可。
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

// SyncRedirectRule 把一条入口重定向写进 zone 的重定向规则集。
//
// 返回的 keepPath 表示是否用上了「保留原始路径」的动态目标。
// Cloudflare 各套餐对动态表达式的支持程度无法在本地断言，被拒就降级成
// 静态目标（用户从子路径进来会落到根路径），由调用方提示用户。
func (cf *Cloudflare) SyncRedirectRule(zoneDomain, ruleKey, entryHost, targetURL string) (bool, error) {
	zoneID, err := cf.zoneID(zoneDomain)
	if err != nil {
		return false, err
	}

	existing, err := cf.readRedirectRules(zoneID)
	if err != nil {
		return false, err
	}

	// 先试保留路径的动态写法
	dynamic, err := buildRedirectRule(ruleKey, entryHost, targetURL, true)
	if err != nil {
		return false, err
	}
	dynErr := cf.writeRedirectRules(zoneID, mergeRedirectRule(existing, ruleKey, dynamic))
	if dynErr == nil {
		return true, nil
	}

	// 失败原因无法可靠区分（套餐限制？表达式语法？权限？），
	// 所以一律退回静态写法再试一次：能用总比整条入口不可用强。
	static, err := buildRedirectRule(ruleKey, entryHost, targetURL, false)
	if err != nil {
		return false, err
	}
	if err := cf.writeRedirectRules(zoneID, mergeRedirectRule(existing, ruleKey, static)); err != nil {
		return false, err
	}
	return false, nil
}

// RemoveRedirectRule 删掉 LinkStar 自己写的那条规则，其余规则不动
func (cf *Cloudflare) RemoveRedirectRule(zoneDomain, ruleKey string) error {
	zoneID, err := cf.zoneID(zoneDomain)
	if err != nil {
		return err
	}

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

// ==== 以下为纯函数，不碰网络，便于测试 ====

// mergeRedirectRule 把 LinkStar 的规则并进现有规则集。
//
// 这是整个功能唯一有数据丢失风险的地方：PUT 会整体替换 rules 数组，
// 所以用户在 Cloudflare 后台手写的规则必须一条不少、一个字段不改地回去。
// LinkStar 只认领 description 等于 ruleKey 的那一条。
func mergeRedirectRule(existing []cfRule, ruleKey string, newRule cfRule) []cfRule {
	out := make([]cfRule, 0, len(existing)+1)
	replaced := false

	for _, r := range existing {
		if ruleDescription(r) != ruleKey {
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
		if ruleDescription(r) == ruleKey {
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

// buildRedirectRule 造一条「入口域名 → 落地地址」的 307 规则
func buildRedirectRule(ruleKey, entryHost, targetURL string, keepPath bool) (cfRule, error) {
	target := map[string]any{}
	if keepPath {
		// 保留用户进来时的路径：fn.zlux.top/library/x → zlux.top:34521/library/x
		target["expression"] = fmt.Sprintf("concat(%s, http.request.uri.path)",
			strconv.Quote(strings.TrimSuffix(targetURL, "/")))
	} else {
		target["value"] = targetURL
	}

	rule := map[string]any{
		"action":      "redirect",
		"description": ruleKey,
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
