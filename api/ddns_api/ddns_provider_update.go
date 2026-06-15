package ddns_api

import (
	"linkstar/middleware"
	"linkstar/modules/ddns"
	"linkstar/modules/ddns/model"
	"linkstar/utils/res"
	"time"

	"github.com/gin-gonic/gin"
)

type DdnsProviderUpdateRequest struct {
	ID       uint   `json:"id"`
	Name     string `json:"name"`
	APIToken string `json:"apiToken"` // 留空表示不修改凭证
}

func (DdnsApi) DdnsProviderUpdateView(c *gin.Context) {
	cr := middleware.GetBindRequest[DdnsProviderUpdateRequest](c)

	if cr.Name == "" {
		res.FailWithMsg("服务商名称不能为空", c)
		return
	}

	found := false
	credChanged := false
	err := ddns.Runtime.Apply(func(cfg *model.DDNSConfig) error {
		for i := range cfg.Providers {
			if cfg.Providers[i].ID != cr.ID {
				continue
			}
			p := &cfg.Providers[i]
			p.Name = cr.Name
			if cr.APIToken != "" {
				if p.Credential == nil {
					p.Credential = map[string]interface{}{}
				}
				p.Credential["apiToken"] = cr.APIToken
				credChanged = true
			}
			p.UpdatedAt = time.Now()
			found = true
			break
		}
		return nil
	})
	if err != nil {
		res.FailWithMsg("保存配置失败", c)
		return
	}
	if !found {
		res.FailWithMsg("服务商不存在", c)
		return
	}

	// 凭证变更才需要重建调度器（重建 httpClient / token）
	if credChanged {
		ddns.Runtime.RebuildScheduler()
	}

	res.OkWithMsg("服务商已更新", c)
}
