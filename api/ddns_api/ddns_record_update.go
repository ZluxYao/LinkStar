package ddns_api

import (
	"linkstar/middleware"
	"linkstar/modules/ddns"
	"linkstar/modules/ddns/model"
	"linkstar/utils/res"

	"github.com/gin-gonic/gin"
)

type DdnsRecordUpdateRequest struct {
	ID           uint                `json:"id"`
	Enabled      bool                `json:"enabled"`
	ProviderID   uint                `json:"providerId"`
	Name         string              `json:"name"`
	Domain       string              `json:"domain"`
	SubDomain    string              `json:"subDomain"`
	RecordType   model.DNSRecordType `json:"recordType"`
	IPSourceType model.IPSourceType  `json:"ipSource"`
	IPSourceArg  string              `json:"ipSourceArg"`
	TTL          int                 `json:"ttl"`
	Proxied      bool                `json:"proxied"`
}

func (DdnsApi) DdnsRecordUpdateView(c *gin.Context) {
	cr := middleware.GetBindRequest[DdnsRecordUpdateRequest](c)

	err := ddns.Runtime.UpdateRecord(cr.ID, ddns.RecordInput{
		Enabled:      cr.Enabled,
		ProviderID:   cr.ProviderID,
		Name:         cr.Name,
		Domain:       cr.Domain,
		SubDomain:    cr.SubDomain,
		RecordType:   cr.RecordType,
		IPSourceType: cr.IPSourceType,
		IPSourceArg:  cr.IPSourceArg,
		TTL:          cr.TTL,
		Proxied:      cr.Proxied,
	})
	if err != nil {
		res.FailWithMsg(err.Error(), c)
		return
	}

	res.OkWithMsg("记录已更新", c)
}
