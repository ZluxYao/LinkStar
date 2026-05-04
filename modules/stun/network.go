package stun

import (
	"context"
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

	//如果发生网络变化更新NatRouter
	if Runtime.Network.LocalIP != addrInfo.LocalIP || Runtime.Network.PublicIP != addrInfo.PublicIP {
		// 更新网络信息
		Runtime.Network.LocalIP = addrInfo.LocalIP
		Runtime.Network.PublicIP = addrInfo.PublicIP

		// 更新NatRouter
		go updateNatRouter()
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
