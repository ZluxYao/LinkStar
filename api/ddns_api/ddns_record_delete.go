package ddns_api

import (
	"linkstar/middleware"
	"linkstar/modules/ddns"
	"linkstar/utils/res"

	"github.com/gin-gonic/gin"
)

type DdnsRecordDeleteRequest struct {
	ID uint `json:"id"`
}

func (DdnsApi) DdnsRecordDeleteView(c *gin.Context) {
	cr := middleware.GetBindRequest[DdnsRecordDeleteRequest](c)

	if err := ddns.Runtime.DeleteRecord(cr.ID); err != nil {
		res.FailWithMsg(err.Error(), c)
		return
	}

	res.OkWithMsg("记录已删除", c)
}
