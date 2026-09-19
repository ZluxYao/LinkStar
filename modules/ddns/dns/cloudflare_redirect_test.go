package dns

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
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
    "expression": "(http.host eq \"blog.example.com\")",
    "action_parameters": {"from_value": {"status_code": 301, "target_url": {"value": "https://new.example.com"}}},
    "some_field_linkstar_does_not_know": {"a": [1, 2, 3]}
  },
  {
    "id": "linkstar-rule",
    "version": "7",
    "action": "redirect",
    "description": "linkstar:entry-5",
    "enabled": true,
    "expression": "(http.host eq \"fn.example.com\")",
    "action_parameters": {"from_value": {"status_code": 307, "target_url": {"value": "https://example.com:11111"}}}
  },
  {
    "id": "user-rule-2",
    "action": "redirect",
    "description": "另一条用户规则",
    "enabled": false,
    "expression": "(http.host eq \"old.example.com\")"
  }
]`

// 本功能唯一有数据丢失风险的地方：PUT 会整体替换 rules 数组。
// 这个测试守的就是「用户手写的规则一条不少、一个字段不改」。
func TestMergePreservesForeignRules(t *testing.T) {
	existing := mustRules(t, existingRulesJSON)
	newRule, err := buildRedirectRule("linkstar:entry-5", "群晖", "fn.example.com", "https://example.com:34521", false)
	if err != nil {
		t.Fatal(err)
	}

	got := mergeRedirectRule(existing, "linkstar:entry-5", newRule)

	if len(got) != 3 {
		t.Fatalf("合并后应有 3 条规则，实际 %d 条", len(got))
	}

	// 顺序必须保持：规则是按顺序求值的，挪位置会改变用户的匹配行为。
	// 中间那条老版本写的规则名还是光秃秃的 linkstar:entry-5，这里必须被
	// 就地换成带服务名的新名字——认不出来就会在旁边多出一条重复规则。
	wantDesc := []string{"把老博客地址转到新站", "linkstar:entry-5 | 群晖", "另一条用户规则"}
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
	if !strings.Contains(string(own["action_parameters"]), "example.com:34521") {
		t.Errorf("自己的规则没有更新到新端口：%s", own["action_parameters"])
	}
	if _, ok := own["version"]; ok {
		t.Error("自己的规则不该带上只读的 version 字段")
	}
}

func TestMergeAppendsWhenAbsent(t *testing.T) {
	existing := mustRules(t, existingRulesJSON)
	newRule, err := buildRedirectRule("linkstar:entry-9", "", "new.example.com", "https://example.com:22222", true)
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
	newRule, err := buildRedirectRule("linkstar:entry-1", "", "fn.example.com", "https://example.com:1", false)
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
	rule, err := buildRedirectRule("linkstar:entry-5", "", "fn.example.com", "https://example.com:34521", false)
	if err != nil {
		t.Fatal(err)
	}
	params := string(rule["action_parameters"])
	if !strings.Contains(params, `"status_code":307`) {
		t.Errorf("状态码必须是 307，实际：%s", params)
	}
	if !strings.Contains(params, `"value":"https://example.com:34521"`) {
		t.Errorf("静态目标地址不对：%s", params)
	}
	if expr := field(t, rule, "expression"); expr != `(http.host eq "fn.example.com")` {
		t.Errorf("匹配表达式不对：%q", expr)
	}

	// 保留路径走动态表达式
	dyn, err := buildRedirectRule("linkstar:entry-5", "", "fn.example.com", "https://example.com:34521/", true)
	if err != nil {
		t.Fatal(err)
	}
	dynParams := string(dyn["action_parameters"])
	if !strings.Contains(dynParams, `concat(\"https://example.com:34521\", http.request.uri.path)`) {
		t.Errorf("动态目标表达式不对（尾斜杠也应剥掉）：%s", dynParams)
	}
}

