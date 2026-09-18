package stun_api

import (
	"testing"

	certmodel "linkstar/modules/cert/model"
)

// TestPickCertDomain 终结 TLS 时对外域名不该空着——空了就回落公网 IP，
// 而证书永远盖不住 IP，用户点首页链接只会看到 ERR_CERT_COMMON_NAME_INVALID。
// 证书上写着域名就直接拿来用，挑不出具体名字时老老实实返回空让用户自己填。
func TestPickCertDomain(t *testing.T) {
	zlux := certmodel.Certificate{ID: 1, Enabled: true, Domains: []string{"zlux.top", "*.zlux.top"}}
	wildcardOnly := certmodel.Certificate{ID: 2, Enabled: true, Domains: []string{"*.zlux.top"}}
	selfSigned := certmodel.Certificate{ID: 3, Enabled: true, IsDefault: true}
	other := certmodel.Certificate{ID: 4, Enabled: true, Domains: []string{"nas.example.com"}}

	cases := []struct {
		name   string
		certs  []certmodel.Certificate
		certID uint
		want   string
	}{
		{"绑定的证书有具体域名", []certmodel.Certificate{zlux}, 1, "zlux.top"},
		{"通配证书挑不出标签，让用户自己填", []certmodel.Certificate{wildcardOnly}, 2, ""},
		{"自签证书本来就没域名", []certmodel.Certificate{selfSigned}, 3, ""},
		{"绑了一张不存在的证书", []certmodel.Certificate{zlux}, 9, ""},
		{"自动匹配 + 兜底证书带域名", []certmodel.Certificate{other, {ID: 5, Enabled: true, IsDefault: true, Domains: []string{"zlux.top"}}}, 0, "zlux.top"},
		{"自动匹配 + 兜底是自签，只有一张带域名的就用它", []certmodel.Certificate{selfSigned, zlux}, 0, "zlux.top"},
		{"自动匹配 + 多张带域名，不替用户做主", []certmodel.Certificate{zlux, other}, 0, ""},
		{"停用的证书不算数", []certmodel.Certificate{{ID: 6, Domains: []string{"off.zlux.top"}}, zlux}, 0, "zlux.top"},
		{"一张证书都没有", nil, 0, ""},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := pickCertDomain(c.certs, c.certID); got != c.want {
				t.Errorf("pickCertDomain(certID=%d) = %q, 想要 %q", c.certID, got, c.want)
			}
		})
	}
}

// TestNormalizeBackendHTTPS 「转发给内网时也用 HTTPS」只有在洞口终结了 TLS 时才成立。
//
// 踩过的坑：内网是个已经带证书的 HTTPS 服务（反向代理的 6660），
// 用户按字面意思勾了这个、却没勾洞口终结——洞口是纯管道，
// 浏览器的 ClientHello 被当成明文塞进 LinkStar 自己那条 TLS，
// 双层 TLS，浏览器报 ERR_SSL_PROTOCOL_ERROR。
// 接口是公开的，不能指望调用方守规矩，在这里挡掉。
func TestNormalizeBackendHTTPS(t *testing.T) {
	cases := []struct {
		name         string
		protocol     string
		backendHTTPS bool
		tlsTerminate bool
		want         bool
	}{
		{"终结 + 内网 HTTPS：解密再加密，成立", "TCP", true, true, true},
		{"终结但内网是明文", "TCP", false, true, false},
		{"不终结却要 tls.Dial：双层 TLS，清掉", "TCP", true, false, false},
		{"两个都不勾：纯管道", "TCP", false, false, false},
		{"UDP 没有 tls.Dial 这条路", "UDP", true, true, false},
		{"UDP 小写也认", "udp", true, true, false},
		{"协议带空格", " TCP ", true, true, true},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := normalizeBackendHTTPS(c.protocol, c.backendHTTPS, c.tlsTerminate); got != c.want {
				t.Errorf("normalizeBackendHTTPS(%q, %v, %v) = %v, 想要 %v",
					c.protocol, c.backendHTTPS, c.tlsTerminate, got, c.want)
			}
		})
	}
}
