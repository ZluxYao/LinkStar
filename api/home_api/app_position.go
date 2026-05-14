package home_api

import (
	"linkstar/middleware"
	"linkstar/modules/home"
	"linkstar/utils/res"

	"github.com/gin-gonic/gin"
)

// 批量设置 paged 布局的绝对槽位 (1-indexed,允许间隔)。
// 与 reorder 不同: 不会重排其他 app 的 pagedOrder,只更新请求中提到的 id。
type AppPositionsRequest struct {
	Positions []AppPositionItem `json:"positions"`
}

type AppPositionItem struct {
	ID         string `json:"id"`
	PagedOrder int    `json:"pagedOrder"`
}

func (HomeApi) AppPositionView(c *gin.Context) {
	cr := middleware.GetBindRequest[AppPositionsRequest](c)
	if len(cr.Positions) == 0 {
		res.OkWithMsg("noop", c)
		return
	}
	index := make(map[string]int, len(cr.Positions))
	for _, p := range cr.Positions {
		if p.PagedOrder > 0 {
			index[p.ID] = p.PagedOrder
		}
	}
	if len(index) == 0 {
		res.OkWithMsg("noop", c)
		return
	}
	err := home.Runtime.WithLock(func(cfg *home.Config) error {
		for i := range cfg.Apps {
			if order, ok := index[cfg.Apps[i].ID]; ok {
				cfg.Apps[i].PagedOrder = order
			}
		}
		return nil
	})
	if err != nil {
		res.FailWithMsg("保存配置失败", c)
		return
	}
	res.OkWithMsg("更新成功", c)
}
