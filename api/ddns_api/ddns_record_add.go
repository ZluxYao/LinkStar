package ddns_api

import (
	"linkstar/middleware"
	"linkstar/modules/ddns"
	"linkstar/modules/ddns/model"
	"linkstar/utils/res"
	"time"

	"github.com/gin-gonic/gin"
)

type DdnsRecordAddRequest struct {
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

func (DdnsApi) DdnsRecordAddView(c *gin.Context) {
	cr := middleware.GetBindRequest[DdnsRecordAddRequest](c)

	if msg := validateRecord(cr.Name, cr.Domain, cr.RecordType, cr.IPSourceType); msg != "" {
		res.FailWithMsg(msg, c)
		return
	}

	var newRecord model.DDNSRecord
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

		now := time.Now()
		newRecord = model.DDNSRecord{
			Enabled:      cr.Enabled,
			ID:           ddns.NextRecordID(cfg),
			ProviderID:   cr.ProviderID,
			Name:         cr.Name,
			Domain:       cr.Domain,
			SubDomain:    cr.SubDomain,
			RecordType:   cr.RecordType,
			IPSourceType: cr.IPSourceType,
			IPSourceArg:  cr.IPSourceArg,
			TTL:          cr.TTL,
			Proxied:      cr.Proxied,
			LastStatus:   model.DDNSRecordStatusPending,
			CreatedAt:    now,
			UpdatedAt:    now,
		}
		cfg.Records = append(cfg.Records, newRecord)
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

	// 新增记录后立即触发一次同步
	ddns.Runtime.TriggerSync()

	res.OkWithData(newRecord, c)
}

// validateRecord 公共字段校验，返回空串表示通过
func validateRecord(name, domain string, recordType model.DNSRecordType, ipSource model.IPSourceType) string {
	if name == "" {
		return "记录名称不能为空"
	}
	if domain == "" {
		return "主域名不能为空"
	}
	if recordType != model.DNSRecordTypeA && recordType != model.DNSRecordTypeAAAA {
		return "记录类型仅支持 A / AAAA"
	}
	switch ipSource {
	case model.IPSourceSTUN, model.IPSourceWeb, model.IPSourceDNS, model.IPSourceInterface:
	default:
		return "无效的 IP 来源类型"
	}
	return ""
}
