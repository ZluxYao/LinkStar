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

type SearchEngineAddRequest struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	ShortName string `json:"shortName"`
	URL       string `json:"url"`
	Color     string `json:"color"`
	Icon      string `json:"icon"`
}

func (HomeApi) SearchEngineAddView(c *gin.Context) {
	cr := middleware.GetBindRequest[SearchEngineAddRequest](c)
	if strings.TrimSpace(cr.Name) == "" || strings.TrimSpace(cr.URL) == "" {
		res.FailWithMsg("name / url 不能为空", c)
		return
	}

	err := home.Runtime.WithLock(func(cfg *home.Config) error {
		id := cr.ID
		if id == "" {
			id = fmt.Sprintf("se-%d", time.Now().UnixNano())
		}
		for _, e := range cfg.SearchEngines {
			if e.ID == id {
				return home.ErrConflict
			}
		}
		maxOrder := 0
		for _, e := range cfg.SearchEngines {
			if e.Order > maxOrder {
				maxOrder = e.Order
			}
		}
		cfg.SearchEngines = append(cfg.SearchEngines, home.SearchEngine{
			ID:        id,
			Name:      cr.Name,
			ShortName: cr.ShortName,
			URL:       cr.URL,
			Color:     cr.Color,
			Icon:      cr.Icon,
			Order:     maxOrder + 1,
		})
		return nil
	})
	if err != nil {
		if err == home.ErrConflict {
			res.FailWithMsg("ID 已存在", c)
			return
		}
		res.FailWithMsg("保存配置失败", c)
		return
	}
	res.OkWithMsg("添加成功", c)
}
