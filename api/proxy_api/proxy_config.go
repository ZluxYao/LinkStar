package proxy_api

import (
	"linkstar/modules/proxy"
	proxymodel "linkstar/modules/proxy/model"
	"linkstar/utils/res"

	"github.com/gin-gonic/gin"
)

// ProxyConfigView 反向代理页要的全部数据。
//
// 三块：默认入口的设置、此刻实际开着的监听、站点表。
// 对照 nginx 就是「默认 listen 那几行」「netstat 看到的端口」「全部 server 块」。
type ProxyConfigView struct {
	Entry     EntryView           `json:"entry"`
	Listeners []proxy.ListenState `json:"listeners"`
	Sites     []proxymodel.Site   `json:"sites"`
}

// EntryView 两个默认入口的设置。
//
// 这里是用户的意愿；实际跑没跑起来看 Listeners——
// 端口被占用时就是「开着但没跑起来」，原因在那条 listener 的 lastError 上。
type EntryView struct {
	Enabled    bool   `json:"enabled"`
	HTTPPort   int    `json:"httpPort"`
	HTTPSPort  int    `json:"httpsPort"`
	CertID     uint   `json:"certId"`
	AccessLog  bool   `json:"accessLog"`
	BaseDomain string `json:"baseDomain"`
}

// ProxyConfigView 取反向代理的配置与状态
func (ProxyApi) ProxyConfigView(c *gin.Context) {
	cfg := proxy.Runtime.Snapshot()
	res.OkWithData(ProxyConfigView{
		Entry:     buildEntryView(cfg),
		Listeners: proxy.ListenStates(),
		Sites:     cfg.Sites,
	}, c)
}

func buildEntryView(cfg proxymodel.ProxyConfig) EntryView {
	return EntryView{
		Enabled:    cfg.Enabled,
		HTTPPort:   cfg.HTTPPort,
		HTTPSPort:  cfg.HTTPSPort,
		CertID:     cfg.CertID,
		AccessLog:  cfg.AccessLog,
		BaseDomain: cfg.BaseDomain,
	}
}
