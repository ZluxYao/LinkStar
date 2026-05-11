package home_api

import (
	"linkstar/middleware"
	"linkstar/modules/home"
	"linkstar/utils/res"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

type CategoryUpdateRequest struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

func (HomeApi) CategoryUpdateView(c *gin.Context) {
	cr := middleware.GetBindRequest[CategoryUpdateRequest](c)
	if cr.ID == "" || strings.TrimSpace(cr.Name) == "" {
		res.FailWithMsg("id / name 不能为空", c)
		return
	}

	err := home.Runtime.WithLock(func(cfg *home.Config) error {
		for i := range cfg.Categories {
			if cfg.Categories[i].ID == cr.ID {
				cfg.Categories[i].Name = cr.Name
				cfg.Categories[i].UpdatedAt = time.Now()
				return nil
			}
		}
		return home.ErrNotFound
	})
	if err == home.ErrNotFound {
		res.FailWithMsg("分类不存在", c)
		return
	}
	if err != nil {
		res.FailWithMsg("保存配置失败", c)
		return
	}
	res.OkWithMsg("更新成功", c)
}
