package home_api

import (
	"linkstar/middleware"
	"linkstar/modules/home"
	"linkstar/utils/res"
	"time"

	"github.com/gin-gonic/gin"
)

// 拖拽分配分类时调用,categoryId 为空 = 未分类
type AppCategoryRequest struct {
	ID         string `json:"id"`
	CategoryID string `json:"categoryId"`
}

func (HomeApi) AppCategoryView(c *gin.Context) {
	cr := middleware.GetBindRequest[AppCategoryRequest](c)
	if cr.ID == "" {
		res.FailWithMsg("id 不能为空", c)
		return
	}

	err := home.Runtime.WithLock(func(cfg *home.Config) error {
		if cr.CategoryID != "" {
			ok := false
			for _, cat := range cfg.Categories {
				if cat.ID == cr.CategoryID {
					ok = true
					break
				}
			}
			if !ok {
				return home.ErrNotFound
			}
		}
		for i := range cfg.Apps {
			if cfg.Apps[i].ID == cr.ID {
				cfg.Apps[i].CategoryID = cr.CategoryID
				cfg.Apps[i].UpdatedAt = time.Now()
				return nil
			}
		}
		return home.ErrNotFound
	})
	if err == home.ErrNotFound {
		res.FailWithMsg("app 或分类不存在", c)
		return
	}
	if err != nil {
		res.FailWithMsg("保存配置失败", c)
		return
	}
	res.OkWithMsg("更新成功", c)
}
