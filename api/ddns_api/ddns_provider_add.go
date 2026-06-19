package ddns_api

import (
	"linkstar/middleware"
	"linkstar/modules/ddns"
	"linkstar/modules/ddns/model"
	"linkstar/utils/res"

	"github.com/gin-gonic/gin"
)

type DdnsProviderAddRequest struct {
	Name     string                `json:"name"`
	Type     model.DNSProviderType `json:"type"`
	APIToken string                `json:"apiToken"` // Cloudflare API Token
}

func (DdnsApi) DdnsProviderAddView(c *gin.Context) {
	cr := middleware.GetBindRequest[DdnsProviderAddRequest](c)

	p, err := ddns.Runtime.AddProvider(cr.Name, cr.Type, cr.APIToken)
	if err != nil {
		res.FailWithMsg(err.Error(), c)
		return
	}

	res.OkWithData(ProviderView{
		ID:            p.ID,
		Name:          p.Name,
		Type:          p.Type,
		HasCredential: true,
	}, c)
}
