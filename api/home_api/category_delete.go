package home_api

import (
	"linkstar/middleware"
	"linkstar/modules/home"
	"linkstar/utils/res"

	"github.com/gin-gonic/gin"
)

type CategoryDeleteRequest struct {
	ID string `json:"id"`
}

// 删除分类:分类下的 app 被置为「未分类」(CategoryID="")
func (HomeApi) CategoryDeleteView(c *gin.Context) {
	cr := middleware.GetBindRequest[CategoryDeleteRequest](c)
	if cr.ID == "" {
		res.FailWithMsg("id 不能为空", c)
		return
	}

	err := home.Runtime.WithLock(func(cfg *home.Config) error {
		out := cfg.Categories[:0]
		found := false
		for _, c := range cfg.Categories {
			if c.ID == cr.ID {
				found = true
				continue
			}
			out = append(out, c)
		}
		if !found {
			return home.ErrNotFound
		}
		cfg.Categories = out
		// 该分类下的 app 改成未分类
		for i := range cfg.Apps {
			if cfg.Apps[i].CategoryID == cr.ID {
				cfg.Apps[i].CategoryID = ""
			}
		}
		return nil
	})
	if err == home.ErrNotFound {
		res.FailWithMsg("分类不存在", c)
		return
	}
	if err != nil {
		res.FailWithMsg("保存配置失败", c)
		return
	}
	res.OkWithMsg("删除成功", c)
}
