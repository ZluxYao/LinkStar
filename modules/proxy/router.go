package proxy

import (
	"net"
	"net/http/httputil"
	"sort"
	"strings"

	"linkstar/modules/proxy/model"
	"linkstar/utils/domain"
)

// siteEntry 一个编译好的站点：配置 + 现成的转发器。
//
// ReverseProxy 在建表时就造好，请求路径上只做查表，不构造对象。
type siteEntry struct {
	Site  model.Site
	proxy *httputil.ReverseProxy
}

// hostGroup 同一个域名（或同一条通配模式）下的全部站点，按前缀从长到短排好
type hostGroup struct {
	pattern string
	entries []*siteEntry
}

// Router 不可变的站点表。
//
// 配置一改就整张重建、换指针，所以请求路径上读它不需要加锁，
// 也不会出现「读到改了一半的表」。
type Router struct {
	exact    map[string]*hostGroup
	wildcard []*hostGroup
	// fallback 没填域名的站点。这个端口上不分流，什么域名都收。
	//
	// 它只会出现在那个站点自己占的端口的表里（保存时就要求填端口），
	// 所以不存在「一条没写域名的站点把默认入口上别人的请求全吃掉」。
	fallback *hostGroup
	hosts    []string // 已配置的域名列表，404 页面上列给用户看
}

// NewRouter 把站点配置编译成转发表，跳过未启用和填写不全的条目
func NewRouter(sites []model.Site) *Router {
	r := &Router{exact: make(map[string]*hostGroup)}

	for _, site := range sites {
		if !site.Enabled {
			continue
		}
		hosts := NormalizeHosts(site.Hosts)
		backend := strings.TrimSpace(site.Backend)
		if backend == "" {
			continue
		}
		site.Hosts = hosts
		site.Backend = backend
		site.PathPrefix = normalizePrefix(site.PathPrefix)

		// 一个站点一个转发器，几个域名共用同一个：
		// 转发到哪、怎么转发都和用户敲的是哪个域名无关。
		entry := &siteEntry{Site: site, proxy: newReverseProxy(site)}

		if len(hosts) == 0 {
			if r.fallback == nil {
				r.fallback = &hostGroup{pattern: "(不限域名)"}
			}
			r.fallback.entries = append(r.fallback.entries, entry)
			continue
		}
		for _, host := range hosts {
			group := r.groupFor(host)
			group.entries = append(group.entries, entry)
		}
	}

	// 长前缀优先：/photo/raw 必须排在 /photo 前面，否则永远匹配不到
	for _, g := range r.exact {
		sortEntries(g.entries)
		r.hosts = append(r.hosts, g.pattern)
	}
	for _, g := range r.wildcard {
		sortEntries(g.entries)
		r.hosts = append(r.hosts, g.pattern)
	}
	sort.Strings(r.hosts)
	if r.fallback != nil {
		sortEntries(r.fallback.entries)
	}

	return r
}

func (r *Router) groupFor(host string) *hostGroup {
	if domain.IsWildcard(host) {
		for _, g := range r.wildcard {
			if g.pattern == host {
				return g
			}
		}
		g := &hostGroup{pattern: host}
		r.wildcard = append(r.wildcard, g)
		return g
	}
	if g, ok := r.exact[host]; ok {
		return g
	}
	g := &hostGroup{pattern: host}
	r.exact[host] = g
	return g
}

// Match 按 Host + 路径找一个站点。
//
// 精确 Host 优先于通配 Host——用户为某个名字单独写的站点不该被 *.zlux.top 抢走。
// 都没中再看没填域名的站点，它才是最后兜底的那一层。
func (r *Router) Match(hostHeader, path string) *siteEntry {
	// Host 为空也要往下走：没填域名的站点本来就不看 Host，
	// HTTP/1.0 的客户端或者裸 TCP 探针不带 Host 也该转过去。
	if host := normalizeHostHeader(hostHeader); host != "" {
		if g, ok := r.exact[host]; ok {
			if e := g.match(path); e != nil {
				return e
			}
		}
		for _, g := range r.wildcard {
			if !domain.MatchWildcard(g.pattern, host) {
				continue
			}
			if e := g.match(path); e != nil {
				return e
			}
		}
	}
	if r.fallback != nil {
		return r.fallback.match(path)
	}
	return nil
}

