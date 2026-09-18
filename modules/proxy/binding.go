package proxy

import (
	"sort"

	"linkstar/modules/proxy/model"
)

// binding 一个监听端口，以及挂在它上面的站点。
//
// 这就是 GoDoxy 的 entrypoint.servers（一个 addr → *http.Server 的 map）
// 在 LinkStar 里的样子：端口不是全局唯一的，有几个 binding 就开几个监听。
type binding struct {
	Port   int
	TLS    bool
	CertID uint // 0 = 按 SNI 自动匹配
	Sites  []model.Site
}

// listenSpec 决定要不要重开监听的那几个参数。
//
// 站点表刻意不在里面：改站点只换转发表指针，正在传的下载、
// WebSocket、SSE 一条都不该被打断。
type listenSpec struct {
	Port   int
	TLS    bool
	CertID uint
}

func (b binding) spec() listenSpec {
	return listenSpec{Port: b.Port, TLS: b.TLS, CertID: b.CertID}
}

// planBindings 算出「现在该开哪几个监听、每个上面挂哪些站点」。
//
// 规则和 GoDoxy 的 getAddr 一致：
//   - 站点没指定端口 → 挂默认 HTTP 入口；勾了 HTTPS 就再挂一份到默认 HTTPS 入口
//   - 站点指定了端口 → 那个端口单独起一个监听，是 http 还是 https 由站点自己说了算
//
// 返回结果按端口排序，保证每次重算顺序稳定，日志和页面上不会来回跳。
func planBindings(cfg model.ProxyConfig) []binding {
	if !cfg.Enabled {
		return nil
	}

	var shared []model.Site          // 走默认入口的
	custom := map[int][]model.Site{} // 自己占端口的，按端口分组
	for _, s := range cfg.Sites {
		if !s.Enabled {
			continue
		}
		// 自己占的端口正好是默认入口时按默认入口处理：
		// 同一个端口不可能 bind 两次，硬开只会有一个失败。
		if s.ListenPort > 0 && s.ListenPort != cfg.HTTPPort && s.ListenPort != cfg.HTTPSPort {
			custom[s.ListenPort] = append(custom[s.ListenPort], s)
			continue
		}
		shared = append(shared, s)
	}

	var out []binding

	if cfg.HTTPPort > 0 && len(shared) > 0 {
		// HTTP 入口收下全部共享站点：勾了 HTTPS 的站点在这儿依然能用明文访问，
		// 内网里直接敲 IP 或者还没配证书的时候不至于打不开
		out = append(out, binding{Port: cfg.HTTPPort, Sites: shared})
	}

	if cfg.HTTPSPort > 0 {
		var tlsSites []model.Site
		for _, s := range shared {
			if s.HTTPS {
				tlsSites = append(tlsSites, s)
			}
		}
		if len(tlsSites) > 0 {
			out = append(out, binding{
				Port:   cfg.HTTPSPort,
				TLS:    true,
				CertID: sharedCertID(tlsSites, cfg.CertID),
				Sites:  tlsSites,
			})
		}
	}

	for port, sites := range custom {
		out = append(out, binding{
			Port:   port,
			TLS:    anyHTTPS(sites),
			CertID: sharedCertID(sites, 0),
			Sites:  sites,
		})
	}

	sort.Slice(out, func(i, j int) bool { return out[i].Port < out[j].Port })
	return out
}

// sharedCertID 一个监听上要用哪张证书。
//
// 一个端口只有一个 tls.Config，所以站点各自指定的证书在这儿要合成一个：
// 大家指的都是同一张就用那张，指得不一样（或者压根没指）就交给 SNI 自动匹配——
// 这正是自动匹配存在的意义，多域名共用一个端口本来就该按 SNI 发不同的证书。
func sharedCertID(sites []model.Site, fallback uint) uint {
	var picked uint
	for _, s := range sites {
		if s.CertID == 0 {
			continue
		}
		if picked != 0 && picked != s.CertID {
			return 0
		}
		picked = s.CertID
	}
	if picked != 0 {
		return picked
	}
	return fallback
}

func anyHTTPS(sites []model.Site) bool {
	for _, s := range sites {
		if s.HTTPS {
			return true
		}
	}
	return false
}
