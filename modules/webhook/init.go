package webhook

import (
	"fmt"
	"linkstar/core"
	"time"

	"github.com/sirupsen/logrus"
)

// InitWebhook 初始化 webhook 模块配置，并注册退出时持久化
func InitWebhook() error {
	cfg, err := ReadConfig()
	if err != nil {
		return fmt.Errorf("读取 webhook 配置失败: %w", err)
	}
	Runtime.mu.Lock()
	Runtime.Config = cfg
	Runtime.mu.Unlock()

	core.OnShutdown(func() {
		Runtime.mu.RLock()
		snapshot := Runtime.Config
		Runtime.mu.RUnlock()
		snapshot.UpdatedAt = time.Now()
		if err := SaveConfig(snapshot); err != nil {
			logrus.Error("保存 webhook 配置失败：", err)
		}
	})
	return nil
}
