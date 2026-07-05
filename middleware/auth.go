package middleware

import (
	"strings"

	"linkstar/modules/auth"
	"linkstar/utils/res"

	"github.com/gin-gonic/gin"
)

// DesktopSecretHeader 桌面 webview 注入的免登录 secret 头
const DesktopSecretHeader = "X-LinkStar-Desktop"

// AuthMiddleware 保护受限接口：桌面 secret 或有效 token 之一通过即放行。
// 未初始化返回 428 引导前端去设置密码；否则 401。
func AuthMiddleware(c *gin.Context) {
	if !auth.Runtime.IsInitialized() {
		res.FailNeedInit(c)
		c.Abort()
		return
	}

	// 桌面免登录通道：仅 wails webview 携带 secret，CLI 构建 secret 为空永不匹配
	if secret := c.GetHeader(DesktopSecretHeader); secret != "" && auth.MatchDesktopSecret(secret) {
		c.Next()
		return
	}

	// token 校验：Authorization: Bearer <token>
	if token := bearerToken(c); token != "" && auth.Runtime.ValidateToken(token) {
		c.Next()
		return
	}

	res.FailUnauthorized(c)
	c.Abort()
}

func bearerToken(c *gin.Context) string {
	h := c.GetHeader("Authorization")
	const prefix = "Bearer "
	if strings.HasPrefix(h, prefix) {
		return strings.TrimSpace(h[len(prefix):])
	}
	return ""
}
