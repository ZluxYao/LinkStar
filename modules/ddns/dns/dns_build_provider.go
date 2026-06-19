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

	// 阿里云 DNS
	case model.DNSProviderAlidns:
		accessKeyID, _ := p.Credential["accessKeyId"].(string)
		accessKeySecret, _ := p.Credential["accessKeySecret"].(string)
		return NewAlidns(accessKeyID, accessKeySecret)

	// 腾讯云 DNSPod
	case model.DNSProviderTencentCloud:
		secretID, _ := p.Credential["secretId"].(string)
		secretKey, _ := p.Credential["secretKey"].(string)
		return NewTencentCloud(secretID, secretKey)

	// DNSPod 老版(dnsapi.cn)
	case model.DNSProviderDnspod:
		id, _ := p.Credential["id"].(string)
		token, _ := p.Credential["token"].(string)
		return NewDnspod(id, token)

	// 百度云 DNS
	case model.DNSProviderBaiduCloud:
		accessKeyID, _ := p.Credential["accessKeyId"].(string)
		accessKeySecret, _ := p.Credential["accessKeySecret"].(string)
		return NewBaiduCloud(accessKeyID, accessKeySecret)

	// 华为云 DNS
	case model.DNSProviderHuaweiCloud:
		accessKeyID, _ := p.Credential["accessKeyId"].(string)
		accessKeySecret, _ := p.Credential["accessKeySecret"].(string)
		return NewHuaweicloud(accessKeyID, accessKeySecret)

	// GoDaddy
	case model.DNSProviderGoDaddy:
		apiKey, _ := p.Credential["apiKey"].(string)
		apiSecret, _ := p.Credential["apiSecret"].(string)
		return NewGoDaddyDNS(apiKey, apiSecret)

	// NameCheap
	case model.DNSProviderNameCheap:
		password, _ := p.Credential["password"].(string)
		return NewNameCheap(password)

	// NameSilo
	case model.DNSProviderNameSilo:
		apiKey, _ := p.Credential["apiKey"].(string)
		return NewNameSilo(apiKey)

	default:
		return nil
	}
}
