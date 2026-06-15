package ddns_api

import (
	"linkstar/middleware"
	"linkstar/modules/ddns"
	"linkstar/modules/ddns/model"
	"linkstar/utils/res"

	"github.com/gin-gonic/gin"
)

type DdnsSettingsUpdateRequest struct {
	Enabled     bool `json:"enabled"`
	IntervalSec int  `json:"intervalSec"`
}

// DdnsSettingsUpdateView 更新总开关与全局同步间隔
func (DdnsApi) DdnsSettingsUpdateView(c *gin.Context) {
	cr := middleware.GetBindRequest[DdnsSettingsUpdateRequest](c)

	if cr.IntervalSec < 30 {
		res.FailWithMsg("同步间隔不能小于 30 秒", c)
		return
	}

	var toggled bool
	err := ddns.Runtime.Apply(func(cfg *model.DDNSConfig) error {
		toggled = cfg.Enabled != cr.Enabled
		cfg.Enabled = cr.Enabled
		cfg.IntervalSec = cr.IntervalSec
		return nil
	})
	if err != nil {
		res.FailWithMsg("保存配置失败", c)
		return
	}

	// 总开关切换：重建调度器（开 -> 启动，关 -> 停止）
	if toggled {
		ddns.Runtime.RebuildScheduler()
	}

	res.OkWithMsg("设置已更新", c)
}
