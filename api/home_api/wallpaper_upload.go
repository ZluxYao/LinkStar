package home_api

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"linkstar/utils/res"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
)

const wallpaperDir = "data/wallpaper"

// 壁纸就是拿来当背景看的，只收位图。
// 不收 svg：它是从本站域名下发出去的，直接访问就是一个同源页面，里面能带脚本。
var allowedWallpaperExt = map[string]struct{}{
	".jpg": {}, ".jpeg": {}, ".png": {}, ".webp": {}, ".avif": {}, ".bmp": {},
}

// 一张 4K 壁纸十几 MB 很正常，图标那 5MB 的上限不够用
const maxWallpaperSize = 20 << 20

// wallpaperPathRe 配置里存的壁纸路径长什么样。这个值会进配置文件、
// 再被前端拼成 <img src>，所以存之前要卡死形状，不能让任意字符串进来。
var wallpaperPathRe = regexp.MustCompile(`^data/wallpaper/[0-9]+\.[a-z]+$`)

// validWallpaperPath 路径形状对，且扩展名在白名单里
func validWallpaperPath(p string) bool {
	if !wallpaperPathRe.MatchString(p) {
		return false
	}
	_, ok := allowedWallpaperExt[strings.ToLower(filepath.Ext(p))]
	return ok
}

// WallpaperUploadView 上传自定义壁纸，返回相对路径如 data/wallpaper/1737000000.jpg
func (HomeApi) WallpaperUploadView(c *gin.Context) {
	file, err := c.FormFile("file")
	if err != nil {
		res.FailWithMsg("缺少 file 字段", c)
		return
	}
	if file.Size > maxWallpaperSize {
		res.FailWithMsg("图片超过 20MB", c)
		return
	}
	ext := strings.ToLower(filepath.Ext(file.Filename))
	if _, ok := allowedWallpaperExt[ext]; !ok {
		res.FailWithMsg("仅支持 jpg/png/webp/avif/bmp", c)
		return
	}

	if err := os.MkdirAll(wallpaperDir, 0755); err != nil {
		res.FailWithMsg("创建目录失败:"+err.Error(), c)
		return
	}

	name := fmt.Sprintf("%d%s", time.Now().UnixNano(), ext)
	relPath := filepath.ToSlash(filepath.Join(wallpaperDir, name))
	if err := c.SaveUploadedFile(file, relPath); err != nil {
		res.FailWithMsg("保存失败:"+err.Error(), c)
		return
	}

	res.OkWithData(gin.H{"path": relPath}, c)
}

// pruneWallpapers 删掉 data/wallpaper 里除 keep 之外的文件。
// 换一张壁纸就留一份，不然这个目录会一直涨——一张 4K 图十几 MB，攒几十张很难看。
// 尽力而为：删不掉只记一条日志，不影响用户已经保存成功的配置。
func pruneWallpapers(keep string) {
	entries, err := os.ReadDir(wallpaperDir)
	if err != nil {
		return
	}
	keepName := filepath.Base(keep)
	for _, e := range entries {
		if e.IsDir() || e.Name() == keepName {
			continue
		}
		if err := os.Remove(filepath.Join(wallpaperDir, e.Name())); err != nil {
			logrus.Warnf("删除旧壁纸 %s 失败：%v", e.Name(), err)
		}
	}
}
