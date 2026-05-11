package home_api

import (
	"linkstar/middleware"
	"linkstar/modules/home"
	"linkstar/utils/res"

	"github.com/gin-gonic/gin"
)

type LayoutUpdateRequest struct {
	LayoutMode string `json:"layoutMode"`
}

func (HomeApi) LayoutUpdateView(c *gin.Context) {
	cr := middleware.GetBindRequest[LayoutUpdateRequest](c)

	switch cr.LayoutMode {
	case "paged-horizontal", "paged-vertical", "paged-free", "scroll":
	default:
		res.FailWithMsg("layoutMode 不合法", c)
		return
	}

	err := home.Runtime.WithLock(func(cfg *home.Config) error {
		cfg.LayoutMode = cr.LayoutMode
		return nil
	})
	if err != nil {
		res.FailWithMsg("保存配置失败", c)
		return
	}
	res.OkWithMsg("更新成功", c)
}
