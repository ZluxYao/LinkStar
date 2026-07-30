package routers

import (
	"linkstar/api"

	"github.com/gin-gonic/gin"
)

func SystemRouters(g *gin.RouterGroup) {
	var app = api.App.SystemApi

	// 公开：获取版本号
	g.GET("version", app.GetVersion)
}
