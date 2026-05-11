package home_api

import (
	"fmt"
	"linkstar/middleware"
	"linkstar/modules/home"
	"linkstar/utils/res"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

type CategoryAddRequest struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

func (HomeApi) CategoryAddView(c *gin.Context) {
	cr := middleware.GetBindRequest[CategoryAddRequest](c)
	name := strings.TrimSpace(cr.Name)
	if name == "" {
		res.FailWithMsg("name 不能为空", c)
		return
	}

	var created home.Category
	err := home.Runtime.WithLock(func(cfg *home.Config) error {
		id := cr.ID
		if id == "" {
			id = fmt.Sprintf("cat-%d", time.Now().UnixNano())
		}
		for _, c := range cfg.Categories {
			if c.ID == id {
				return home.ErrConflict
			}
		}
		maxOrder := 0
		for _, c := range cfg.Categories {
			if c.Order > maxOrder {
				maxOrder = c.Order
			}
		}
		now := time.Now()
		created = home.Category{
			ID:        id,
			Name:      name,
			Order:     maxOrder + 1,
			CreatedAt: now,
			UpdatedAt: now,
		}
		cfg.Categories = append(cfg.Categories, created)
		return nil
	})
	if err == home.ErrConflict {
		res.FailWithMsg("ID 已存在", c)
		return
	}
	if err != nil {
		res.FailWithMsg("保存配置失败", c)
		return
	}
	res.OkWithData(created, c)
}
