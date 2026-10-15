package stun

import (
	"sync"
	"testing"

	"linkstar/modules/stun/model"
)

func Test网络快照不会共享NAT结果和路由切片(t *testing.T) {
	networkMu.Lock()
	old := Runtime.Network
	Runtime.Network = model.NetworkState{
		NatStatuUDP:   &model.NatDetectResult{NatType: "原始类型"},
		NatStatuTCP:   &model.NatDetectResult{NatType: "原始类型"},
		NatRouterList: []model.NatRouterInfo{{LanIp: "192.0.2.1"}},
	}
	networkMu.Unlock()
	t.Cleanup(func() {
		networkMu.Lock()
		Runtime.Network = old
		networkMu.Unlock()
	})

	snap := NetworkSnapshot()
	snap.NatStatuUDP.NatType = "被调用方修改"
	snap.NatStatuTCP.NatType = "被调用方修改"
	snap.NatRouterList[0].LanIp = "192.0.2.2"

	again := NetworkSnapshot()
	if again.NatStatuUDP.NatType != "原始类型" || again.NatStatuTCP.NatType != "原始类型" || again.NatRouterList[0].LanIp != "192.0.2.1" {
		t.Fatalf("快照被改写后运行时发生变化: %+v", again)
	}

	list := []model.NatRouterInfo{{LanIp: "192.0.2.3"}}
	setNatRouters(list)
	list[0].LanIp = "192.0.2.4"
	if got := NetworkSnapshot().NatRouterList[0].LanIp; got != "192.0.2.3" {
		t.Fatalf("路由更新保留了调用方的切片: %q", got)
	}
}

func Test公网IP的读写不会互相踩(t *testing.T) {
	networkMu.Lock()
	old := Runtime.Network
	Runtime.Network = model.NetworkState{}
	networkMu.Unlock()
	t.Cleanup(func() {
		networkMu.Lock()
		Runtime.Network = old
		networkMu.Unlock()
	})

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for n := 0; n < 200; n++ {
				setObservedAddress("192.0.2.10", "203.0.113.10")
				if ip := PublicIP(); ip != "203.0.113.10" {
					t.Errorf("读到意外的公网 IP %q", ip)
				}
				snap := NetworkSnapshot()
				if snap.LocalIP != "192.0.2.10" || snap.PublicIP != "203.0.113.10" {
					t.Errorf("读到不完整的地址快照: %+v", snap)
				}
			}
		}()
	}
	wg.Wait()
}