// TestRuleDescriptionKeepsKeyStable 规则名后面挂服务名，认领只看前面那段。
//
// 守的是「服务改名不会在 Cloudflare 那边多出一条孤儿规则」：认领要是连
// 服务名一起比，改一次名就认不出上一条，旧规则留在那儿继续把人送到
// 一个早就没了的端口，而两边都不报错。
func TestRuleDescriptionKeepsKeyStable(t *testing.T) {
	cases := []struct {
		name     string
		label    string
		wantDesc string
	}{
		{"带服务名", "群晖", "linkstar:1-2 | 群晖"},
		{"没名字就只剩 key", "  ", "linkstar:1-2"},
		{"名字里的竖线和换行洗掉", "群晖 | NAS\n备用", "linkstar:1-2 | 群晖 NAS 备用"},
		{"名字太长就截断", strings.Repeat("长", 100), "linkstar:1-2 | " + strings.Repeat("长", ruleLabelMaxRunes)},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			desc := composeRuleDescription("linkstar:1-2", c.label)
			if desc != c.wantDesc {
				t.Fatalf("规则名 = %q，期望 %q", desc, c.wantDesc)
			}
			rule, err := buildRedirectRule("linkstar:1-2", c.label, "fn.example.com", "https://example.com:1", false)
			if err != nil {
				t.Fatal(err)
			}
			if got := ruleKeyOf(rule); got != "linkstar:1-2" {
				t.Fatalf("认领用的 key = %q，期望 linkstar:1-2", got)
			}
		})
	}

	// 改名之后还得认得出上一条，否则就是多写一条、旧的成孤儿
	old := mustRules(t, `[{"description":"linkstar:1-2 | 旧名字","action":"redirect"}]`)
	renamed, err := buildRedirectRule("linkstar:1-2", "新名字", "fn.example.com", "https://example.com:1", false)
	if err != nil {
		t.Fatal(err)
	}
	got := mergeRedirectRule(old, "linkstar:1-2", renamed)
	if len(got) != 1 {
		t.Fatalf("改名后应就地替换，实际变成 %d 条", len(got))
	}
	if d := field(t, got[0], "description"); d != "linkstar:1-2 | 新名字" {
		t.Fatalf("改名没生效：%q", d)
	}
	if _, removed := removeRedirectRule(got, "linkstar:1-2"); !removed {
		t.Fatal("带服务名的规则应该也能按 key 删掉")
	}
}

func TestGuardRedirectLoop(t *testing.T) {
	cases := []struct {
		name      string
		entryHost string
		targetURL string
		wantErr   bool
	}{
		{"正常的子域名入口", "linkstarcf.example.com", "https://example.com:24969", false},
		{"入口和落地同名", "example.com", "https://example.com:24969", true},
		{"同名但大小写不同", "EXAMPLE.com", "https://example.com:24969", true},
		{"同名但入口带根点", "example.com.", "https://example.com:24969", true},
		{"落地回落成公网 IP", "linkstarcf.example.com", "http://1.2.3.4:24969", false},
		{"目标地址解析不了就别拦", "example.com", "://bad", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := guardRedirectLoop(c.entryHost, c.targetURL)
			if c.wantErr && err == nil {
				t.Fatalf("应该拦下来，却放过了")
			}
			if !c.wantErr && err != nil {
				t.Fatalf("不该拦，却报了: %v", err)
			}
		})
	}
}

// TestShortCause 服务商报错会把整个响应体带回来，压成一行短句再给用户看
func TestShortCause(t *testing.T) {
	long := `返回内容：{"success":false,"errors":[{"code":10000,"message":"Authentication error",` +
		`"documentation_url":"https://developers.cloudflare.com/api/resources/dns/subresources/records/methods/list"}],` +
		`"messages":[],"result":null}` + "\n ,返回状态码:403"

	got := shortCause(long)
	if strings.ContainsAny(got, "\n\r") {
		t.Fatalf("压完还带换行，贴进界面会撑开一大块: %q", got)
	}
	if n := len([]rune(got)); n > 121 {
		t.Fatalf("压完还有 %d 个字，太长了: %q", n, got)
	}
	// 头部得留住：这是用户拿去搜的那半句
	if !strings.HasPrefix(got, "返回内容：") {
		t.Fatalf("开头被截没了: %q", got)
	}

	if got := shortCause("短原因"); got != "短原因" {
		t.Fatalf("本来就短的不该改动: %q", got)
	}
}

// ==== 多个服务同时往一份规则集里写 ====

// cfFakeAPI 够跑通一次 SyncRedirectRule 的假 Cloudflare：zone 查询、
// 入口域名记录查询、规则集读写。规则集是有状态的，写进去下次就读得到。
type cfFakeAPI struct {
	mu        sync.Mutex
	rules     []cfRule
	readDelay time.Duration
}

