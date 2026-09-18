package auth_api

import (
	"linkstar/middleware"
	"linkstar/modules/auth"
	"linkstar/utils/res"

	"github.com/gin-gonic/gin"
)

// StatusView 公开：返回是否已初始化，前端据此决定显示登录页还是引导页。
// authed 是当前这次请求的登录态——导航主页是公开的，但改配置要登录，
// 它靠这个字段决定「设置」按钮是打开面板还是先去登录。
func (AuthApi) StatusView(c *gin.Context) {
	res.OkWithData(gin.H{
		"initialized": auth.Runtime.IsInitialized(),
		"authed":      middleware.IsAuthed(c),
	}, c)
}
