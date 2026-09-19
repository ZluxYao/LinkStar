//go:build desktop

package main

import (
	"crypto/rand"
	_ "embed"
	"encoding/hex"
	"net/url"
	"os"
	"runtime"

	"linkstar/core"
	"linkstar/modules/auth"

	"github.com/sirupsen/logrus"
	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
	"github.com/wailsapp/wails/v3/pkg/icons"
)

const (
	mainWinName = "linkstar-main"
	// 默认开这么大：STUN 页面「设备列表 + 服务卡片」两栏能并排各自舒展开，
	// 服务卡片刚好两列，不用一进来就先拉窗口
	windowWidth  = 1440
	windowHeight = 900
	minWinWidth  = 960
	minWinHeight = 640
)

// fitWindow 把默认尺寸收进主显示器的可用区域。
// 1440x900 在 1366x768 这类小屏笔记本上比屏幕还大，wails 居中之后标题栏会跑到屏幕上边外面，
// 鼠标够不着就拖不回来了。留 48px 余量往下压，压到最小尺寸为止。
func fitWindow(w, h int) (int, int) {
	availW, availH := primaryWorkArea()
	if availW > 0 && w > availW-48 {
		w = availW - 48
	}
	if availH > 0 && h > availH-48 {
		h = availH - 48
	}
	return max(w, minWinWidth), max(h, minWinHeight)
}

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

// browserArgs 排查用：设了 LINKSTAR_WEBVIEW_DEBUG_PORT 才给 WebView2 开远程调试端口，平时为空。
func browserArgs() []string {
	if port := os.Getenv("LINKSTAR_WEBVIEW_DEBUG_PORT"); port != "" {
		return []string{"--remote-debugging-port=" + port}
	}
	return nil
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
			AdditionalBrowserArgs:         browserArgs(),
		},
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: false,
		},
		Linux: application.LinuxOptions{
			DisableQuitOnLastWindowClosed: true,
		},
	})

	winW, winH := fitWindow(windowWidth, windowHeight)
	window = app.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:         appName,
		Name:          mainWinName,
		Width:         winW,
		Height:        winH,
		MinWidth:      minWinWidth,
		MinHeight:     minWinHeight,
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
