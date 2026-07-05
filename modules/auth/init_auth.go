package auth

import "fmt"

// InitAuth 读配置进 Runtime。中间件依赖此配置，未就绪时零值 Initialized=false，
// 走 fail-closed（拒绝受保护请求 / 引导设置流程），安全。
func InitAuth() error {
	cfg, err := ReadConfig()
	if err != nil {
		return fmt.Errorf("读取 Auth 配置失败: %w", err)
	}
	Runtime.mu.Lock()
	Runtime.Config = cfg
	Runtime.mu.Unlock()
	return nil
}
