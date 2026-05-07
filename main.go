package main

import (
	"embed"
	"linkstar/core"
	"linkstar/modules/stun"
	"linkstar/routers"
	"os"

	"github.com/sirupsen/logrus"
)

//go:embed web/dist web/home/dist
var webFS embed.FS

func main() {
	// 设置时区
	os.Setenv("TZ", "Asia/Shanghai")
	core.InitLogger()
	logrus.Info("LinkStar Run")

	stun.InitSTUN()

	routers.Run(webFS)

}
