package ddns_api

import (
	"linkstar/middleware"
	"linkstar/modules/ddns"
	"linkstar/modules/ddns/model"
	"linkstar/utils/res"
	"time"

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

	if msg := validateRecord(cr.Name, cr.Domain, cr.RecordType, cr.IPSourceType); msg != "" {
		res.FailWithMsg(msg, c)
		return
	}

	found := false
	providerOK := false
	err := ddns.Runtime.Apply(func(cfg *model.DDNSConfig) error {
		for _, p := range cfg.Providers {
			if p.ID == cr.ProviderID {
				providerOK = true
				break
			}
		}
		if !providerOK {
			return nil
		}

		for i := range cfg.Records {
			if cfg.Records[i].ID != cr.ID {
				continue
			}
			r := &cfg.Records[i]

			// 目标地址相关字段变了，清空 LastIP 触发重推
			if r.Domain != cr.Domain || r.SubDomain != cr.SubDomain ||
				r.RecordType != cr.RecordType || r.IPSourceType != cr.IPSourceType ||
				r.IPSourceArg != cr.IPSourceArg || r.ProviderID != cr.ProviderID {
				r.LastIP = ""
				r.LastStatus = model.DDNSRecordStatusPending
				r.LastCheckAt = time.Time{}
			}

			r.Enabled = cr.Enabled
			r.ProviderID = cr.ProviderID
			r.Name = cr.Name
			r.Domain = cr.Domain
			r.SubDomain = cr.SubDomain
			r.RecordType = cr.RecordType
			r.IPSourceType = cr.IPSourceType
			r.IPSourceArg = cr.IPSourceArg
			r.TTL = cr.TTL
			r.Proxied = cr.Proxied
			r.UpdatedAt = time.Now()
			found = true
			break
		}
		return nil
	})
	if err != nil {
		res.FailWithMsg("保存配置失败", c)
		return
	}
	if !providerOK {
		res.FailWithMsg("所选服务商不存在", c)
		return
	}
	if !found {
		res.FailWithMsg("记录不存在", c)
		return
	}

	ddns.Runtime.TriggerSync()

	res.OkWithMsg("记录已更新", c)
}
