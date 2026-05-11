package stun

import "sync"

// ServiceDeletedHandler 当 stun service 被删除时触发
type ServiceDeletedHandler func(deviceID, serviceID uint)

var (
	hooksMu                sync.RWMutex
	onServiceDeletedHooks  []ServiceDeletedHandler
)

// RegisterOnServiceDeleted 由外部模块（如 home）调用，注册删除回调
// stun 不感知调用方，避免循环 import
func RegisterOnServiceDeleted(h ServiceDeletedHandler) {
	if h == nil {
		return
	}
	hooksMu.Lock()
	onServiceDeletedHooks = append(onServiceDeletedHooks, h)
	hooksMu.Unlock()
}

// EmitServiceDeleted 通知所有订阅者
func EmitServiceDeleted(deviceID, serviceID uint) {
	hooksMu.RLock()
	handlers := make([]ServiceDeletedHandler, len(onServiceDeletedHooks))
	copy(handlers, onServiceDeletedHooks)
	hooksMu.RUnlock()

	for _, h := range handlers {
		h(deviceID, serviceID)
	}
}
