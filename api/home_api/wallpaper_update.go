package home_api

import (
	"linkstar/middleware"
	"linkstar/modules/home"
	"linkstar/utils/res"

	"github.com/gin-gonic/gin"
)

// maxWallpaperCSS 默认背景自定义样式的长度上限。写成 data: URI 的小图也放得下，
// 又不至于让人把一整张图塞进配置文件。
const maxWallpaperCSS = 20000

type WallpaperUpdateRequest struct {
	Mode       string `json:"mode"`
	Resolution string `json:"resolution"`
	Blur       int    `json:"blur"`
	Custom     string `json:"custom"`
	CSS        string `json:"css"`
}

func (HomeApi) WallpaperUpdateView(c *gin.Context) {
	cr := middleware.GetBindRequest[WallpaperUpdateRequest](c)

	if cr.Mode != "default" && cr.Mode != "bing" && cr.Mode != "custom" {
		res.FailWithMsg("mode 只能是 default / bing / custom", c)
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
	// custom 会被前端拼成 <img src>，只认上传接口吐出来的那个形状
	if cr.Custom != "" && !validWallpaperPath(cr.Custom) {
		res.FailWithMsg("壁纸路径不合法，请重新上传", c)
		return
	}
	if cr.Mode == "custom" && cr.Custom == "" {
		res.FailWithMsg("请先上传一张壁纸", c)
		return
	}
	if len(cr.CSS) > maxWallpaperCSS {
		res.FailWithMsg("背景样式太长了，最多 2 万个字符", c)
		return
	}

	err := home.Runtime.WithLock(func(cfg *home.Config) error {
		cfg.Wallpaper = home.Wallpaper{
			Mode:       cr.Mode,
			Resolution: cr.Resolution,
			Blur:       cr.Blur,
			Custom:     cr.Custom,
			CSS:        cr.CSS,
		}
		return nil
	})
	if err != nil {
		res.FailWithMsg("保存配置失败", c)
		return
	}

	// 配置已经落盘，现在磁盘上只该留下还被引用的那一张
	pruneWallpapers(cr.Custom)

	res.OkWithMsg("更新成功", c)
}
