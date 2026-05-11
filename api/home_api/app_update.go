package home_api

import (
	"linkstar/middleware"
	"linkstar/modules/home"
	"linkstar/utils/res"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// stun 类型仅允许改 Name/Icon/Color/CategoryID；static 类型还可以改 Addresses
type AppUpdateRequest struct {
	ID         string             `json:"id"`
	Name       string             `json:"name"`
	Icon       string             `json:"icon"`
	Color      string             `json:"color"`
	CategoryID string             `json:"categoryId"`
	Addresses  *home.AppAddresses `json:"addresses,omitempty"`
}

func (HomeApi) AppUpdateView(c *gin.Context) {
	cr := middleware.GetBindRequest[AppUpdateRequest](c)
	if cr.ID == "" || strings.TrimSpace(cr.Name) == "" {
		res.FailWithMsg("id / name 不能为空", c)
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
			if cfg.Apps[i].ID != cr.ID {
				continue
			}
			app := &cfg.Apps[i]
			app.Name = cr.Name
			app.Icon = cr.Icon
			app.Color = cr.Color
			app.CategoryID = cr.CategoryID
			if app.Type == "static" && cr.Addresses != nil {
				copy := *cr.Addresses
				app.Addresses = &copy
			}
			app.UpdatedAt = time.Now()
			return nil
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
