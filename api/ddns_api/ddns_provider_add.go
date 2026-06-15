package ddns_api

import (
	"linkstar/middleware"
	"linkstar/modules/ddns"
	"linkstar/modules/ddns/model"
	"linkstar/utils/res"
	"time"

	"github.com/gin-gonic/gin"
)

type DdnsProviderAddRequest struct {
	Name     string                `json:"name"`
	Type     model.DNSProviderType `json:"type"`
	APIToken string                `json:"apiToken"` // Cloudflare API Token
}

func (DdnsApi) DdnsProviderAddView(c *gin.Context) {
	cr := middleware.GetBindRequest[DdnsProviderAddRequest](c)

	if cr.Name == "" {
		res.FailWithMsg("服务商名称不能为空", c)
		return
	}
	if cr.Type != model.DNSProviderCloudflare {
		res.FailWithMsg("暂不支持的服务商类型", c)
		return
	}
	if cr.APIToken == "" {
		res.FailWithMsg("API Token 不能为空", c)
		return
	}

	var newProvider model.DDNSProvider
	err := ddns.Runtime.Apply(func(cfg *model.DDNSConfig) error {
		now := time.Now()
		newProvider = model.DDNSProvider{
			ID:         ddns.NextProviderID(cfg),
			Name:       cr.Name,
			Type:       cr.Type,
			Credential: map[string]interface{}{"apiToken": cr.APIToken},
			CreatedAt:  now,
			UpdatedAt:  now,
		}
		cfg.Providers = append(cfg.Providers, newProvider)
		return nil
	})
	if err != nil {
		res.FailWithMsg("保存配置失败", c)
		return
	}

	// 新增服务商需重建调度器，使其拥有对应 worker
	ddns.Runtime.RebuildScheduler()

	res.OkWithData(ProviderView{
		ID:            newProvider.ID,
		Name:          newProvider.Name,
		Type:          newProvider.Type,
		HasCredential: true,
	}, c)
}
