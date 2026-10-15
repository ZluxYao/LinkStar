package stun

import (
	"linkstar/modules/stun/model"
	"sync"
)

// networkMu 保护 Runtime.Network。DDNS、调度器和周期刷新会同时读写真公网 IP 和 NAT 链路。
var networkMu sync.RWMutex

// PublicIP 返回当前公网 IP 的副本。
func PublicIP() string {
	networkMu.RLock()
	defer networkMu.RUnlock()
	return Runtime.Network.PublicIP
}

// LocalIP 返回出口网卡地址的副本。
func LocalIP() string {
	networkMu.RLock()
	defer networkMu.RUnlock()
	return Runtime.Network.LocalIP
}

// NetworkSnapshot 返回网络状态的副本，调用方改切片不会影响运行时。
func NetworkSnapshot() model.NetworkState {
	networkMu.RLock()
	defer networkMu.RUnlock()
	n := Runtime.Network
	if n.NatRouterList != nil {
		n.NatRouterList = append([]model.NatRouterInfo(nil), n.NatRouterList...)
	}
	// 指针字段也要复制。否则调用方改快照里的 NAT 结果，会直接改到运行时。
	if n.NatStatuUDP != nil {
		udp := *n.NatStatuUDP
		n.NatStatuUDP = &udp
	}
	if n.NatStatuTCP != nil {
		tcp := *n.NatStatuTCP
		n.NatStatuTCP = &tcp
	}
	return n
}

func setObservedAddress(local, public string) (publicChanged bool) {
	networkMu.Lock()
	defer networkMu.Unlock()
	publicChanged = Runtime.Network.PublicIP != public
	Runtime.Network.LocalIP = local
	Runtime.Network.PublicIP = public
	return publicChanged
}

func setLocalIP(local string) {
	networkMu.Lock()
	Runtime.Network.LocalIP = local
	networkMu.Unlock()
}

func setNatRouters(list []model.NatRouterInfo) {
	networkMu.Lock()
	defer networkMu.Unlock()
	Runtime.Network.NatRouterList = append([]model.NatRouterInfo(nil), list...)
}
