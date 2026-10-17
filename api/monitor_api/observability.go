package monitor_api

import (
	"linkstar/modules/monitor"
	"linkstar/utils/res"

	"github.com/gin-gonic/gin"
)

// GetSystemObservabilityView 返回当前主机和 LinkStar 进程的资源快照。
func (MonitorApi) GetSystemObservabilityView(c *gin.Context) {
	res.OkWithData(monitor.GetSystemSnapshot(), c)
}
