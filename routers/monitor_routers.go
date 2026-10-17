package routers

import (
	"linkstar/api"

	"github.com/gin-gonic/gin"
)

// MonitorRouters 注册运行监控接口。
func MonitorRouters(g *gin.RouterGroup) {
	g.GET("monitor/observability", api.App.MonitorApi.GetSystemObservabilityView)
}
