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

// BuildACMEClient 构建支持 ACME DNS-01 的客户端。
// 服务商未实现 ACMEDNSProvider 时返回 nil，调用方据此提示用户改用其他方式。
func BuildACMEClient(p model.DDNSProvider) ACMEDNSProvider {
	c := BuildClient(p)
	if c == nil {
		return nil
	}
	acmeClient, ok := c.(ACMEDNSProvider)
	if !ok {
		return nil
	}
	return acmeClient
}

// SupportsACMEDNS 该服务商类型是否支持 DNS-01（给前端做能力展示用）
func SupportsACMEDNS(t model.DNSProviderType) bool {
	switch t {
	case model.DNSProviderCloudflare:
		return true
	default:
		return false
	}
}

// BuildRedirectClient 构建支持入口重定向规则同步的客户端。
// 服务商未实现 RedirectRuleProvider 时返回 nil，调用方据此提示用户换服务商。
func BuildRedirectClient(p model.DDNSProvider) RedirectRuleProvider {
	c := BuildClient(p)
	if c == nil {
		return nil
	}
	redirectClient, ok := c.(RedirectRuleProvider)
	if !ok {
		return nil
	}
	return redirectClient
}

// SupportsRedirectRule 该服务商类型是否支持入口重定向（给前端做能力展示用）
func SupportsRedirectRule(t model.DNSProviderType) bool {
	switch t {
	case model.DNSProviderCloudflare:
		return true
	default:
		return false
	}
}
