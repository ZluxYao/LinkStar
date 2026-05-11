package home_api

import (
	"linkstar/middleware"
	"linkstar/modules/home"
	"linkstar/utils/res"

	"github.com/gin-gonic/gin"
)

type WallpaperUpdateRequest struct {
	Mode       string `json:"mode"`
	Resolution string `json:"resolution"`
	Blur       int    `json:"blur"`
}

func (HomeApi) WallpaperUpdateView(c *gin.Context) {
	cr := middleware.GetBindRequest[WallpaperUpdateRequest](c)

	if cr.Mode != "default" && cr.Mode != "bing" {
		res.FailWithMsg("mode 只能是 default / bing", c)
		return
	}
	if cr.Resolution != "1080" && cr.Resolution != "uhd" {
		res.FailWithMsg("resolution 只能是 1080 / uhd", c)
		return
	}
	if cr.Blur < 0 || cr.Blur > 12 {
		res.FailWithMsg("blur 取值 0-12", c)
		return
	}

	err := home.Runtime.WithLock(func(cfg *home.Config) error {
		cfg.Wallpaper = home.Wallpaper{Mode: cr.Mode, Resolution: cr.Resolution, Blur: cr.Blur}
		return nil
	})
	if err != nil {
		res.FailWithMsg("保存配置失败", c)
		return
	}
	res.OkWithMsg("更新成功", c)
}
