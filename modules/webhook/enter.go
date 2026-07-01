package webhook

import "sync"

// WebhookRuntime 保存 webhook 模块的运行期配置
type WebhookRuntime struct {
	mu     sync.RWMutex
	Config Config
}

// Runtime webhook 模块全局运行时
var Runtime = &WebhookRuntime{}
