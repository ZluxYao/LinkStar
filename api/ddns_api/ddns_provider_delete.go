package ddns_api

import (
	"fmt"
	"linkstar/middleware"
	"linkstar/modules/ddns"
	"linkstar/modules/ddns/model"
	"linkstar/utils/res"

	"github.com/gin-gonic/gin"
)

type DdnsProviderDeleteRequest struct {
	ID uint `json:"id"`
}

func (DdnsApi) DdnsProviderDeleteView(c *gin.Context) {
	cr := middleware.GetBindRequest[DdnsProviderDeleteRequest](c)

	found := false
	err := ddns.Runtime.Apply(func(cfg *model.DDNSConfig) error {
		// 拒绝删除仍被记录引用的服务商
		var inUse int
		for _, r := range cfg.Records {
			if r.ProviderID == cr.ID {
				inUse++
			}
		}
		if inUse > 0 {
			return fmt.Errorf("该服务商下还有 %d 条解析记录，请先删除记录", inUse)
		}

		idx := -1
		for i := range cfg.Providers {
			if cfg.Providers[i].ID == cr.ID {
				idx = i
				break
			}
		}
		if idx == -1 {
			return nil
		}
		found = true
		cfg.Providers = append(cfg.Providers[:idx], cfg.Providers[idx+1:]...)
		return nil
	})
	if err != nil {
		res.FailWithMsg(err.Error(), c)
		return
	}
	if !found {
		res.FailWithMsg("服务商不存在", c)
		return
	}

	ddns.Runtime.RebuildScheduler()

	res.OkWithMsg("服务商已删除", c)
}
