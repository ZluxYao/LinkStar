package ddns_api

import (
	"linkstar/middleware"
	"linkstar/modules/ddns"
	"linkstar/utils/res"

	"github.com/gin-gonic/gin"
)

type DdnsSettingsUpdateRequest struct {
	IntervalSec int `json:"intervalSec"`
}

// DdnsSettingsUpdateView 更新全局同步间隔
func (DdnsApi) DdnsSettingsUpdateView(c *gin.Context) {
	cr := middleware.GetBindRequest[DdnsSettingsUpdateRequest](c)

	if err := ddns.Runtime.UpdateSettings(cr.IntervalSec); err != nil {
		res.FailWithMsg(err.Error(), c)
		return
	}

	res.OkWithMsg("设置已更新", c)
}
