package routers

import (
	"io/fs"
	"net/http"
	_ "net/http/pprof" // 加下划线，只要副作用（自动注册路由）
	"strings"
	"time"

	"linkstar/middleware"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
)

func Run(webFS fs.FS) {
	// 单独起 pprof，只在排查问题时用
	// go func() {
	// 	logrus.Info("pprof 运行在：0.0.0.0:3334")
	// 	http.ListenAndServe("0.0.0.0:3334", nil)
	// }()

	gin.SetMode("release")
	r := gin.Default()
	r.RedirectTrailingSlash = false

	// API 路由
	g := r.Group("api")

	// 公开接口：鉴权自身 + 导航主页读接口 + 系统信息
	AuthRouters(g)
	HomeRoutersPublic(g)
	SystemRouters(g)

	// 受保护接口：需登录（或桌面 secret）
	protected := g.Group("", middleware.AuthMiddleware)
	StunRouters(protected)
	DdnsRouters(protected)
	CertRouters(protected)
	ProxyRouters(protected)
	WebhookRouters(protected)
	HomeRoutersProtected(protected)
	SystemRoutersProtected(protected)

	// 用户上传的图标静态目录
	r.Static("/data/icon", "data/icon")

	// 服务索引显式入口：/go/{服务名}/... → 307 到该服务当前的洞
	PortalRouters(r)

	// 剥掉 路径 前缀
	adminFS, _ := fs.Sub(webFS, "web/admin/dist")
	homeFS, _ := fs.Sub(webFS, "web/home/dist")

	// 所有非 API 请求：先找静态文件，找不到就返回 index.html（Vue Router 兜底）
	r.NoRoute(func(c *gin.Context) {
		path := c.Request.URL.Path

		if strings.HasPrefix(path, "/api") {
			c.JSON(404, gin.H{"error": "not found"})
			return
		}

		if path == "/linkstar" {
			c.Redirect(http.StatusMovedPermanently, "/linkstar/")
			return
		}

		if strings.HasPrefix(path, "/linkstar/") {
			filePath := strings.TrimPrefix(path, "/linkstar/")
			if filePath != "" {
				if _, err := fs.Stat(adminFS, filePath); err == nil {
					c.FileFromFS(filePath, http.FS(adminFS))
					return
				}
			}

			data, _ := fs.ReadFile(adminFS, "index.html")
			c.Data(200, "text/html; charset=utf-8", data)
			return
		}

		filePath := strings.TrimPrefix(path, "/")
		if filePath != "" {
			if _, err := fs.Stat(homeFS, filePath); err == nil {
				c.FileFromFS(filePath, http.FS(homeFS))
				return
			}
		}

		// 裸路径服务索引：linkstar.zlux.top/fw → 307 到 fw 当前的洞。
		// 必须排在静态文件命中之后——否则一个叫 favicon.ico 的服务名
		// 会把真实静态资源顶掉；没有同名服务则继续走前端 SPA 兜底。
		if TryPortal(c) {
			return
		}

		data, _ := fs.ReadFile(homeFS, "index.html")
		c.Data(200, "text/html; charset=utf-8", data)
	})

	logrus.Info("后端运行在：0.0.0.0:3333")

	srv := &http.Server{
		Addr:        "0.0.0.0:3333",
		Handler:     r,
		IdleTimeout: 60 * time.Second,
	}
	if err := srv.ListenAndServe(); err != nil {
		logrus.Fatal("启动失败：", err)
	}
}
