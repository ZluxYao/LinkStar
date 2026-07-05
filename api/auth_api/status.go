package auth_api

import (
	"linkstar/modules/auth"
	"linkstar/utils/res"

	"github.com/gin-gonic/gin"
)

// StatusView 公开：返回是否已初始化，前端据此决定显示登录页还是引导页
func (AuthApi) StatusView(c *gin.Context) {
	res.OkWithData(gin.H{
		"initialized": auth.Runtime.IsInitialized(),
	}, c)
}
