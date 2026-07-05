//go:build !desktop

package main

import "github.com/sirupsen/logrus"

func main() {
	initRuntime()

	started, err := startBackend(webFS)
	if err != nil {
		logrus.Fatal(err)
	}
	if !started {
		logrus.Infof("LinkStar 已在 %s 运行", adminURL)
		return
	}

	logrus.Infof("管理后台运行在 %s", adminURL)
	logrus.Infof("导航主页运行在 %s", homeURL)
	select {}
}
