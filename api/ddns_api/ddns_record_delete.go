package ddns_api

import (
	"linkstar/middleware"
	"linkstar/modules/ddns"
	"linkstar/modules/ddns/model"
	"linkstar/utils/res"

	"github.com/gin-gonic/gin"
)

type DdnsRecordDeleteRequest struct {
	ID uint `json:"id"`
}

func (DdnsApi) DdnsRecordDeleteView(c *gin.Context) {
	cr := middleware.GetBindRequest[DdnsRecordDeleteRequest](c)

	found := false
	_ = ddns.Runtime.Apply(func(cfg *model.DDNSConfig) error {
		idx := -1
		for i := range cfg.Records {
			if cfg.Records[i].ID == cr.ID {
				idx = i
				break
			}
		}
		if idx == -1 {
			return nil
		}
		found = true
		cfg.Records = append(cfg.Records[:idx], cfg.Records[idx+1:]...)
		return nil
	})

	if !found {
		res.FailWithMsg("记录不存在", c)
		return
	}

	res.OkWithMsg("记录已删除", c)
}
