package ddns_api

import (
	"linkstar/middleware"
	"linkstar/modules/ddns"
	"linkstar/utils/res"

	"github.com/gin-gonic/gin"
)

type DdnsProviderUpdateRequest struct {
	ID         uint              `json:"id"`
	Name       string            `json:"name"`
	Credential map[string]string `json:"credential"` // 留空字段表示不修改
}

func (DdnsApi) DdnsProviderUpdateView(c *gin.Context) {
	cr := middleware.GetBindRequest[DdnsProviderUpdateRequest](c)

	if err := ddns.Runtime.UpdateProvider(cr.ID, cr.Name, cr.Credential); err != nil {
		res.FailWithMsg(err.Error(), c)
		return
	}

	res.OkWithMsg("服务商已更新", c)
}
