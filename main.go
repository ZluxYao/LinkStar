package main

import (
	"embed"
	"linkstar/core"
	"linkstar/modules/home"
	"linkstar/modules/stun"
	"linkstar/routers"
	"os"

	"github.com/sirupsen/logrus"
)

//go:embed web/admin/dist web/home/dist
var webFS embed.FS

func main() {
	// 设置时区
	os.Setenv("TZ", "Asia/Shanghai")
	core.InitLogger()
	logrus.Info("LinkStar Run")

	stun.InitSTUN()
	if err := home.InitHome(); err != nil {
		logrus.Error("Home 模块初始化失败：", err)
	}

	routers.Run(webFS)

}
