package stun

import (
	"context"
	"fmt"
	"time"

	"github.com/sirupsen/logrus"
	"golang.org/x/sync/errgroup"
)

func InitSTUN() error {
	var err error

	// 读取 STUN 配置文件
	Runtime.Config, err = ReadConfig()
	if err != nil {
		return fmt.Errorf("读取 STUN 配置失败: %w", err)
	}

	// 初始化调度器
	Runtime.Scheduler = NewScheduler(
		NewSTUNTunnelRunner(),
		func() TunnelEnvironment {
			return TunnelEnvironment{
				LocalIP:  Runtime.Network.LocalIP,
				BestSTUN: Runtime.Config.BestSTUN,
			}
		},
	)

	// 监听退出保存配置文件
	go SetupShutdownHook(func() {
		syncRuntimeConfig()
		if err := UpdateConfig(Runtime.Config); err != nil {
			logrus.Error("保存配置失败：", err)
		}
	})

	var g errgroup.Group

	// 1. 初始化 STUN 服务，获取最快的 STUN 服务器
	g.Go(func() error {
		Runtime.STUNService = NewSTUNService(Runtime.Config.StunServerList)
		Runtime.Config.BestSTUN = Runtime.STUNService.BestSTUNServer
		logrus.Info("当前使用stun服务器：", Runtime.Config.BestSTUN)

		if Runtime.Config.BestSTUN == "" {
			return fmt.Errorf("没有可用的 STUN 服务器")
		}
		return nil
	})

	// 2. 获取 NAT 路由列表
	g.Go(func() error {
		natRouterList, err := GetNatRouterList()
		if err != nil {
			return fmt.Errorf("获取 NAT Router 列表失败: %w", err)
		}
		Runtime.Network.NatRouterList = natRouterList
		logrus.Info("当前NAT Router：", Runtime.Network.NatRouterList)
		return nil
	})

	// 3. 发现 UPnP 设备并选择默认网关
	g.Go(func() error {
		gateway := DiscoverUPnPGateway()
		SelectDefaultGateway(gateway)
		Runtime.UpnpGateway = gateway
		return nil
	})

	// 等待基础运行时准备完成
	if err := g.Wait(); err != nil {
		return fmt.Errorf("初始化 STUN 运行时失败: %w", err)
	}

	// 4. 获取网络地址信息，依赖上面选出的 STUN 服务器
	addrInfo, err := GetPublicIPInfo(Runtime.Config.BestSTUN)
	if err != nil {
		return fmt.Errorf("获取网络地址信息失败: %w", err)
	}
	Runtime.Network.LocalIP = addrInfo.LocalIP
	Runtime.Network.PublicIP = addrInfo.PublicIP

	syncRuntimeConfig()

	// 5. 启动网络信息更新器
	go RunNetworkRuntimeUpdater(context.Background())

	// 6. 启动所有 STUN 服务映射
	go Runtime.Scheduler.StartAll(Runtime.Config.Devices)

	return nil
}

func syncRuntimeConfig() {
	if Runtime.STUNService != nil && Runtime.STUNService.BestSTUNServer != "" {
		Runtime.Config.BestSTUN = Runtime.STUNService.BestSTUNServer
	}
	Runtime.Config.UpdatedAt = time.Now()
}
