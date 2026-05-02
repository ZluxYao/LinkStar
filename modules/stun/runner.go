package stun

import "context"

type STUNRequest struct {
	ServiceName  string
	TargetIP     string
	InternalPort uint16
	Protocol     string
	UseUPnP      bool
}

type STUNStateType int

const (
	STUNMapped STUNStateType = iota
	STUNAlive
	STUNFailed
)

type STUNState struct {
	State        STUNStateType
	ExternalPort uint16
}

type TunnelRunner interface {
	Run(ctx context.Context, req STUNRequest, onState func(STUNState)) error
}

type STUNTunnelRunner struct{}

func NewSTUNTunnelRunner() TunnelRunner {
	return STUNTunnelRunner{}
}
