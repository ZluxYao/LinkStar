package ddns_api

import (
	"linkstar/middleware"
	"linkstar/modules/ddns"
	"linkstar/utils/res"

	"github.com/gin-gonic/gin"
)

type DdnsRecordSyncRequest struct {
	ID uint `json:"id"` // 0 表示同步全部
}

// DdnsRecordSyncView 立即同步：指定记录同步执行并返回结果，0 表示触发全量异步同步
func (DdnsApi) DdnsRecordSyncView(c *gin.Context) {
	cr := middleware.GetBindRequest[DdnsRecordSyncRequest](c)

	if cr.ID == 0 {
		ddns.Runtime.TriggerSync()
		res.OkWithMsg("已触发全量同步", c)
		return
	}

	if err := ddns.Runtime.SyncRecordNow(cr.ID); err != nil {
		res.FailWithMsg(err.Error(), c)
		return
	}

	res.OkWithMsg("同步成功", c)
}
