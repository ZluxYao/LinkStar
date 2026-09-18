package cert_api

import (
	"linkstar/modules/ddns"
	"linkstar/modules/ddns/dns"
	"linkstar/modules/ddns/model"
	"linkstar/utils/res"

	"github.com/gin-gonic/gin"
)

// ACMEProviderView DNS-01 可选服务商。
// 复用 DDNS 里已经配好凭证的服务商，用户不用再填一遍 token。
type ACMEProviderView struct {
	ID          uint                  `json:"id"`
	Name        string                `json:"name"`
	Type        model.DNSProviderType `json:"type"`
	SupportsDNS bool                  `json:"supportsDns01"` // false 时前端应禁选并说明原因
}

// CertProvidersView 列出可用于 DNS-01 的服务商
func (CertApi) CertProvidersView(c *gin.Context) {
	cfg := ddns.Runtime.Snapshot()

	list := make([]ACMEProviderView, 0, len(cfg.Providers))
	for _, p := range cfg.Providers {
		list = append(list, ACMEProviderView{
			ID:          p.ID,
			Name:        p.Name,
			Type:        p.Type,
			SupportsDNS: dns.SupportsACMEDNS(p.Type),
		})
	}
	res.OkWithList(list, int64(len(list)), c)
}
