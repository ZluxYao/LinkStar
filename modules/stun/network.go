package stun

import (
	"context"
	"linkstar/modules/stun/model"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
)

// 运行网络运行时信息更新器
func RunNetworkRuntimeUpdater(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			refreshNetworkRuntime()

		}

	}

}

// 更新网络信息
func refreshNetworkRuntime() {
	refreshOutboundIface()

	// 获取STUN服务器
	stunServer, err := Runtime.STUNService.GetBestSTUNServer()
	if err != nil {
		logrus.Warnf("获取最佳 STUN 服务器失败: %v", err)
	}

	if stunServer == "" {
		stunServer, err = Runtime.STUNService.GetBackupSTUNServer()
		if err != nil {
			logrus.Errorf("获取备用 STUN 服务器失败: %v", err)
			return
		}
	}

	// 更新网络信息
	if updateNetworkAddress(stunServer) {
		return
	}

	// 获取备用 STUN 服务器
	backupServer, err := Runtime.STUNService.GetBackupSTUNServer()
	if err != nil {
		logrus.Errorf("刷新网络信息失败，且获取备用 STUN 服务器失败: %v", err)
		return
	}

	if !updateNetworkAddress(backupServer) {
		logrus.Errorf("使用备用 STUN 服务器刷新网络信息失败: %s", backupServer)
	}
}

// 更新Runtime的网络信息
func updateNetworkAddress(stunServer string) bool {
	if stunServer == "" {
		return false
	}

	addrInfo, err := GetPublicIPInfo(stunServer)
	if err != nil {
		logrus.Warnf("通过 STUN 服务器获取网络地址失败: server=%s err=%v", stunServer, err)
		return false
	}

	logrus.Infof("通过 STUN 服务器 %s 获取到的网络地址: LocalIP=%s, PublicIP=%s", stunServer, addrInfo.LocalIP, addrInfo.PublicIP)

	// 如果公网ip是10网段，就失败（部分 STUN 服务器会错误地返回内网地址）
	if strings.HasPrefix(addrInfo.PublicIP, "10.") {
		logrus.Warnf("STUN 服务器 %s 返回的公网 IP 属于 10 网段，视为失败: %s", stunServer, addrInfo.PublicIP)

		return false
	}

	//如果发生网络变化更新NatRouter
	if Runtime.Network.LocalIP != addrInfo.LocalIP || Runtime.Network.PublicIP != addrInfo.PublicIP {
		publicChanged := Runtime.Network.PublicIP != addrInfo.PublicIP

		// 更新网络信息
		Runtime.Network.LocalIP = addrInfo.LocalIP
		Runtime.Network.PublicIP = addrInfo.PublicIP

		// 更新NatRouter
		go updateNatRouter()

		// 家宽 IP 换了，域名还指着上一个，立刻叫 DDNS 推一次，别等下个周期
		if publicChanged {
			go EmitPublicIPChanged(addrInfo.PublicIP)
		}
	}

	return true
}

// 更新NATRouter
func updateNatRouter() {
	natRouterList, err := GetNatRouterList()
	if err != nil {
		logrus.Warnf("更新 NAT Router 失败: %v", err)
		return
	}
	Runtime.Network.NatRouterList = natRouterList
}

// refreshOutboundIface 出口网卡换了（拔网线切 Wi-Fi、DHCP 换地址、改了网络设置）：
// 打洞的套接字还绑在旧地址上，UPnP 映射也指着旧 IP，自己修不好，
// 只能换上新出口、重选 UPnP 网关、把打洞服务全部重启。
func refreshOutboundIface() {
	cur, err := DetectOutboundIface()
	if err != nil {
		logrus.Warnf("获取出口网卡失败: %v", err)
		return // 断网时什么都不动，等网络回来再比
	}
	old := currentOutboundIface.Swap(&cur)
	if old == nil || old.same(cur) {
		return
	}

	logrus.Warnf("出口网卡变化：%s → %s，重新选 UPnP 网关并重启打洞服务", old, cur)
	gateway := DiscoverUPnPGateway()
	SelectDefaultGateway(gateway)
	Runtime.UpnpGateway = gateway

	// 公网 IP 交给紧接着的那一轮去问；LocalIP 在这里先换掉，
	// 不然下面重启的服务还会绑回旧地址
	Runtime.Network.LocalIP = cur.LocalIP
	Runtime.Network.Iface, Runtime.Network.Gateway = cur.Name, cur.Gateway
	go updateNatRouter()
	if Runtime.Scheduler != nil {
		go Runtime.Scheduler.StartAll(Runtime.Config.Devices)
	}
}

// ApplyNetworkConfig 换上新的网络设置，返回按它挑出来的出口。
//
// 只有挑网卡是当场做的（很快）；出口真变了的话，重选 UPnP 网关（发现要好几秒）、
// 重启打洞服务、重查公网 IP 都放到后台，不让保存按钮转圈。
// 新设置下挑不出网卡（指定的网卡没连上）时返回错误，但设置已经生效 ——
// 等那张卡连上，下一轮更新器自己会切过去。
func ApplyNetworkConfig(cfg model.NetworkConfig) (OutboundIface, error) {
	networkConfig.Store(&cfg)
	o, err := DetectOutboundIface()
	if err != nil {
		return OutboundIface{}, err
	}
	go func() {
		if Runtime.STUNService == nil {
			refreshOutboundIface() // STUN 模块没起来，起码把出口换上
			return
		}
		refreshNetworkRuntime()
	}()
	return o, nil
}
