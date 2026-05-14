package home_api

import (
	"linkstar/middleware"
	"linkstar/modules/home"
	"linkstar/utils/res"

	"github.com/gin-gonic/gin"
)

// mode=paged: ids 为全局顺序,写 PagedOrder
// mode=scroll: ids 为某个分类内的顺序,写 ScrollOrder
type AppReorderRequest struct {
	Mode       string   `json:"mode"`
	CategoryID string   `json:"categoryId,omitempty"`
	IDs        []string `json:"ids"`
}

func (HomeApi) AppReorderView(c *gin.Context) {
	cr := middleware.GetBindRequest[AppReorderRequest](c)
	if cr.Mode != "paged" && cr.Mode != "scroll" {
		res.FailWithMsg("mode 只能是 paged / scroll", c)
		return
	}

	err := home.Runtime.WithLock(func(cfg *home.Config) error {
		index := make(map[string]int, len(cr.IDs))
		for i, id := range cr.IDs {
			index[id] = i + 1
		}
		for i := range cfg.Apps {
			order, ok := index[cfg.Apps[i].ID]
			if !ok {
				continue
			}
			if cr.Mode == "paged" {
				cfg.Apps[i].PagedOrder = order
			} else {
				if cfg.Apps[i].CategoryID != cr.CategoryID {
					continue
				}
				cfg.Apps[i].ScrollOrder = order
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
