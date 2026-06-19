package main

import (
	"embed"
	"linkstar/core"
	"linkstar/modules/ddns"
	"linkstar/modules/home"
	"linkstar/modules/stun"
	"linkstar/routers"
	"os"
	"sync"

	"github.com/sirupsen/logrus"
)

//go:embed web/admin/dist web/home/dist
var webFS embed.FS

func main() {
	// 设置时区
	os.Setenv("TZ", "Asia/Shanghai")
	core.InitLogger()
	logrus.Info("LinkStar Run")

	// 统一监听退出信号，保存各模块配置
	core.ListenShutdown()

	var wg sync.WaitGroup
	wg.Add(3)
	go func() {
		defer wg.Done()
		stun.InitSTUN()
	}()
	go func() {
		defer wg.Done()
		if err := home.InitHome(); err != nil {
			logrus.Error("Home 模块初始化失败：", err)
		}
	}()
	go func() {
		defer wg.Done()
		if err := ddns.DDNSInit(); err != nil {
			logrus.Error("DDNS 模块初始化失败：", err)
		}
	}()
	wg.Wait()

	routers.Run(webFS)

}
