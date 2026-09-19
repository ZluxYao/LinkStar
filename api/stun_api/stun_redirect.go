package stun_api

import (
	"linkstar/middleware"
	"linkstar/modules/stun"
	"linkstar/utils/res"

	"github.com/gin-gonic/gin"
)

type StunRedirectRequest struct {
	DeviceID  uint `json:"deviceId"`
	ServiceID uint `json:"serviceId"`
}

// StunRedirectSyncView 立即把服务当前的外网地址写到入口域名上。
// 平时端口一变会自动同步，这个按钮是给「刚配完想马上看效果」和同步失败后重试用的。
func (StunApi) StunRedirectSyncView(c *gin.Context) {
	cr := middleware.GetBindRequest[StunRedirectRequest](c)

	r, err := stun.SyncRedirect(cr.DeviceID, cr.ServiceID)
	if err != nil {
		res.FailWithMsg(err.Error(), c)
		return
	}

	msg := "已同步：" + r.Target
	if !r.KeepPath {
		msg += "（服务商不接受保留路径的写法，从子路径进来会落到根路径）"
	}
	switch {
	case r.LandingAdded:
		msg += "；已在 DDNS 里加上 " + r.LandingHost + "，之后公网 IP 变了它自己会跟"
	case r.LandingWarn != "":
		msg += "；" + r.LandingWarn
	}
	// 入口域名那条记录没确认上的话，规则写得再对访问也是域名不存在，
	// 这句必须跟着「已同步」一起出现，不然用户只会看到一片成功
	if r.EntryWarn != "" {
		msg += "；" + r.EntryWarn
	}
	res.Ok(gin.H{
		"target":       r.Target,
		"keepPath":     r.KeepPath,
		"entryWarn":    r.EntryWarn,
		"landingHost":  r.LandingHost,
		"landingAdded": r.LandingAdded,
		"landingWarn":  r.LandingWarn,
	}, msg, c)
}

// StunRedirectInspectView 入口重定向靠的那两条解析记录现在各是什么样。
//
// 只读，但仍走 POST：和同 Body 的另外两个接口保持一致，少一套参数绑定。
// 入口那条要现场去服务商查（可能几百毫秒），所以做成单独接口按需调，
// 不塞进服务列表里跟着每次刷新一起跑。
func (StunApi) StunRedirectInspectView(c *gin.Context) {
	cr := middleware.GetBindRequest[StunRedirectRequest](c)

	r, err := stun.InspectRedirect(cr.DeviceID, cr.ServiceID)
	if err != nil {
		res.FailWithMsg(err.Error(), c)
		return
	}
	res.OkWithData(r, c)
}

// StunRedirectRemoveView 删掉服务商那边 LinkStar 写的那条规则。
//
// 单独做一个按钮而不是「关掉开关就删」：关开关是本地配置，删规则是打远端 API，
// 会失败。把它们绑在一起的话，API 一失败用户就会以为已经删干净了，
// 实际上那条规则还在，还指着一个早就换掉的端口。
func (StunApi) StunRedirectRemoveView(c *gin.Context) {
	cr := middleware.GetBindRequest[StunRedirectRequest](c)

	if err := stun.RemoveRedirect(cr.DeviceID, cr.ServiceID); err != nil {
		res.FailWithMsg(err.Error(), c)
		return
	}
	res.OkWithMsg("已删除服务商那边的规则", c)
}
