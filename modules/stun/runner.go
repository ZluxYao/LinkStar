package stun

import "context"

type TunnelEnvironment struct {
	LocalIP  string
	BestSTUN string
}

type TunnelRequest struct {
	ServiceName  string
	TargetIP     string
	InternalPort uint16
	Protocol     string
	UseUPnP      bool
	Environment  TunnelEnvironment
}

type TunnelEventType int

const (
	TunnelMapped TunnelEventType = iota
	TunnelAlive
)

type TunnelEvent struct {
	Type         TunnelEventType
	ExternalPort uint16
}

type TunnelRunner interface {
	Run(ctx context.Context, req TunnelRequest, onEvent func(TunnelEvent)) error
}

type TunnelEnvironmentProvider func() TunnelEnvironment

type STUNTunnelRunner struct{}

func NewSTUNTunnelRunner() TunnelRunner {
	return STUNTunnelRunner{}
}
