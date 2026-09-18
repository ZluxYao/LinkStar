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

func SystemRoutersProtected(g *gin.RouterGroup) {
	var app = api.App.SystemApi

	// 运行日志：日志里有内网地址、域名、后端错误，不能公开
	g.GET("system/log/days", app.LogDaysView)
	g.GET("system/log", app.LogListView)
}
