package dns

import (
	"linkstar/modules/ddns/model"
)

// 根据服务商类型创建服务商
func BuildClient(p model.DDNSProvider) DNSProvider {
	switch p.Type {

	// Cloudflare
	case model.DNSProviderCloudflare:
		token, _ := p.Credential["apiToken"].(string)
		return NewCloudflare(token)

	default:
		return nil
	}
}
