package ddns_api

import (
	"linkstar/middleware"
	"linkstar/modules/ddns"
	"linkstar/modules/ddns/model"
	"linkstar/utils/res"

	"github.com/gin-gonic/gin"
)

type DdnsProviderAddRequest struct {
	Name       string                `json:"name"`
	Type       model.DNSProviderType `json:"type"`
	Credential map[string]string     `json:"credential"` // 凭证字段，key 因服务商而异
}

func (DdnsApi) DdnsProviderAddView(c *gin.Context) {
	cr := middleware.GetBindRequest[DdnsProviderAddRequest](c)

	p, err := ddns.Runtime.AddProvider(cr.Name, cr.Type, cr.Credential)
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
