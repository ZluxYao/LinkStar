package stun

import (
	"context"
	"linkstar/modules/stun/model"
	"linkstar/modules/webhook"
)

// STUN 需要的参数
type STUNRequest struct {
	ServiceName   string
	TargetIP      string
	InternalPort  uint16
	Protocol      string
	UseUPnP       bool
	WebhookConfig webhook.WebhookConfig

	// Redirect 入口域名跟着外部端口走的配置
	Redirect model.RedirectConfig

	// TLSTerminate 是否由 LinkStar 在洞口终结 TLS（仅 TCP 有效）
	TLSTerminate bool
	// CertID 绑定的证书；0 表示按 SNI / 默认证书自动匹配
	CertID uint
	// BackendHTTPS 拨内网时用 tls.Dial（等价 nginx 的 proxy_pass https://），只在终结 TLS 时成立
	BackendHTTPS bool
}

// 定义STUN 状态类型
type STUNStateType int

const (
	STUNMapped STUNStateType = iota
	STUNAlive
	STUNFailed
	STUNLog
)

// 当前STUN 服务的状态
type STUNState struct {
	State        STUNStateType // 状态
	ExternalIP   string        // 外部 IP
	ExternalPort uint16        // 外部端口
	Log          string        // 日志
}

type Runner interface {
	Run(ctx context.Context, req STUNRequest, onState func(STUNState)) error
}

// STUN Runner
type STUNRunner struct {
}

// 创建STUN Runner
func NewSTUNRunner() Runner {
	return STUNRunner{}
}
