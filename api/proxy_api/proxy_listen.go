package proxy_api

import (
	"linkstar/middleware"
	"linkstar/modules/proxy"
	"linkstar/utils/res"

	"github.com/gin-gonic/gin"
)

// EntrySaveRequest 默认入口设置，对应 nginx 的 listen 80 / listen 443 ssl
type EntrySaveRequest struct {
	Enabled   bool `json:"enabled"`
	HTTPPort  int  `json:"httpPort"`  // 0 = 不开 HTTP 入口
	HTTPSPort int  `json:"httpsPort"` // 0 = 不开 HTTPS 入口
	CertID    uint `json:"certId"`    // 0 表示按 SNI 自动匹配

	BaseDomain string `json:"baseDomain"` // 主域名，加站点时只填前缀就能补全
}

// ProxyEntrySaveView 保存默认入口设置并立刻生效
func (ProxyApi) ProxyEntrySaveView(c *gin.Context) {
	cr := middleware.GetBindRequest[EntrySaveRequest](c)

	err := proxy.SetEntry(proxy.EntrySettings{
		Enabled:    cr.Enabled,
		HTTPPort:   cr.HTTPPort,
		HTTPSPort:  cr.HTTPSPort,
		CertID:     cr.CertID,
		BaseDomain: cr.BaseDomain,
	})
	if err != nil {
		// 配置已经存下了，没存下的只是「跑起来」这件事。
		// 照实报错，前端刷新后能看到同一句话挂在对应端口上。
		res.FailWithMsg(err.Error(), c)
		return
	}
	res.OkWithData(buildEntryView(proxy.Runtime.Snapshot()), c)
}

// AccessLogRequest 访问日志开关
type AccessLogRequest struct {
	Enabled bool `json:"enabled"`
}

// ProxyAccessLogView 开关访问日志。
//
// 单独一个接口而不是并进入口表单：它即点即生效，
// 不需要、也不该顺带把监听重开一遍。
func (ProxyApi) ProxyAccessLogView(c *gin.Context) {
	cr := middleware.GetBindRequest[AccessLogRequest](c)
	if err := proxy.SetAccessLog(cr.Enabled); err != nil {
		res.FailWithMsg("保存失败："+err.Error(), c)
		return
	}
	if cr.Enabled {
		res.OkWithMsg("访问日志已开启", c)
		return
	}
	res.OkWithMsg("访问日志已关闭", c)
}
