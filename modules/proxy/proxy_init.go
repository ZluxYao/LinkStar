package proxy

import (
	"fmt"

	"linkstar/core"

	"github.com/sirupsen/logrus"
)

// InitProxy 读配置 → 建站点表 → 开监听
func InitProxy() error {
	cfg, err := ReadConfig()
	if err != nil {
		return fmt.Errorf("读取反向代理配置失败: %w", err)
	}

	Runtime.mu.Lock()
	Runtime.Config = cfg
	Runtime.Router = NewRouter(cfg.Sites)
	Runtime.mu.Unlock()

	core.OnShutdown(func() {
		Runtime.pool.StopAll()
		if err := SaveConfig(Runtime.Snapshot()); err != nil {
			logrus.Error("保存反向代理配置失败：", err)
		}
	})

	// 监听起不来不算模块初始化失败：站点配置都还在，用户到页面上
	// 换个端口就能救回来。原因已经记进状态里，页面上看得到。
	if err := Runtime.pool.Apply(cfg); err != nil {
		logrus.Error("[proxy] ", err)
	}

	if n := len(Runtime.CurrentRouter().Hosts()); n > 0 {
		logrus.Infof("[proxy] 已加载 %d 个站点", n)
	}
	return nil
}
