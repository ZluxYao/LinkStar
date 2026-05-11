package home_api

import (
	"linkstar/middleware"
	"linkstar/modules/home"
	"linkstar/utils/res"

	"github.com/gin-gonic/gin"
)

type SearchEngineReorderRequest struct {
	IDs []string `json:"ids"`
}

func (HomeApi) SearchEngineReorderView(c *gin.Context) {
	cr := middleware.GetBindRequest[SearchEngineReorderRequest](c)

	err := home.Runtime.WithLock(func(cfg *home.Config) error {
		index := make(map[string]int, len(cr.IDs))
		for i, id := range cr.IDs {
			index[id] = i + 1
		}
		for i := range cfg.SearchEngines {
			if order, ok := index[cfg.SearchEngines[i].ID]; ok {
				cfg.SearchEngines[i].Order = order
			}
		}
		// 按 Order 升序重排切片
		engines := append([]home.SearchEngine(nil), cfg.SearchEngines...)
		sortEnginesByOrder(engines)
		cfg.SearchEngines = engines
		return nil
	})
	if err != nil {
		res.FailWithMsg("保存配置失败", c)
		return
	}
	res.OkWithMsg("更新成功", c)
}

func sortEnginesByOrder(arr []home.SearchEngine) {
	for i := 1; i < len(arr); i++ {
		for j := i; j > 0 && arr[j-1].Order > arr[j].Order; j-- {
			arr[j-1], arr[j] = arr[j], arr[j-1]
		}
	}
}
