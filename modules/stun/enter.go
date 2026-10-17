package stun

import (
	"sync"

	"linkstar/modules/stun/model"
)

type STUNRuntime struct {
	Config      model.Config
	networkMu   sync.RWMutex
	Network     model.NetworkState
	STUNService *STUNService
	UpnpGateway *model.UpnpGateway
	Scheduler   *Scheduler
}

var Runtime = &STUNRuntime{}

// NetworkSnapshot 返回网络运行时的副本，避免接口把内部切片和 NAT 结果指针交出去。
func (r *STUNRuntime) NetworkSnapshot() model.NetworkState {
	r.networkMu.RLock()
	defer r.networkMu.RUnlock()

	state := r.Network
	state.NatRouterList = append([]model.NatRouterInfo(nil), r.Network.NatRouterList...)
	if r.Network.NatStatuUDP != nil {
		value := *r.Network.NatStatuUDP
		state.NatStatuUDP = &value
	}
	if r.Network.NatStatuTCP != nil {
		value := *r.Network.NatStatuTCP
		state.NatStatuTCP = &value
	}
	return state
}

func (r *STUNRuntime) updateNetwork(mutator func(*model.NetworkState)) {
	r.networkMu.Lock()
	defer r.networkMu.Unlock()
	mutator(&r.Network)
}
