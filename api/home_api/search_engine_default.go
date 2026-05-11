package home_api

import (
	"linkstar/middleware"
	"linkstar/modules/home"
	"linkstar/utils/res"

	"github.com/gin-gonic/gin"
)

type SearchEngineDefaultRequest struct {
	ID string `json:"id"`
}

func (HomeApi) SearchEngineDefaultView(c *gin.Context) {
	cr := middleware.GetBindRequest[SearchEngineDefaultRequest](c)
	if cr.ID == "" {
		res.FailWithMsg("id 不能为空", c)
		return
	}

	err := home.Runtime.WithLock(func(cfg *home.Config) error {
		for _, e := range cfg.SearchEngines {
			if e.ID == cr.ID {
				cfg.DefaultSearchEngineID = cr.ID
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
