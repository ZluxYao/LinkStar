package proxy

import (
	"fmt"
	"html"
	"net/http"
	"strings"
)

// 本文件的内容协商与缓存头处理借鉴 GoDoxy
// (github.com/yusing/godoxy, MIT License) 的 origin_unreachable_page.go，
// 页面文案与「指出是哪个后端连不上」为 LinkStar 自有实现。

// setNoStoreHeaders 错误页绝对不能进缓存。
//
// 后端起来之后用户一刷新就该好，如果这个 502 被缓存了，
// 他会以为「修好了但还是坏的」，然后去重启整个 LinkStar。
func setNoStoreHeaders(h http.Header) {
	h.Set("Cache-Control", "no-store, no-cache, must-revalidate, max-age=0")
	h.Set("Pragma", "no-cache")
	h.Set("Expires", "0")
}

// wantsHTML 只有「人用浏览器点进来」才值得给一整页 HTML。
// API 调用方收到一坨 HTML 只会让它的报错更难读。
func wantsHTML(req *http.Request) bool {
	if req == nil || req.Method != http.MethodGet {
		return false
	}
	accept := req.Header.Get("Accept")
	if accept == "" {
		// 没有 Accept 的裸 GET：只有访问根路径时才当成是人
		return req.URL != nil && req.URL.Path == "/"
	}
	return strings.Contains(accept, "text/html") || strings.Contains(accept, "*/*")
}

// writeOriginUnreachable 后端连不上时的 502。
//
// 比浏览器自带的 ERR_CONNECTION_REFUSED 强的地方只有一点：
// 它能说出到底是哪个站点、哪个后端地址连不上。这正是用户当下唯一需要知道的信息。
func writeOriginUnreachable(w http.ResponseWriter, req *http.Request, host, backend string, err error) {
	setNoStoreHeaders(w.Header())

	detail := ""
	if err != nil {
		detail = describeBackendError(err)
	}

	if !wantsHTML(req) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusBadGateway)
		fmt.Fprintf(w, "502 后端连不上\n\n站点: %s\n后端: %s\n", host, backend)
		if detail != "" {
			fmt.Fprintf(w, "原因: %s\n", detail)
		}
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusBadGateway)

	detailHTML := ""
	if detail != "" {
		detailHTML = `<p class="err">` + html.EscapeString(detail) + `</p>`
	}
	fmt.Fprintf(w, originUnreachableTmpl,
		html.EscapeString(host),
		html.EscapeString(backend),
		detailHTML,
	)
}

// writeNoSite 收到了一个没有任何站点认领的 Host。
//
// 把已配置的域名列出来——用户十有八九是把域名拼错了，或者忘了那个站点没启用。
func writeNoSite(w http.ResponseWriter, req *http.Request, hostHeader string, hosts []string) {
	setNoStoreHeaders(w.Header())

	if !wantsHTML(req) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprintf(w, "404 没有匹配的站点\n\n请求的 Host: %s\n", hostHeader)
		if len(hosts) > 0 {
			fmt.Fprintf(w, "已配置: %s\n", strings.Join(hosts, ", "))
		} else {
			fmt.Fprint(w, "反向代理还没有配置任何站点。\n")
		}
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusNotFound)

	var list string
	if len(hosts) == 0 {
		list = `<p class="err">反向代理还没有配置任何站点。请到管理后台的「反向代理」页添加。</p>`
	} else {
		var b strings.Builder
		b.WriteString(`<p class="hint">已配置的域名：</p><ul>`)
		for _, h := range hosts {
			b.WriteString("<li><code>" + html.EscapeString(h) + "</code></li>")
		}
		b.WriteString("</ul>")
		list = b.String()
	}
	fmt.Fprintf(w, noSiteTmpl, html.EscapeString(hostHeader), list)
}

const pageStyle = `<style>
:root{color-scheme:light dark}
body{margin:0;min-height:100vh;display:flex;align-items:center;justify-content:center;
background:#0f1115;color:#e6e8ec;
font-family:system-ui,-apple-system,"Segoe UI","PingFang SC","Microsoft YaHei",sans-serif}
.card{max-width:34rem;padding:2.5rem 2rem;text-align:left}
h1{margin:0 0 .25rem;font-size:1.5rem;font-weight:600}
.code{color:#7d8590;font-size:.8rem;letter-spacing:.08em;margin:0 0 1.5rem}
p{margin:.5rem 0;line-height:1.7;color:#adb4bf}
code{background:#1c2029;padding:.15rem .4rem;border-radius:4px;
font-family:ui-monospace,SFMono-Regular,Consolas,monospace;font-size:.875rem;color:#e6e8ec}
.err{color:#e5786d}
.hint{font-size:.875rem}
ul{margin:.5rem 0;padding-left:1.25rem;line-height:1.9}
.foot{margin-top:1.75rem;font-size:.75rem;color:#5c636e}
</style>`

const originUnreachableTmpl = `<!DOCTYPE html><html lang="zh-CN"><head><meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1"><title>502 后端连不上</title>
` + pageStyle + `</head><body><div class="card">
<p class="code">502 BAD GATEWAY</p>
<h1>后端连不上</h1>
<p>LinkStar 收到了对 <code>%s</code> 的请求，但后端 <code>%s</code> 没有响应。</p>
%s
<p class="hint">通常是这台内网机器关机了、服务没启动，或者后端地址填错了。服务恢复后直接刷新本页即可。</p>
<p class="foot">LinkStar 反向代理</p>
</div></body></html>`

const noSiteTmpl = `<!DOCTYPE html><html lang="zh-CN"><head><meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1"><title>404 没有匹配的站点</title>
` + pageStyle + `</head><body><div class="card">
<p class="code">404 NOT FOUND</p>
<h1>没有匹配的站点</h1>
<p>没有任何一个站点认领 <code>%s</code>。</p>
%s
<p class="hint">检查一下域名是否拼错、这个站点是否被禁用，以及该域名的 DNS 是否解析到了本机。</p>
<p class="foot">LinkStar 反向代理</p>
</div></body></html>`
