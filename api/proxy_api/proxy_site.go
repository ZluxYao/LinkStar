package proxy_api

import (
	"linkstar/middleware"
	"linkstar/modules/proxy"
	proxymodel "linkstar/modules/proxy/model"
	"linkstar/utils/res"

	"github.com/gin-gonic/gin"
)

// SiteSaveRequest 新增 / 修改一个站点，对应 nginx 的一个 server 块。ID 为 0 表示新增。
type SiteSaveRequest struct {
	ID uint `json:"id"`
	// Hosts 域名，如 fn.example.com；支持 *.example.com 通配，可以填多个
	Hosts []string `json:"hosts"`

	PathPrefix  string `json:"pathPrefix"`  // 留空表示整站
	StripPrefix bool   `json:"stripPrefix"` // 转发前剥掉前缀，后端需自己支持 base path

	Backend      string `json:"backend"`      // 192.168.1.20:8080
	BackendHTTPS bool   `json:"backendHttps"` // 后端本身就说 TLS（自签也算）

	// 对外这一侧：不填端口就挂在默认入口上，填了就单独开一个监听
	ListenPort int  `json:"listenPort"` // 0 = 用默认入口
	HTTPS      bool `json:"https"`      // 这个站点对外走不走 HTTPS
	CertID     uint `json:"certId"`     // 0 = 按 SNI 自动匹配

	Enabled     bool   `json:"enabled"`
	Description string `json:"description"`
}

func (r SiteSaveRequest) toModel() proxymodel.Site {
	return proxymodel.Site{
		ID:           r.ID,
		Hosts:        r.Hosts,
		PathPrefix:   r.PathPrefix,
		StripPrefix:  r.StripPrefix,
		Backend:      r.Backend,
		BackendHTTPS: r.BackendHTTPS,
		ListenPort:   r.ListenPort,
		HTTPS:        r.HTTPS,
		CertID:       r.CertID,
		Enabled:      r.Enabled,
		Description:  r.Description,
	}
}

type SiteDeleteRequest struct {
	ID uint `json:"id"`
}

// ProxySiteAddView 新增一个站点
func (ProxyApi) ProxySiteAddView(c *gin.Context) {
	cr := middleware.GetBindRequest[SiteSaveRequest](c)
	cr.ID = 0 // 新增接口不认前端带过来的 ID，免得写成一次静默的覆盖

	saved, err := proxy.SaveSite(cr.toModel())
	if err != nil {
		res.FailWithMsg(err.Error(), c)
		return
	}
	res.OkWithData(saved, c)
}

// ProxySiteUpdateView 修改一个站点
func (ProxyApi) ProxySiteUpdateView(c *gin.Context) {
	cr := middleware.GetBindRequest[SiteSaveRequest](c)
	if cr.ID == 0 {
		res.FailWithMsg("缺少站点 ID", c)
		return
	}

	saved, err := proxy.SaveSite(cr.toModel())
	if err != nil {
		res.FailWithMsg(err.Error(), c)
		return
	}
	res.OkWithData(saved, c)
}

// ProxySiteDeleteView 删除一个站点
func (ProxyApi) ProxySiteDeleteView(c *gin.Context) {
	cr := middleware.GetBindRequest[SiteDeleteRequest](c)
	if err := proxy.DeleteSite(cr.ID); err != nil {
		res.FailWithMsg(err.Error(), c)
		return
	}
	res.OkWithMsg("删除成功", c)
}

// ProxySiteTestView 立即拨一次后端，回报连通性和探到的协议。
//
// 不要求先保存：用户填到一半就能点，省得为了试一下先存一条坏配置。
func (ProxyApi) ProxySiteTestView(c *gin.Context) {
	cr := middleware.GetBindRequest[SiteSaveRequest](c)
	if cr.Backend == "" {
		res.FailWithMsg("请先填写后端地址", c)
		return
	}
	res.OkWithData(proxy.ProbeBackend(cr.toModel()), c)
}