// Hosts 已配置的域名列表
func (r *Router) Hosts() []string {
	return r.hosts
}

// Empty 站点表里一个可用站点都没有
func (r *Router) Empty() bool {
	return len(r.exact) == 0 && len(r.wildcard) == 0 && r.fallback == nil
}

// Size 表里有多少条可用站点。
//
// 按 entry 去重：一个站点挂几个域名就会出现在几个 hostGroup 里，
// 直接把各组条数加起来的话，页面上「3 站点」其实只有 2 条。
func (r *Router) Size() int {
	seen := make(map[*siteEntry]bool)
	count := func(g *hostGroup) {
		for _, e := range g.entries {
			seen[e] = true
		}
	}
	for _, g := range r.exact {
		count(g)
	}
	for _, g := range r.wildcard {
		count(g)
	}
	if r.fallback != nil {
		count(r.fallback)
	}
	return len(seen)
}

func (g *hostGroup) match(path string) *siteEntry {
	for _, e := range g.entries {
		if matchPrefix(e.Site.PathPrefix, path) {
			return e
		}
	}
	return nil
}

// sortEntries 前缀长的排前面，等长的按字典序，保证每次重建顺序稳定
func sortEntries(entries []*siteEntry) {
	sort.SliceStable(entries, func(i, j int) bool {
		a, b := entries[i].Site.PathPrefix, entries[j].Site.PathPrefix
		if len(a) != len(b) {
			return len(a) > len(b)
		}
		return a < b
	})
}

// NormalizeHosts 规整站点的域名列表：统一小写、去掉空项、按输入顺序去重。
//
// 去重是必须的：同一个站点重复写了一个域名的话，那条 hostGroup 里会挂上
// 两个一模一样的 entry，404 页面上也会把它列两遍。
func NormalizeHosts(in []string) []string {
	out := make([]string, 0, len(in))
	seen := make(map[string]bool, len(in))
	for _, h := range in {
		h = domain.Normalize(h)
		if h == "" || seen[h] {
			continue
		}
		seen[h] = true
		out = append(out, h)
	}
	return out
}

// normalizePrefix 规整成 "/photo" 形式：有前导斜杠、无尾部斜杠；整站为空串
func normalizePrefix(prefix string) string {
	p := strings.TrimSpace(prefix)
	if p == "" || p == "/" {
		return ""
	}
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	return strings.TrimRight(p, "/")
}

// matchPrefix 路径是否落在该前缀下。
//
// 必须卡在路径分隔处：前缀 /photo 不能匹配 /photobooth，
// 否则用户一加新站点就会被老站点吃掉。
func matchPrefix(prefix, path string) bool {
	if prefix == "" {
		return true
	}
	if !strings.HasPrefix(path, prefix) {
		return false
	}
	rest := path[len(prefix):]
	return rest == "" || strings.HasPrefix(rest, "/")
}

// normalizeHostHeader 从 Host 头里取出域名部分。
//
// 反代不一定监听在 80/443，浏览器会把端口带在 Host 里（zlux.top:8080）；
// 而站点表里用户填的是纯域名，不剥端口就永远匹配不上。
// IPv6 字面量形如 [::1]:8080，交给 SplitHostPort 处理。
func normalizeHostHeader(hostHeader string) string {
	h := strings.TrimSpace(hostHeader)
	if h == "" {
		return ""
	}
	if host, _, err := net.SplitHostPort(h); err == nil {
		h = host
	}
	return domain.Normalize(strings.Trim(h, "[]"))
}
