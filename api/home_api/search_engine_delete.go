package home_api

import (
	"linkstar/middleware"
	"linkstar/modules/home"
	"linkstar/utils/res"

	"github.com/gin-gonic/gin"
)

type SearchEngineDeleteRequest struct {
	ID string `json:"id"`
}

func (HomeApi) SearchEngineDeleteView(c *gin.Context) {
	cr := middleware.GetBindRequest[SearchEngineDeleteRequest](c)
	if cr.ID == "" {
		res.FailWithMsg("id 不能为空", c)
		return
	}

	err := home.Runtime.WithLock(func(cfg *home.Config) error {
		out := cfg.SearchEngines[:0]
		found := false
		for _, e := range cfg.SearchEngines {
			if e.ID == cr.ID {
				found = true
				continue
			}
			out = append(out, e)
		}
		if !found {
			return home.ErrNotFound
		}
		cfg.SearchEngines = out
		if cfg.DefaultSearchEngineID == cr.ID {
			if len(cfg.SearchEngines) > 0 {
				cfg.DefaultSearchEngineID = cfg.SearchEngines[0].ID
			} else {
				cfg.DefaultSearchEngineID = ""
			}
		}
		return nil
	})
	if err == home.ErrNotFound {
		res.FailWithMsg("搜索引擎不存在", c)
		return
	}
	if err != nil {
		res.FailWithMsg("保存配置失败", c)
		return
	}
	res.OkWithMsg("删除成功", c)
}
