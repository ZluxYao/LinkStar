package stun

import "context"

// STUN 需要的参数
type STUNRequest struct {
	ServiceName  string
	TargetIP     string
	InternalPort uint16
	Protocol     string
	UseUPnP      bool
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
