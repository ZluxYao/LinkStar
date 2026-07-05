package routers

import (
	"linkstar/api"
	"linkstar/api/auth_api"
	"linkstar/middleware"

	"github.com/gin-gonic/gin"
)

func AuthRouters(g *gin.RouterGroup) {
	var app = api.App.AuthApi

	// 公开：是否已初始化
	g.GET("auth/status", app.StatusView)

	// 公开：首次设置密码（内部限制仅未初始化可用）
	g.POST("auth/setup",
		middleware.BindJsonMiddleware[auth_api.SetupRequest], app.SetupView)

	// 公开：登录
	g.POST("auth/login",
		middleware.BindJsonMiddleware[auth_api.LoginRequest], app.LoginView)

	// 受保护：修改密码
	protected := g.Group("", middleware.AuthMiddleware)
	protected.PUT("auth/password",
		middleware.BindJsonMiddleware[auth_api.ChangePasswordRequest], app.ChangePasswordView)
}
