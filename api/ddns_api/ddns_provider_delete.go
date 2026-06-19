package ddns_api

import (
	"linkstar/middleware"
	"linkstar/modules/ddns"
	"linkstar/utils/res"

	"github.com/gin-gonic/gin"
)

type DdnsProviderDeleteRequest struct {
	ID uint `json:"id"`
}

func (DdnsApi) DdnsProviderDeleteView(c *gin.Context) {
	cr := middleware.GetBindRequest[DdnsProviderDeleteRequest](c)

	if err := ddns.Runtime.DeleteProvider(cr.ID); err != nil {
		res.FailWithMsg(err.Error(), c)
		return
	}

	res.OkWithMsg("服务商已删除", c)
}
