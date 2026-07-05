//go:build desktop

package main

import (
	_ "embed"
	"crypto/rand"
	"encoding/hex"
	"net/url"
	"runtime"

	"linkstar/core"
	"linkstar/modules/auth"

	"github.com/sirupsen/logrus"
	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
	"github.com/wailsapp/wails/v3/pkg/icons"
)

const (
	mainWinName  = "linkstar-main"
	windowWidth  = 1180
	windowHeight = 760
)

//go:embed icon_64.png
var trayIcon []byte

// desktopSecret 本次进程的桌面免登录 secret，注入后端并随 admin URL 传给 webview。
var desktopSecret = genDesktopSecret()

func genDesktopSecret() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return ""
	}
	return hex.EncodeToString(b)
}

// desktopAdminURL 在 admin URL 上附带 secret，前端首屏读取后存入 sessionStorage 并从地址栏抹掉。
func desktopAdminURL() string {
	if desktopSecret == "" {
		return adminURL
	}
	return adminURL + "?desktop_secret=" + url.QueryEscape(desktopSecret)
}

func main() {
	initRuntime()

	// 注入桌面免登录 secret：仅 wails 构建设置，CLI 构建 secret 为空，中间件永不放行桌面通道
	auth.SetDesktopSecret(desktopSecret)

	app := newDesktopApp()

	started, err := startBackend(webFS)
	if err != nil {
		logrus.Fatal(err)
	}
	if !started {
		logrus.Infof("复用已运行的后端 %s", backendURL)
	}

	if err := app.Run(); err != nil {
		logrus.Fatal(err)
	}
}

func newDesktopApp() *application.App {
	var window application.Window

	app := application.New(application.Options{
		Name:                        appName,
		Description:                 "LinkStar 桌面控制台",
		Icon:                        trayIcon,
		Assets:                      application.AlphaAssets,
		DisableDefaultSignalHandler: true,
		SingleInstance: &application.SingleInstanceOptions{
			UniqueID: "com.linkstar.desktop",
			ExitCode: 0,
			OnSecondInstanceLaunch: func(data application.SecondInstanceData) {
				if window != nil {
					showWindow(window, adminURL)
				}
			},
		},
		OnShutdown: func() {
			core.RunShutdown()
			logrus.Info("LinkStar 配置已保存")
		},
		Windows: application.WindowsOptions{
			DisableQuitOnLastWindowClosed: true,
		},
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: false,
		},
		Linux: application.LinuxOptions{
			DisableQuitOnLastWindowClosed: true,
		},
	})

	window = app.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:         appName,
		Name:          mainWinName,
		Width:         windowWidth,
		Height:        windowHeight,
		MinWidth:      960,
		MinHeight:     640,
		URL:           desktopAdminURL(),
		HideOnEscape:  true,
		DisableResize: false,
		BackgroundColour: application.NewRGB(
			246,
			248,
			252,
		),
	})
	window.RegisterHook(events.Common.WindowClosing, func(e *application.WindowEvent) {
		window.Hide()
		e.Cancel()
	})

	tray := app.SystemTray.New()
	if runtime.GOOS == "darwin" {
		tray.SetTemplateIcon(icons.SystrayMacTemplate)
	} else {
		tray.SetIcon(trayIcon)
	}
	tray.SetTooltip(appName)
	tray.OnClick(func() {
		showWindow(window, adminURL)
	})
	tray.SetMenu(buildTrayMenu(app, window))

	return app
}

func buildTrayMenu(app *application.App, window application.Window) *application.Menu {
	menu := app.NewMenu()
	menu.Add("打开 LinkStar").OnClick(func(ctx *application.Context) {
		showWindow(window, adminURL)
	})
	menu.Add("打开导航主页").OnClick(func(ctx *application.Context) {
		showWindow(window, homeURL)
	})
	menu.AddSeparator()
	menu.Add("退出").OnClick(func(ctx *application.Context) {
		app.Quit()
	})
	return menu
}

func showWindow(window application.Window, url string) {
	window.SetURL(url)
	window.Restore()
	window.Show()
	window.Focus()
}
