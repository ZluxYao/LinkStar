package ddns

import (
	"fmt"

	"github.com/sirupsen/logrus"
)

func DDNSInit() error {
	var err error

	// 读取 STUN 配置文件
	Runtime.Config, err = ReadConfig()
	if err != nil {
		return fmt.Errorf("读取 STUN 配置失败: %w", err)
	}

	// 初始化调度器
	// Runtime.Scheduler = NewScheduler(NewSTUNRunner())

	// 监听退出保存配置文件
	go SetupShutdownHook(func() {
		if err := UpdateConfig(Runtime.Config); err != nil {
			logrus.Error("保存DDSN配置失败：", err)
		}
	})

	return nil
}
