package stun_api

import (
	"strings"

	"linkstar/modules/cert"
	certmodel "linkstar/modules/cert/model"
	"linkstar/utils/domain"
)

// normalizeTLSFields 收敛对外访问相关字段。
//
// 前端已经在 UDP 时禁用了开关，但接口是公开的，不能指望调用方守规矩：
// UDP 洞口终结的是 DTLS，不是 TLS，这里强制清掉，否则 ForwardUDP 会拿到
// 一个永远用不上的证书配置，而用户看到的却是「已开启」。
func normalizeTLSFields(protocol, domain string, tlsTerminate bool, certID uint) (string, bool, uint) {
	domain = strings.ToLower(strings.TrimSpace(domain))

	if strings.EqualFold(strings.TrimSpace(protocol), "UDP") {
		return domain, false, 0
	}
	if !tlsTerminate {
		// 没开终结就没有绑定证书一说，避免关掉后 CertID 还留着造成误解
		return domain, false, 0
	}
	return domain, true, certID
}

// fillDomainFromCert 终结 TLS 却没填对外域名时，用证书上的域名补上。
//
// 前端已经在勾选时自动填了，但接口是公开的；而且这个洞一旦没有域名，
// ServiceEndpoint 会回落公网 IP（lookup.go:84），首页链接和 /go 跳转就都指向
// 一个证书盖不住的地址——用户点进去只会看到 ERR_CERT_COMMON_NAME_INVALID。
// 证书上既然写着域名，这里就替他填上。
func fillDomainFromCert(domainName string, tlsTerminate bool, certID uint) string {
	if domainName != "" || !tlsTerminate {
		return domainName
	}
	return pickCertDomain(cert.Runtime.Snapshot().Certificates, certID)
}

// pickCertDomain 从证书里挑一个能直接当对外域名用的具体域名。
//
// 通配证书（*.example.com）挑不出来：用哪个标签只有用户知道，替他猜一个
// 反而会让洞看起来配好了、实际指向一个他根本没解析过的名字。返回空串即可，
// 前端那边会要求用户自己填。
func pickCertDomain(certs []certmodel.Certificate, certID uint) string {
	concrete := func(c certmodel.Certificate) string {
		for _, d := range c.Domains {
			if d = domain.Normalize(d); d != "" && !domain.IsWildcard(d) {
				return d
			}
		}
		return ""
	}

	if certID != 0 {
		for _, c := range certs {
			if c.ID == certID {
				return concrete(c)
			}
		}
		return ""
	}

	// 按 SNI 自动匹配：兜底证书是 SNI 落空时真正会用上的那张，优先看它；
	// 它没有具体域名（自签就没有）时，只有唯一一张带域名的证书才好替用户做主
	var withDomain []certmodel.Certificate
	for _, c := range certs {
		if !c.Enabled {
			continue
		}
		if c.IsDefault {
			if d := concrete(c); d != "" {
				return d
			}
		}
		if concrete(c) != "" {
			withDomain = append(withDomain, c)
		}
	}
	if len(withDomain) == 1 {
		return concrete(withDomain[0])
	}
	return ""
}

// normalizeBackendHTTPS 收敛「转发给内网时也用 HTTPS」。
//
// 这个字段管的是 LinkStar 自己怎么拨内网（tls.Dial 还是裸 dial），
// 只有在洞口终结了 TLS、也就是 LinkStar 真的要重新加密一遍时才成立：
//
//   - 洞口终结 + 这个勾：解密后再加密送进内网，对应 nginx 的 proxy_pass https://
//   - 洞口不终结：洞是纯字节管道，LinkStar 一个字节都不解析，
//     浏览器的 TLS 直达内网服务。此时再让它 tls.Dial，就是把浏览器的
//     ClientHello 当明文塞进 LinkStar 自己那条 TLS——双层 TLS，必坏，
//     浏览器侧表现为 ERR_SSL_PROTOCOL_ERROR
//
// UDP 侧没有 tls.Dial 这条路（ForwardUDP 压根不看这个字段），同样清掉。
func normalizeBackendHTTPS(protocol string, backendHTTPS, tlsTerminate bool) bool {
	if strings.EqualFold(strings.TrimSpace(protocol), "UDP") {
		return false
	}
	if !tlsTerminate {
		return false
	}
	return backendHTTPS
}
