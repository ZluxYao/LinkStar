package stun

import (
	"context"
	"fmt"
	"linkstar/core"
	"sync"
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
	Runtime.Scheduler = NewScheduler(NewSTUNRunner())

	// 监听退出保存配置文件
	core.OnShutdown(func() {
		syncRuntimeConfig()
		if err := UpdateConfig(Runtime.Config); err != nil {
			logrus.Error("保存配置失败：", err)
		}
	})

	// 先认出口网卡：下面测 STUN 服务器、扫路由、选 UPnP 网关、查公网 IP 都从这张卡出去
	networkConfig.Store(&Runtime.Config.Network)
	outbound, err := DetectOutboundIface()
	if err != nil {
		logrus.Warnf("获取出口网卡失败: %v", err)
	}
	currentOutboundIface.Store(&outbound)
	networkMu.Lock()
	Runtime.Network.Iface, Runtime.Network.Gateway = outbound.Name, outbound.Gateway
	networkMu.Unlock()
	logrus.Info("当前出口网卡：", outbound)

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
		setNatRouters(natRouterList)
		logrus.Info("当前NAT Router：", NetworkSnapshot().NatRouterList)
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
		// 114/119 超时或开机时网还没好，这一轮会空。不能就此停掉：
		// 调度器在下面才启动，返回之后没有人再调用 InitSTUN。
		if Runtime.STUNService == nil || Runtime.STUNService.BestSTUNServer == "" {
			go retrySTUNServerSelection()
		}
		return fmt.Errorf("初始化 STUN 运行时失败: %w", err)
	}

	startSTUNRuntime()
	return nil
}

// stunRetryWait 选不出服务器之后隔多久再探一次。测试可改短。
var stunRetryWait = 15 * time.Second

// stunRetryMax 退避上限。翻倍之后如果超过它，就停在这里，不能再长到 8 分钟。
const stunRetryMax = 5 * time.Minute

// nextSTUNRetryWait 把等待时间翻倍，并钳在 5 分钟。240 秒再翻倍会变成 480 秒，这里截成 300 秒。
func nextSTUNRetryWait(wait time.Duration) time.Duration {
	if wait >= stunRetryMax {
		return stunRetryMax
	}
	wait *= 2
	if wait > stunRetryMax {
		return stunRetryMax
	}
	return wait
}

var startSTUNRuntimeOnce sync.Once

// retrySTUNServerSelection 在后台重试选服务器，选到之后再把打洞跑起来。
func retrySTUNServerSelection() {
	wait := stunRetryWait
	for {
		time.Sleep(wait)
		if Runtime.STUNService == nil {
			Runtime.STUNService = NewSTUNService(Runtime.Config.StunServerList)
		} else {
			Runtime.STUNService.UpdateSTUNService()
		}
		Runtime.Config.BestSTUN = Runtime.STUNService.BestSTUNServer
		if Runtime.Config.BestSTUN == "" {
			logrus.Warnf("仍然没有可用的 STUN 服务器，%s 后再试", wait)
			wait = nextSTUNRetryWait(wait)
			continue
		}
		logrus.Infof("STUN 服务器已就绪：%s", Runtime.Config.BestSTUN)
		startSTUNRuntime()
		return
	}
}

// startSTUNRuntime 选出服务器之后才做的事：查公网 IP、周期刷新、拉起洞口。
// 初始化失败后的重试和第一次成功走这里，只执行一次。
func startSTUNRuntime() {
	startSTUNRuntimeOnce.Do(func() {
		finishSTUNStartup()
	})
}

func finishSTUNStartup() {
	// 4. 获取网络地址信息，依赖上面选出的 STUN 服务器
	addrInfo, err := GetPublicIPInfo(Runtime.Config.BestSTUN)
	if err != nil {
		// 服务器已经选出，公网 IP 这一轮失败不该把洞口也停掉。
		// 周期刷新（下面的 RunNetworkRuntimeUpdater）下一轮还会再问。
		logrus.Errorf("获取网络地址信息失败: %v", err)
	} else {
		// 和周期刷新一样持锁写，避免和 DDNS、调度器同时改 Runtime.Network。
		setObservedAddress(addrInfo.LocalIP, addrInfo.PublicIP)
		// DDNS 是和 STUN 并排起的，它先跑那一轮多半赶在这行之前，只能记一次失败。
		// 这里回头叫它一声，就绪了立刻补一次，不用干等到下个周期。
		go EmitPublicIPChanged(addrInfo.PublicIP)
	}

	syncRuntimeConfig()

	// 5. 启动网络信息更新器
	go RunNetworkRuntimeUpdater(context.Background())

	// 6. 异步检测 UDP/TCP NAT 类型
	go func() {
		udpResult, tcpResult, err := DetectAndClassifyNat()
		if err != nil {
			logrus.Warnf("启动时检测 NAT 类型失败: %v", err)
		}
		if udpResult != nil {
			logrus.Infof("UDP NAT检测结果: type=%s mapping=%s filtering=%s", udpResult.NatType, udpResult.Mapping, udpResult.Filtering)
		}
		if tcpResult != nil {
			logrus.Infof("TCP NAT检测结果: type=%s mapping=%s", tcpResult.NatType, tcpResult.Mapping)
		}
	}()

	// 7. 启动所有 STUN 服务映射
	go Runtime.Scheduler.StartAll(Runtime.Config.Devices)
}

func syncRuntimeConfig() {
	if Runtime.STUNService != nil && Runtime.STUNService.BestSTUNServer != "" {
		Runtime.Config.BestSTUN = Runtime.STUNService.BestSTUNServer
	}
	Runtime.Config.UpdatedAt = time.Now()
}
