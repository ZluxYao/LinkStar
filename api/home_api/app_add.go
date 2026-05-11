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

// 仅支持 type=static (常用网站)。stun 类型由 stun 模块联动创建
type AppAddRequest struct {
	Name       string             `json:"name"`
	Icon       string             `json:"icon"`
	Color      string             `json:"color"`
	CategoryID string             `json:"categoryId"`
	Addresses  home.AppAddresses  `json:"addresses"`
}

func (HomeApi) AppAddView(c *gin.Context) {
	cr := middleware.GetBindRequest[AppAddRequest](c)
	if strings.TrimSpace(cr.Name) == "" {
		res.FailWithMsg("name 不能为空", c)
		return
	}
	if strings.TrimSpace(cr.Addresses.WANv4) == "" &&
		strings.TrimSpace(cr.Addresses.WANv6) == "" &&
		strings.TrimSpace(cr.Addresses.LAN) == "" {
		res.FailWithMsg("至少填写一个地址", c)
		return
	}

	var created home.App
	err := home.Runtime.WithLock(func(cfg *home.Config) error {
		// 校验分类存在（空 = 未分类，允许）
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
		now := time.Now()
		addrs := cr.Addresses
		maxPaged, maxScroll := 0, 0
		for _, a := range cfg.Apps {
			if a.PagedOrder > maxPaged {
				maxPaged = a.PagedOrder
			}
			if a.CategoryID == cr.CategoryID && a.ScrollOrder > maxScroll {
				maxScroll = a.ScrollOrder
			}
		}
		created = home.App{
			ID:          fmt.Sprintf("app-%d", now.UnixNano()),
			Name:        cr.Name,
			Icon:        cr.Icon,
			Color:       cr.Color,
			CategoryID:  cr.CategoryID,
			PagedOrder:  maxPaged + 1,
			ScrollOrder: maxScroll + 1,
			Type:        "static",
			Addresses:   &addrs,
			CreatedAt:   now,
			UpdatedAt:   now,
		}
		cfg.Apps = append(cfg.Apps, created)
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
	res.OkWithData(created, c)
}
