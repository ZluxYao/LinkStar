package home

import (
	"fmt"
	"os"

	"github.com/sirupsen/logrus"
)

// InitHome 在 stun 初始化之后调用：读配置 + 订阅 stun 删除事件
func InitHome() error {
	cfg, err := ReadConfig()
	if err != nil {
		return fmt.Errorf("读取 Home 配置失败: %w", err)
	}
	Runtime.mu.Lock()
	Runtime.Config = cfg
	Runtime.mu.Unlock()

	// 图标目录
	if err := os.MkdirAll("data/icon", 0755); err != nil {
		logrus.Warn("创建 data/icon 失败：", err)
	}

	registerStunHooks()
	return nil
}
