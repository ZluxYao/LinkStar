package routers

import (
	"linkstar/api"
	"linkstar/api/proxy_api"
	"linkstar/middleware"

	"github.com/gin-gonic/gin"
)

func ProxyRouters(g *gin.RouterGroup) {
	var app = api.App.ProxyApi

	// 入口设置 + 监听状态 + 站点表，一次给全
	g.GET(
		"proxy/config",
		app.ProxyConfigView,
	)

	// 默认入口：HTTP 端口、HTTPS 端口、默认证书、主域名
	g.POST(
		"proxy/entry",
		middleware.BindJsonMiddleware[proxy_api.EntrySaveRequest],
		app.ProxyEntrySaveView,
	)

	// 访问日志单独一个开关，不碰监听
	g.POST(
		"proxy/accesslog",
		middleware.BindJsonMiddleware[proxy_api.AccessLogRequest],
		app.ProxyAccessLogView,
	)

	// 站点增 / 改 / 删
	g.POST(
		"proxy/site",
		middleware.BindJsonMiddleware[proxy_api.SiteSaveRequest],
		app.ProxySiteAddView,
	)
	g.PUT(
		"proxy/site",
		middleware.BindJsonMiddleware[proxy_api.SiteSaveRequest],
		app.ProxySiteUpdateView,
	)
	g.DELETE(
		"proxy/site",
		middleware.BindJsonMiddleware[proxy_api.SiteDeleteRequest],
		app.ProxySiteDeleteView,
	)

	// 立即拨一次后端，不要求先保存
	g.POST(
		"proxy/site/test",
		middleware.BindJsonMiddleware[proxy_api.SiteSaveRequest],
		app.ProxySiteTestView,
	)
}
