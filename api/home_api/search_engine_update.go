package home_api

import (
	"linkstar/middleware"
	"linkstar/modules/home"
	"linkstar/utils/res"

	"github.com/gin-gonic/gin"
)

type SearchEngineUpdateRequest struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	ShortName string `json:"shortName"`
	URL       string `json:"url"`
	Color     string `json:"color"`
	Icon      string `json:"icon"`
}

func (HomeApi) SearchEngineUpdateView(c *gin.Context) {
	cr := middleware.GetBindRequest[SearchEngineUpdateRequest](c)
	if cr.ID == "" {
		res.FailWithMsg("id 不能为空", c)
		return
	}

	err := home.Runtime.WithLock(func(cfg *home.Config) error {
		for i := range cfg.SearchEngines {
			if cfg.SearchEngines[i].ID == cr.ID {
				cfg.SearchEngines[i].Name = cr.Name
				cfg.SearchEngines[i].ShortName = cr.ShortName
				cfg.SearchEngines[i].URL = cr.URL
				cfg.SearchEngines[i].Color = cr.Color
				cfg.SearchEngines[i].Icon = cr.Icon
				return nil
			}
		}
		return home.ErrNotFound
	})
	if err == home.ErrNotFound {
		res.FailWithMsg("搜索引擎不存在", c)
		return
	}
	if err != nil {
		res.FailWithMsg("保存配置失败", c)
		return
	}
	res.OkWithMsg("更新成功", c)
}