func (f *cfFakeAPI) handler(t *testing.T) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("content-type", "application/json")
		switch {
		case r.URL.Path == "/zones":
			fmt.Fprint(w, `{"success":true,"result":[{"id":"z1","name":"example.com"}]}`)

		case strings.HasSuffix(r.URL.Path, "/dns_records"):
			// 入口域名那条记录已经建好也开着小黄云，这个测试不关心它
			fmt.Fprint(w, `{"success":true,"result":[{"id":"rec1","type":"A",`+
				`"content":"192.0.2.1","proxied":true,"ttl":1,"comment":"linkstar:entry"}]}`)

		case strings.HasSuffix(r.URL.Path, "/entrypoint") && r.Method == http.MethodGet:
			f.mu.Lock()
			raw, err := json.Marshal(f.rules)
			f.mu.Unlock()
			if err != nil {
				t.Errorf("规则集序列化失败: %v", err)
			}
			// 快照取完再拖一会儿才回：没有锁的话，几个请求就都拿到同一份旧快照，
			// 正是开机时三个服务同时同步的样子
			time.Sleep(f.readDelay)
			fmt.Fprintf(w, `{"success":true,"result":{"id":"rs1","rules":%s}}`, raw)

		case strings.HasSuffix(r.URL.Path, "/entrypoint"):
			var body cfRulesetPut
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Errorf("PUT 的 body 解析不了: %v", err)
			}
			f.mu.Lock()
			f.rules = body.Rules
			f.mu.Unlock()
			fmt.Fprint(w, `{"success":true,"result":{"id":"rs1","rules":[]}}`)

		default:
			t.Errorf("测试没预料到的请求: %s %s", r.Method, r.URL.Path)
			http.NotFound(w, r)
		}
	}
}

// TestSyncRedirectRuleConcurrentWrites 几个服务同时同步，谁的端口都不能被别人按回去。
//
// 「界面显示成功、Cloudflare 那边还是旧端口」还有第二个来源，0.6.5 才修掉：
// 改一条规则要整份读回来再整份写回去，两个服务的读-改-写一交错，
// 后写的那份带着前一个的旧端口，把刚写好的盖掉。被盖的那边收到的是 200，
// 于是记下「已经是新端口了」，之后地址没变就再也不同步——永远不会自己纠正。
// 开机时所有洞几乎同时打通，必撞。
func TestSyncRedirectRuleConcurrentWrites(t *testing.T) {
	fake := &cfFakeAPI{readDelay: 50 * time.Millisecond}
	srv := httptest.NewServer(fake.handler(t))
	defer srv.Close()

	old := zonesAPI
	zonesAPI = srv.URL + "/zones"
	t.Cleanup(func() { zonesAPI = old })

	// 三个服务各自的入口域名和当前外网端口
	type svc struct{ key, entry, target string }
	services := []svc{
		{"linkstar:1-1", "linkstar.example.com", "https://example.com:27096"},
		{"linkstar:2-1", "fn.example.com", "https://example.com:27094"},
		{"linkstar:2-4", "blog.example.com", "https://example.com:26297"},
	}

	cf := NewCloudflare("test-token")
	var wg sync.WaitGroup
	for _, s := range services {
		wg.Add(1)
		go func(s svc) {
			defer wg.Done()
			if _, _, err := cf.SyncRedirectRule("example.com", s.key, "", s.entry, s.target); err != nil {
				t.Errorf("%s 同步失败: %v", s.key, err)
			}
		}(s)
	}
	wg.Wait()

	fake.mu.Lock()
	got := fake.rules
	fake.mu.Unlock()

	if len(got) != len(services) {
		t.Fatalf("三个服务各一条，Cloudflare 那边应该有 %d 条，实际 %d 条", len(services), len(got))
	}
	byKey := make(map[string]cfRule, len(got))
	for _, r := range got {
		byKey[ruleKeyOf(r)] = r
	}
	for _, s := range services {
		r, ok := byKey[s.key]
		if !ok {
			t.Errorf("%s 的规则被别的服务写没了", s.key)
			continue
		}
		if !strings.Contains(string(r["action_parameters"]), s.target) {
			t.Errorf("%s 的端口被别的服务按回旧值了：%s", s.key, r["action_parameters"])
		}
	}
}

// TestWarnEntryRecordNoPerm 这句话是给人照着做的，域名和占位地址都得在里面
func TestWarnEntryRecordNoPerm(t *testing.T) {
	msg := warnEntryRecordNoPerm("linkstarcf.example.com")
	for _, want := range []string{"linkstarcf.example.com", entryRecordIP, "DNS"} {
		if !strings.Contains(msg, want) {
			t.Fatalf("提示里缺了 %q: %s", want, msg)
		}
	}
}
