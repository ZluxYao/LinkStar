package routers

import (
	"linkstar/api"
	"linkstar/api/ddns_api"
	"linkstar/middleware"

	"github.com/gin-gonic/gin"
)

func DdnsRouters(g *gin.RouterGroup) {
	var app = api.App.DdnsApi

	// 输出当前 DDNS 配置（凭证脱敏）
	g.GET(
		"ddns/config",
		app.GetDdnsConfigView,
	)

	// 更新总开关与全局同步间隔
	g.PUT(
		"ddns/settings",
		middleware.BindJsonMiddleware[ddns_api.DdnsSettingsUpdateRequest],
		app.DdnsSettingsUpdateView,
	)

	// 服务商：增 / 改 / 删
	g.POST(
		"ddns/provider/add",
		middleware.BindJsonMiddleware[ddns_api.DdnsProviderAddRequest],
		app.DdnsProviderAddView,
	)
	g.PUT(
		"ddns/provider/update",
		middleware.BindJsonMiddleware[ddns_api.DdnsProviderUpdateRequest],
		app.DdnsProviderUpdateView,
	)
	g.DELETE(
		"ddns/provider/delete",
		middleware.BindJsonMiddleware[ddns_api.DdnsProviderDeleteRequest],
		app.DdnsProviderDeleteView,
	)

	// 解析记录：增 / 改 / 删
	g.POST(
		"ddns/record/add",
		middleware.BindJsonMiddleware[ddns_api.DdnsRecordAddRequest],
		app.DdnsRecordAddView,
	)
	g.PUT(
		"ddns/record/update",
		middleware.BindJsonMiddleware[ddns_api.DdnsRecordUpdateRequest],
		app.DdnsRecordUpdateView,
	)
	g.DELETE(
		"ddns/record/delete",
		middleware.BindJsonMiddleware[ddns_api.DdnsRecordDeleteRequest],
		app.DdnsRecordDeleteView,
	)

	// 立即同步（id=0 全量，否则单条）
	g.POST(
		"ddns/record/sync",
		middleware.BindJsonMiddleware[ddns_api.DdnsRecordSyncRequest],
		app.DdnsRecordSyncView,
	)
}
