package auth

import (
	"crypto/subtle"
	"sync"
)

// Config 鉴权配置，持久化到 config/authConfig.json
type Config struct {
	Initialized   bool   `json:"initialized"`   // 是否已设置过密码
	PasswordHash  string `json:"passwordHash"`  // pwd.HashPassword 产物
	JwtSecret     string `json:"jwtSecret"`     // 首次启动随机生成，用于签发/校验 token
	TokenTTLHours int    `json:"tokenTtlHours"` // token 有效期（小时）
	UpdatedAt     string `json:"updatedAt"`
}

type AuthRuntime struct {
	mu     sync.RWMutex
	Config Config
}

var Runtime = &AuthRuntime{}

// desktopSecret 桌面版启动时注入，仅 webview 携带；CLI 构建保持空。
var desktopSecret string

// SetDesktopSecret 由桌面构建在启动时调用注入本地 secret。
func SetDesktopSecret(s string) {
	desktopSecret = s
}

// MatchDesktopSecret 常数时间比较请求携带的 secret；空 secret 永不匹配。
func MatchDesktopSecret(s string) bool {
	if desktopSecret == "" || s == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(desktopSecret), []byte(s)) == 1
}
