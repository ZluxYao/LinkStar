package proxy

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"linkstar/modules/proxy/model"
	"linkstar/utils/domain"
)

// 本文件是 api 层改配置的唯一入口。
//
// 校验刻意放在模块里而不是 api 层：站点表的不变量（域名必填、同域名同前缀
// 不能重复）是转发行为的一部分，让每个调用方各写一份迟早会写歪。

// ==== 站点 ====

// SaveSite 新增或更新一个站点，ID 为 0 表示新增
func SaveSite(in model.Site) (model.Site, error) {
	site, err := normalizeSite(in, Runtime.Snapshot().BaseDomain)
	if err != nil {
		return model.Site{}, err
	}

	var saved model.Site
	err = Runtime.Update(func(cfg *model.ProxyConfig) error {
		if dup, host, ok := conflictSite(cfg.Sites, site); ok {
			return fmt.Errorf("%s 上的%s已经配过了（后端 %s），换一个路径，或者去改那一条",
				host, prefixLabel(dup.PathPrefix), dup.Backend)
		}
		if err := checkPortProtocol(cfg, site); err != nil {
			return err
		}
		site.UpdatedAt = time.Now()

		if site.ID == 0 {
			site.ID = nextSiteID(cfg)
			cfg.Sites = append(cfg.Sites, site)
			saved = site
			return nil
		}
		for i := range cfg.Sites {
			if cfg.Sites[i].ID == site.ID {
				cfg.Sites[i] = site
				saved = site
				return nil
			}
		}
		return errors.New("站点不存在")
	})
	if err != nil {
		return model.Site{}, err
	}
	return saved, nil
}

// DeleteSite 删除一个站点
func DeleteSite(id uint) error {
	return Runtime.Update(func(cfg *model.ProxyConfig) error {
		for i := range cfg.Sites {
			if cfg.Sites[i].ID == id {
				cfg.Sites = append(cfg.Sites[:i], cfg.Sites[i+1:]...)
				return nil
			}
		}
		return errors.New("站点不存在")
	})
}

func normalizeSite(in model.Site, base string) (model.Site, error) {
	out := in
	baseDomain := domain.Normalize(base)

	hosts := make([]string, 0, len(out.Hosts))
	for _, raw := range out.Hosts {
		h := domain.Normalize(raw)
		if h == "" {
			continue
		}
		// 先挑毛病再补主域名：写成 nas:8080 的话补完会变成 nas:8080.example.com，
		// 报错时把那个怪东西原样甩给用户只会更难懂
		if strings.ContainsAny(h, "/:") {
			return out, fmt.Errorf("域名 %s 只填域名本身，不要带协议、端口或路径", h)
		}
		hosts = append(hosts, expandHost(h, baseDomain))
	}
	out.Hosts = NormalizeHosts(hosts)

	out.Backend = strings.TrimSpace(out.Backend)
	out.PathPrefix = normalizePrefix(out.PathPrefix)
	out.Description = strings.TrimSpace(out.Description)

	switch {
	// 域名留空 = 当端口转发使，那这个端口上只能有它一条，所以端口必填。
	// nginx 里不写 server_name 的 server 块同理，得自己占一个 listen。
	case len(out.Hosts) == 0 && out.ListenPort == 0:
		return out, errors.New("要么填域名，要么给它填一个自己的端口——挂在默认入口上的站点靠域名分流，没域名分不开")
	case out.Backend == "":
		return out, errors.New("后端地址必填，形如 192.168.1.20:8080")
	case strings.ContainsAny(out.Backend, " \t"):
		return out, errors.New("后端地址里有空格，请检查")
	case strings.HasPrefix(out.Backend, "/"):
		return out, errors.New("后端地址要填主机和端口，形如 192.168.1.20:8080")
	}

	if out.ListenPort != 0 {
		if out.ListenPort < 0 || out.ListenPort > 65535 {
			return out, errors.New("端口要填 1 到 65535 之间的数字")
		}
		if out.ListenPort == adminPort {
			return out, fmt.Errorf("%d 是 LinkStar 自己的后台端口，换一个", adminPort)
		}
	}
	if !out.HTTPS {
		// 没开 HTTPS 就没有绑定证书一说，和 STUN 那边同一个取舍
		out.CertID = 0
	}
	return out, nil
}

// checkPortProtocol 同一个端口不能既是 http 又是 https。
//
// 一个监听只有一个 tls.Config，两边都想要的话必然有一边打不开——
// 与其让用户对着一个打不开的站点猜，不如在保存的时候就说清楚。
// 证书不同倒无所谓：那是 SNI 该干的事，planBindings 会自动退回自动匹配。
func checkPortProtocol(cfg *model.ProxyConfig, site model.Site) error {
	port := site.ListenPort
	if port == 0 || !site.Enabled {
		return nil
	}
	if port == cfg.HTTPPort && site.HTTPS {
		return fmt.Errorf("端口 %d 是默认 HTTP 入口，想让这个站点走 HTTPS 就别单独指定端口，或者换一个端口", port)
	}
	if port == cfg.HTTPSPort && !site.HTTPS {
		return fmt.Errorf("端口 %d 是默认 HTTPS 入口，这个站点没开 HTTPS——换一个端口，或者把 HTTPS 打开", port)
	}

	for _, s := range cfg.Sites {
		if s.ID == site.ID || !s.Enabled || s.ListenPort != port {
			continue
		}
		if s.HTTPS != site.HTTPS {
			return fmt.Errorf("端口 %d 上的站点 %s 是 %s，同一个端口不能一半 HTTP 一半 HTTPS",
				port, s.Label(), schemeLabel(s.HTTPS))
		}
	}
	return nil
}

