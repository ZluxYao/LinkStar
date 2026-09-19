package stun

import "sync"

// ServiceDeletedHandler 当 stun service 被删除时触发
type ServiceDeletedHandler func(deviceID, serviceID uint)

// PublicIPChangedHandler 公网 IP 第一次探到、或者后来变了的时候调一下
type PublicIPChangedHandler func(ip string)

var (
	hooksMu                sync.RWMutex
	onServiceDeletedHooks  []ServiceDeletedHandler
	onPublicIPChangedHooks []PublicIPChangedHandler
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

// RegisterOnPublicIPChanged 注册「公网 IP 有了 / 变了」的回调。
//
// 给 DDNS 用：开机时两个模块是并排起的，DDNS 先跑一轮的时候 STUN 常常还在挑
// 服务器，取不到 IP 只能记一次失败。没人叫它的话就得干等下一个周期，域名在这
// 段时间里一直指着旧地址。同样，跑着跑着家宽 IP 变了也该立刻推一次。
//
// 和 RegisterOnServiceDeleted 一个路子：stun 不认识调用方，避免循环 import。
func RegisterOnPublicIPChanged(h PublicIPChangedHandler) {
	if h == nil {
		return
	}
	hooksMu.Lock()
	onPublicIPChangedHooks = append(onPublicIPChangedHooks, h)
	hooksMu.Unlock()
}

// EmitPublicIPChanged 通知所有订阅者。空 IP 不通知：那不是「变了」，是「还没有」。
func EmitPublicIPChanged(ip string) {
	if ip == "" {
		return
	}
	hooksMu.RLock()
	handlers := make([]PublicIPChangedHandler, len(onPublicIPChangedHooks))
	copy(handlers, onPublicIPChangedHooks)
	hooksMu.RUnlock()

	for _, h := range handlers {
		h(ip)
	}
}
