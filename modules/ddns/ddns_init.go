package ddns

import (
	"fmt"
	"linkstar/core"

	"github.com/sirupsen/logrus"
	"golang.org/x/sync/errgroup"
)

func DDNSInit() error {
	var err error

	// 读取 STUN 配置文件
	Runtime.Config, err = ReadConfig()
	if err != nil {
		return fmt.Errorf("读取 DDNS 配置失败: %w", err)
	}

	// 监听退出保存配置文件
	core.OnShutdown(func() {
		if err := SaveConfig(Runtime.Snapshot()); err != nil {
			logrus.Error("保存 DDNS 配置失败：", err)
		}
	})

	// 并发初始化
	var g errgroup.Group

	// 1. 初始化WebIpSource
	g.Go(func() error {
		initDefaultIPSources()
		return nil
	})

	// 等待基础运行时准备完成
	if err := g.Wait(); err != nil {
		return fmt.Errorf("初始化 DDNS 运行时失败: %w", err)
	}

	Runtime.Scheduler = NewScheduler()
	Runtime.Scheduler.Start()
	Runtime.Scheduler.Trigger()

	return nil
}
