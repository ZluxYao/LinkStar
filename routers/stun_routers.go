package routers

import (
	"linkstar/api"
	"linkstar/api/stun_api"
	"linkstar/middleware"

	"github.com/gin-gonic/gin"
)

func StunRouters(g *gin.RouterGroup) {
	var app = api.App.StunApi

	// 输出当前stun配置文件
	g.GET(
		"stun/config",
		app.GetStunConfigView,
	)

	// 输出最近一次 UDP/TCP NAT 类型检测结果
	g.GET(
		"stun/nat-type",
		app.GetNatTypeView,
	)
	g.POST(
		"stun/nat-type/detect",
		app.DetectNatTypeView,
	)

	// 运行时状态快照（一次性）
	g.GET(
		"stun/status",
		app.GetStunStatusView,
	)

	// 运行时状态 SSE 推送
	g.GET(
		"stun/status/events",
		app.StunStatusEventsView,
	)

	// 新增服务
	g.POST(
		"stun/service/add",
		middleware.BindJsonMiddleware[stun_api.StunServiceAddViewRequest],
		app.StunServiceAddView,
	)

	// 本机接在哪几个局域网上
	g.GET(
		"stun/lan/subnets",
		app.StunLanSubnetListView,
	)

	// 扫一段局域网，找出在线的机器和它们开着的端口
	g.POST(
		"stun/lan/scan",
		middleware.BindJsonMiddleware[stun_api.StunLanScanRequest],
		app.StunLanScanView,
	)

	// 新增设备
	g.POST(
		"stun/device/add",
		middleware.BindJsonMiddleware[stun_api.StunDeviceAddViewRequest],
		app.StunDeviceAddView,
	)

	// 修改服务
	g.PUT(
		"stun/service/update",
		middleware.BindJsonMiddleware[stun_api.StunServiceUpdateViewRequest],
		app.StunServiceUpdateView,
	)

	// 删除服务
	g.DELETE(
		"stun/service/delete",
		middleware.BindJsonMiddleware[stun_api.StunServiceDeleteViewRequest],
		app.StunServiceDeleteView,
	)

	// 删除设备
	g.DELETE(
		"stun/device/delete",
		middleware.BindJsonMiddleware[stun_api.StunDeviceDeleteViewRequest],
		app.StunDeviceDeleteView,
	)

	// 修改设备
	g.PUT(
		"stun/device/update",
		middleware.BindJsonMiddleware[stun_api.StunDeviceUpdateViewRequest],
		app.StunDeviceUpdateView,
	)

	// 立即同步入口重定向
	g.POST(
		"stun/redirect/sync",
		middleware.BindJsonMiddleware[stun_api.StunRedirectRequest],
		app.StunRedirectSyncView,
	)

	// 入口/落地这两条解析记录现在各是什么样
	g.POST(
		"stun/redirect/inspect",
		middleware.BindJsonMiddleware[stun_api.StunRedirectRequest],
		app.StunRedirectInspectView,
	)

	// 删掉服务商那边的入口重定向规则
	g.DELETE(
		"stun/redirect",
		middleware.BindJsonMiddleware[stun_api.StunRedirectRequest],
		app.StunRedirectRemoveView,
	)

	// 切换某个 service 是否在 home 显示（与 home 模块联动）
	g.PUT(
		"stun/service/show-on-home",
		middleware.BindJsonMiddleware[stun_api.StunServiceShowOnHomeRequest],
		app.StunServiceShowOnHomeView,
	)

}
