package dns

import (
	"encoding/json"
	"strings"
	"testing"
)

// mustRules 把一段 JSON 规则数组解析成 []cfRule
func mustRules(t *testing.T, raw string) []cfRule {
	t.Helper()
	var rules []cfRule
	if err := json.Unmarshal([]byte(raw), &rules); err != nil {
		t.Fatalf("解析规则失败: %v", err)
	}
	return rules
}

func field(t *testing.T, r cfRule, key string) string {
	t.Helper()
	raw, ok := r[key]
	if !ok {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return string(raw)
	}
	return s
}

// 用户手写的规则 + LinkStar 上次写的规则
const existingRulesJSON = `[
  {
    "id": "user-rule-1",
    "version": "3",
    "last_updated": "2026-01-01T00:00:00Z",
    "action": "redirect",
    "description": "把老博客地址转到新站",
    "enabled": true,
    "expression": "(http.host eq \"blog.zlux.top\")",
    "action_parameters": {"from_value": {"status_code": 301, "target_url": {"value": "https://new.example.com"}}},
    "some_field_linkstar_does_not_know": {"a": [1, 2, 3]}
  },
  {
    "id": "linkstar-rule",
    "version": "7",
    "action": "redirect",
    "description": "linkstar:entry-5",
    "enabled": true,
    "expression": "(http.host eq \"fn.zlux.top\")",
    "action_parameters": {"from_value": {"status_code": 307, "target_url": {"value": "https://zlux.top:11111"}}}
  },
  {
    "id": "user-rule-2",
    "action": "redirect",
    "description": "另一条用户规则",
    "enabled": false,
    "expression": "(http.host eq \"old.zlux.top\")"
  }
]`

// 本功能唯一有数据丢失风险的地方：PUT 会整体替换 rules 数组。
// 这个测试守的就是「用户手写的规则一条不少、一个字段不改」。
func TestMergePreservesForeignRules(t *testing.T) {
	existing := mustRules(t, existingRulesJSON)
	newRule, err := buildRedirectRule("linkstar:entry-5", "fn.zlux.top", "https://zlux.top:34521", false)
	if err != nil {
		t.Fatal(err)
	}

	got := mergeRedirectRule(existing, "linkstar:entry-5", newRule)

	if len(got) != 3 {
		t.Fatalf("合并后应有 3 条规则，实际 %d 条", len(got))
	}

	// 顺序必须保持：规则是按顺序求值的，挪位置会改变用户的匹配行为
	wantDesc := []string{"把老博客地址转到新站", "linkstar:entry-5", "另一条用户规则"}
	for i, want := range wantDesc {
		if got := field(t, got[i], "description"); got != want {
			t.Errorf("第 %d 条规则 description = %q，期望 %q", i, got, want)
		}
	}

	// 用户那条规则的每个字段（包括 LinkStar 不认识的）都必须原样还在
	foreign := got[0]
	for _, key := range []string{"id", "action", "description", "enabled", "expression",
		"action_parameters", "some_field_linkstar_does_not_know"} {
		if _, ok := foreign[key]; !ok {
			t.Errorf("用户规则的字段 %q 被吃掉了", key)
		}
	}
	if string(foreign["some_field_linkstar_does_not_know"]) != `{"a": [1, 2, 3]}` &&
		string(foreign["some_field_linkstar_does_not_know"]) != `{"a":[1,2,3]}` {
		t.Errorf("不认识的字段内容被改动了：%s", foreign["some_field_linkstar_does_not_know"])
	}
	// 只读字段不该写回去
	for _, key := range cfReadOnlyRuleFields {
		if _, ok := foreign[key]; ok {
			t.Errorf("只读字段 %q 不该出现在写回的规则里", key)
		}
	}

	// 自己那条应该被更新成新端口，且保住原来的 id
	own := got[1]
	if id := field(t, own, "id"); id != "linkstar-rule" {
		t.Errorf("自己的规则 id = %q，期望沿用 linkstar-rule", id)
	}
	if !strings.Contains(string(own["action_parameters"]), "zlux.top:34521") {
		t.Errorf("自己的规则没有更新到新端口：%s", own["action_parameters"])
	}
	if _, ok := own["version"]; ok {
		t.Error("自己的规则不该带上只读的 version 字段")
	}
}

