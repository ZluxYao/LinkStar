package webhook

import "time"

// WebhookConfig 单个 webhook 请求配置
type WebhookConfig struct {
	Enabled         bool `json:"enabled"`
	OnlyWhenChanged bool `json:"onlyWhenChanged"` // 仅在地址和上一次不同时触发

	URL     string `json:"url"`
	Method  string `json:"method"`  // GET/POST/PUT/PATCH/DELETE/HEAD/OPTIONS,默认 GET
	Headers string `json:"headers"` // 多行 "Key: Value"
	Body    string `json:"body"`    // 请求体(仅非 GET 有意义)

	DisableSuccessCheck bool   `json:"disableSuccessCheck"` // 禁用「成功字符串检测」,只看 HTTP 2xx
	SuccessContains     string `json:"successContains"`     // 未禁用时,返回体须包含此串才算成功

	Proxy string `json:"proxy"` // 可选 http(s):// 代理
}

// WebhookTemplate webhook 预置/自定义模板
type WebhookTemplate struct {
	ID          string        `json:"id"`
	Name        string        `json:"name"`
	Description string        `json:"description"`
	Builtin     bool          `json:"builtin"`
	Config      WebhookConfig `json:"config"`
	CreatedAt   time.Time     `json:"createdAt"`
	UpdatedAt   time.Time     `json:"updatedAt"`
}

// Config webhook 模块配置
type Config struct {
	Templates []WebhookTemplate `json:"templates"`
	CreatedAt time.Time         `json:"createdAt"`
	UpdatedAt time.Time         `json:"updatedAt"`
}
