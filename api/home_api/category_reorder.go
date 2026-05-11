package home_api

import (
	"linkstar/middleware"
	"linkstar/modules/home"
	"linkstar/utils/res"

	"github.com/gin-gonic/gin"
)

type CategoryReorderRequest struct {
	IDs []string `json:"ids"`
}

func (HomeApi) CategoryReorderView(c *gin.Context) {
	cr := middleware.GetBindRequest[CategoryReorderRequest](c)

	err := home.Runtime.WithLock(func(cfg *home.Config) error {
		index := make(map[string]int, len(cr.IDs))
		for i, id := range cr.IDs {
			index[id] = i + 1
		}
		for i := range cfg.Categories {
			if order, ok := index[cfg.Categories[i].ID]; ok {
				cfg.Categories[i].Order = order
			}
		}
		sortCategoriesByOrder(cfg.Categories)
		return nil
	})
	if err != nil {
		res.FailWithMsg("保存配置失败", c)
		return
	}
	res.OkWithMsg("更新成功", c)
}

func sortCategoriesByOrder(arr []home.Category) {
	for i := 1; i < len(arr); i++ {
		for j := i; j > 0 && arr[j-1].Order > arr[j].Order; j-- {
			arr[j-1], arr[j] = arr[j], arr[j-1]
		}
	}
}