func TestMergeAppendsWhenAbsent(t *testing.T) {
	existing := mustRules(t, existingRulesJSON)
	newRule, err := buildRedirectRule("linkstar:entry-9", "new.zlux.top", "https://zlux.top:22222", true)
	if err != nil {
		t.Fatal(err)
	}

	got := mergeRedirectRule(existing, "linkstar:entry-9", newRule)
	if len(got) != 4 {
		t.Fatalf("新入口应追加一条规则，实际共 %d 条", len(got))
	}
	if d := field(t, got[3], "description"); d != "linkstar:entry-9" {
		t.Errorf("新规则应追加在末尾，实际末尾是 %q", d)
	}
	// 新规则不该凭空冒出一个 id——那是 Cloudflare 生成的
	if _, ok := got[3]["id"]; ok {
		t.Error("新规则不该自带 id")
	}
}

func TestMergeOnEmptyRuleset(t *testing.T) {
	newRule, err := buildRedirectRule("linkstar:entry-1", "fn.zlux.top", "https://zlux.top:1", false)
	if err != nil {
		t.Fatal(err)
	}
	got := mergeRedirectRule(nil, "linkstar:entry-1", newRule)
	if len(got) != 1 {
		t.Fatalf("空规则集合并后应有 1 条，实际 %d 条", len(got))
	}
}

func TestRemoveOnlyTouchesOwnRule(t *testing.T) {
	existing := mustRules(t, existingRulesJSON)

	got, removed := removeRedirectRule(existing, "linkstar:entry-5")
	if !removed {
		t.Fatal("应该摘到 LinkStar 自己的规则")
	}
	if len(got) != 2 {
		t.Fatalf("删除后应剩 2 条用户规则，实际 %d 条", len(got))
	}
	for _, r := range got {
		if strings.HasPrefix(field(t, r, "description"), "linkstar:") {
			t.Error("LinkStar 的规则没删干净")
		}
	}

	// 规则不存在时必须报告「没删到」，好让上层跳过这次 PUT
	if _, removed := removeRedirectRule(existing, "linkstar:entry-404"); removed {
		t.Error("不存在的规则不该报告为已删除")
	}
}

func TestBuildRedirectRule(t *testing.T) {
	// 端口会漂，永久重定向会被浏览器永久缓存——只能用 307
	rule, err := buildRedirectRule("linkstar:entry-5", "fn.zlux.top", "https://zlux.top:34521", false)
	if err != nil {
		t.Fatal(err)
	}
	params := string(rule["action_parameters"])
	if !strings.Contains(params, `"status_code":307`) {
		t.Errorf("状态码必须是 307，实际：%s", params)
	}
	if !strings.Contains(params, `"value":"https://zlux.top:34521"`) {
		t.Errorf("静态目标地址不对：%s", params)
	}
	if expr := field(t, rule, "expression"); expr != `(http.host eq "fn.zlux.top")` {
		t.Errorf("匹配表达式不对：%q", expr)
	}

	// 保留路径走动态表达式
	dyn, err := buildRedirectRule("linkstar:entry-5", "fn.zlux.top", "https://zlux.top:34521/", true)
	if err != nil {
		t.Fatal(err)
	}
	dynParams := string(dyn["action_parameters"])
	if !strings.Contains(dynParams, `concat(\"https://zlux.top:34521\", http.request.uri.path)`) {
		t.Errorf("动态目标表达式不对（尾斜杠也应剥掉）：%s", dynParams)
	}
}
