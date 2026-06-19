package ddns_api

import (
	"linkstar/modules/ddns"
	"linkstar/modules/ddns/model"
	"linkstar/utils/res"

	"github.com/gin-gonic/gin"
)

// ProviderView 服务商对外视图：隐藏凭证明文，只回是否已配置
type ProviderView struct {
	ID            uint                  `json:"id"`
	Name          string                `json:"name"`
	Type          model.DNSProviderType `json:"type"`
	HasCredential bool                  `json:"hasCredential"`
}

type DdnsConfigView struct {
	IntervalSec int                `json:"intervalSec"`
	Providers   []ProviderView     `json:"providers"`
	Records     []model.DDNSRecord `json:"records"`
}

// GetDdnsConfigView 返回 DDNS 全量配置（凭证脱敏）
func (DdnsApi) GetDdnsConfigView(c *gin.Context) {
	view := DdnsConfigView{
		Providers: []ProviderView{},
		Records:   []model.DDNSRecord{},
	}

	cfg := ddns.Runtime.Snapshot()
	view.IntervalSec = cfg.IntervalSec
	for _, p := range cfg.Providers {
		view.Providers = append(view.Providers, ProviderView{
			ID:            p.ID,
			Name:          p.Name,
			Type:          p.Type,
			HasCredential: len(p.Credential) > 0,
		})
	}
	view.Records = append(view.Records, cfg.Records...)

	res.OkWithData(view, c)
}