func schemeLabel(https bool) string {
	if https {
		return "HTTPS"
	}
	return "HTTP"
}

// expandHost 用主域名把前缀补成完整域名：配了 example.com 之后，nas → nas.example.com。
//
// 判据是「有没有点」：带点的当成用户已经写全了，原样保留——
// 他完全可能同时代理另一个域名下的服务，不该被主域名绑死。
func expandHost(host, base string) string {
	if host == "" || base == "" || strings.Contains(host, ".") {
		return host
	}
	if host == "*" {
		return "*." + base
	}
	return host + "." + base
}

// conflictSite 同一个域名下同一个路径前缀只能有一条。
//
// 一个站点能挂好几个域名，所以撞车的判据是「域名集合有交集」。
// 返回值里带上撞的是哪个域名：只说「和某某站点冲突」的话，
// 挂了五个域名的那条站点还得让用户自己去比对。
//
// 没填域名的站点比不了域名，改按端口比：那种站点是所在端口的兜底，
// 一个端口上同一个路径有两条兜底的话，谁生效全看排序，等于抛硬币。
func conflictSite(sites []model.Site, site model.Site) (model.Site, string, bool) {
	if len(site.Hosts) == 0 {
		for _, s := range sites {
			if s.ID == site.ID || normalizePrefix(s.PathPrefix) != site.PathPrefix {
				continue
			}
			if len(NormalizeHosts(s.Hosts)) > 0 || s.ListenPort != site.ListenPort {
				continue
			}
			return s, site.Label(), true
		}
		return model.Site{}, "", false
	}

	want := make(map[string]bool, len(site.Hosts))
	for _, h := range site.Hosts {
		want[h] = true
	}
	for _, s := range sites {
		if s.ID == site.ID || normalizePrefix(s.PathPrefix) != site.PathPrefix {
			continue
		}
		for _, h := range NormalizeHosts(s.Hosts) {
			if want[h] {
				return s, h, true
			}
		}
	}
	return model.Site{}, "", false
}

func prefixLabel(prefix string) string {
	if prefix == "" {
		return "整站转发"
	}
	return "路径 " + prefix
}

// ==== 默认入口 ====

// adminPort LinkStar 后台自己占的端口，反代不能抢
const adminPort = 3333

// EntrySettings 两个默认入口：HTTP 一个端口，HTTPS 一个端口。
//
// 对应 nginx 的 listen 80 / listen 443 ssl，也对应 GoDoxy 的
// HTTP_ADDR / HTTPS_ADDR。站点不单独指定端口就挂在这两个上面。
type EntrySettings struct {
	Enabled    bool
	HTTPPort   int
	HTTPSPort  int
	CertID     uint
	BaseDomain string
}

// SetEntry 保存默认入口设置并立刻生效。
// 端口占用之类的失败会原样返回——配置已经存下了，用户改个端口再存一次即可。
func SetEntry(in EntrySettings) error {
	if err := checkEntryPort("HTTP 端口", in.HTTPPort); err != nil {
		return err
	}
	if err := checkEntryPort("HTTPS 端口", in.HTTPSPort); err != nil {
		return err
	}
	if in.Enabled && in.HTTPPort == 0 && in.HTTPSPort == 0 {
		return errors.New("HTTP 和 HTTPS 端口至少要填一个，否则没有入口")
	}
	if in.HTTPPort != 0 && in.HTTPPort == in.HTTPSPort {
		return errors.New("HTTP 和 HTTPS 不能用同一个端口")
	}

	base := domain.Normalize(in.BaseDomain)
	if strings.ContainsAny(base, "/:*") {
		return errors.New("主域名只填域名本身，形如 example.com")
	}

	return Runtime.Update(func(cfg *model.ProxyConfig) error {
		cfg.Enabled = in.Enabled
		cfg.BaseDomain = base
		cfg.HTTPPort = in.HTTPPort
		cfg.HTTPSPort = in.HTTPSPort
		cfg.CertID = in.CertID
		if in.HTTPSPort == 0 {
			cfg.CertID = 0
		}
		return nil
	})
}

// checkEntryPort 0 表示不开这个入口，是合法的
func checkEntryPort(label string, port int) error {
	switch {
	case port == 0:
		return nil
	case port < 0 || port > 65535:
		return fmt.Errorf("%s要填 1 到 65535 之间的数字", label)
	case port == adminPort:
		return fmt.Errorf("%d 是 LinkStar 自己的后台端口，换一个", adminPort)
	}
	return nil
}

// SetAccessLog 单独开关访问日志，不碰监听
func SetAccessLog(on bool) error {
	return Runtime.Update(func(cfg *model.ProxyConfig) error {
		cfg.AccessLog = on
		return nil
	})
}
