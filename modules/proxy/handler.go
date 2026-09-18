package proxy

import (
	"context"
	"crypto/tls"
	"errors"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"linkstar/modules/proxy/model"

	"github.com/sirupsen/logrus"
)

// backendDialTimeout 拨内网后端的超时。和 forward.go 的字节管道保持一致：
// 内网一跳，3 秒还没握上手就是机器不在。
const backendDialTimeout = 3 * time.Second

// newHandler 一个监听端口的入口。
//
// 每个端口有自己的一张转发表——站点可以自己占端口，8443 上的站点不该
// 出现在 443 的表里。currentRouter 每个请求现取，改站点立刻生效，不用重开监听。
func newHandler(currentRouter func() *Router) http.Handler {
	return withAccessLog(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		serve(currentRouter(), w, r)
	}))
}

func serve(router *Router, w http.ResponseWriter, r *http.Request) {
	if router == nil {
		writeNoSite(w, r, r.Host, nil)
		return
	}

	entry := router.Match(r.Host, r.URL.Path)
	if entry == nil {
		writeNoSite(w, r, r.Host, router.Hosts())
		return
	}

	entry.proxy.ServeHTTP(w, r)
}

// newReverseProxy 把一个站点编译成转发器。建表时调用一次，请求路径上只用不建。
func newReverseProxy(site model.Site) *httputil.ReverseProxy {
	target := parseBackend(site)
	transport := newBackendTransport(site)

	return &httputil.ReverseProxy{
		Rewrite: func(r *httputil.ProxyRequest) {
			r.SetURL(target)
			r.SetXForwarded() // X-Forwarded-For / -Host / -Proto

			// ★ SetURL 会把 Out.Host 覆盖成后端的 host，必须写回原始 Host。
			// 这就是 nginx 里 $http_host 和 $host 的区别：Jellyfin、Home Assistant
			// 这类服务靠 Host 生成回跳 URL，不写回的话用户登录完会被甩到内网 IP 上。
			r.Out.Host = r.In.Host

			// stdlib 只设 For/Host/Proto，这两个是 forward-auth 类中间件的惯例，
			// 补上不花钱（借鉴 GoDoxy）
			r.Out.Header.Set("X-Forwarded-Method", r.In.Method)
			r.Out.Header.Set("X-Forwarded-Uri", r.In.URL.RequestURI())

			if site.StripPrefix && site.PathPrefix != "" {
				stripPrefix(r.Out.URL, site.PathPrefix)
				// 告诉后端它被挂在哪个子路径下。支持 base path 的服务
				// （Jellyfin / qBittorrent）认这个头，生成资源链接时会带上前缀。
				r.Out.Header.Set("X-Forwarded-Prefix", site.PathPrefix)
			}
		},

		// -1 = 收到就冲刷。SSE、日志流、AI 的流式输出全靠它，
		// 等价于 nginx 的 proxy_buffering off。
		FlushInterval: -1,

		Transport: transport,

		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			// 客户端自己关掉页面导致的中断不是故障，不该刷日志也不该写 502
			if errors.Is(err, context.Canceled) {
				return
			}
			logrus.Warnf("[proxy] %s%s → %s 转发失败：%s",
				site.Label(), r.URL.Path, site.Backend, describeBackendError(err))
			writeOriginUnreachable(w, r, site.Label(), site.Backend, err)
		},
	}
}

// parseBackend 把用户填的后端地址解析成目标 URL。
//
// 宽容一点：既接受 "192.168.1.20:8080"，也接受 "https://192.168.1.20:8443/app"。
// 带了 scheme 就以填的为准——用户明确写出来的比一个勾选框更可信。
func parseBackend(site model.Site) *url.URL {
	backend := strings.TrimSpace(site.Backend)

	if strings.Contains(backend, "://") {
		if u, err := url.Parse(backend); err == nil && u.Host != "" {
			u.Path = strings.TrimSuffix(u.Path, "/")
			return u
		}
	}

	scheme := "http"
	if site.BackendHTTPS {
		scheme = "https"
	}
	return &url.URL{Scheme: scheme, Host: backend}
}

// stripPrefix 剥掉路径前缀，转义前后两份路径一起改。
// 判断条件抄 stdlib 的 http.StripPrefix：两份对不上就干脆不改，
// 宁可把带前缀的路径原样发给后端，也不要发一个自相矛盾的 URL。
func stripPrefix(u *url.URL, prefix string) {
	p := strings.TrimPrefix(u.Path, prefix)
	rp := strings.TrimPrefix(u.RawPath, prefix)
	if len(p) >= len(u.Path) || (u.RawPath != "" && len(rp) >= len(u.RawPath)) {
		return
	}
	if p == "" {
		p = "/"
	}
	u.Path = p
	if u.RawPath != "" {
		if rp == "" {
			rp = "/"
		}
		u.RawPath = rp
	}
}

// newBackendTransport 每个站点一个 Transport，连接池互不干扰
func newBackendTransport(site model.Site) http.RoundTripper {
	base := &http.Transport{
		// 明确不走环境变量里的代理：后端是内网地址，
		// 跟着 HTTP_PROXY 跑出去纯属灾难
		Proxy: nil,

		DialContext: (&net.Dialer{
			Timeout:   backendDialTimeout,
			KeepAlive: 30 * time.Second,
		}).DialContext,

		// 内网服务基本都是自签，本来就无从校验（与 forward.go 的行为一致）
		TLSClientConfig:     &tls.Config{InsecureSkipVerify: true},
		TLSHandshakeTimeout: backendDialTimeout,
		ForceAttemptHTTP2:   true,

		// 默认 2 太小：一个浏览器页面就能开满 6 条，
		// 剩下的每次都要重新握手
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 32,
		IdleConnTimeout:     90 * time.Second,

		ExpectContinueTimeout: 1 * time.Second,

		// 绝不设 ResponseHeaderTimeout：SSE / 长轮询 / WebSocket
		// 的第一个响应头本来就可能隔很久才来
	}

	return &schemeFixTransport{base: base, site: site}
}

// schemeFixTransport 后端 scheme 勾错时自动纠正一次。
//
// 「内网服务本身是 HTTPS」这个勾选框是最容易填错的一项，而错了之后
// Go 抛出的原始错误（tls: first record does not look like a TLS handshake）
// 完全没提该去动哪个开关。与其只报错，不如换个 scheme 再试一次——
// 成功就记住，后续请求直接走对的那条路。（思路借鉴 GoDoxy 的 OnSchemeMisMatch）
type schemeFixTransport struct {
	base *http.Transport
	site model.Site

	corrected atomic.Pointer[string] // 纠正后的 scheme，nil = 还没纠正过
	warnOnce  sync.Once
}

func (t *schemeFixTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if fixed := t.corrected.Load(); fixed != nil && *fixed != req.URL.Scheme {
		req = cloneWithScheme(req, *fixed)
	}

	resp, err := t.base.RoundTrip(req)
	if err == nil || !isSchemeMismatch(err) {
		return resp, err
	}

	retry, ok := t.retryRequest(req)
	if !ok {
		return resp, err
	}

	resp2, err2 := t.base.RoundTrip(retry)
	if err2 != nil {
		// 换了 scheme 还是不行，说明问题不在这儿。
		// 返回原始错误——它离真相更近。
		return resp, err
	}

	scheme := retry.URL.Scheme
	t.corrected.Store(&scheme)
	t.warnOnce.Do(func() {
		logrus.Warnf("[proxy] 站点 %s 的后端 %s 实际是 %s，已自动纠正并继续转发；"+
			"请到反向代理页把「后端是 HTTPS」改对，以免每次请求都多拨一遍",
			t.site.Label(), t.site.Backend, retry.URL.Scheme)
	})
	return resp2, nil
}

// retryRequest 准备换 scheme 的重试请求；body 无法重放时返回 false。
//
// ★ 必须检查 GetBody：body 已经被第一次尝试读掉了，没有它就重放不了。
// 这时候发一个空 body 的 POST 出去，比干脆失败糟糕得多。
func (t *schemeFixTransport) retryRequest(req *http.Request) (*http.Request, bool) {
	alt := "https"
	if req.URL.Scheme == "https" {
		alt = "http"
	}

	retry := cloneWithScheme(req, alt)

	if req.Body != nil && req.Body != http.NoBody {
		if req.GetBody == nil {
			return nil, false
		}
		body, err := req.GetBody()
		if err != nil {
			return nil, false
		}
		retry.Body = body
	}
	return retry, true
}

func cloneWithScheme(req *http.Request, scheme string) *http.Request {
	out := req.Clone(req.Context()) // Clone 会深拷贝 URL，改它不影响原请求
	out.URL.Scheme = scheme
	return out
}

// isSchemeMismatch 这个错误是不是「http/https 弄反了」
func isSchemeMismatch(err error) bool {
	if errors.Is(err, http.ErrSchemeMismatch) {
		return true
	}
	// 按 HTTPS 去连一个明文服务：TLS 握手读到的是 HTTP 响应文本
	var rec tls.RecordHeaderError
	if errors.As(err, &rec) {
		return true
	}
	// 反过来，按明文去连一个 TLS 服务：对端回的是 TLS alert 记录（0x15 开头），
	// Go 只会抱怨响应不是合法 HTTP。除了认这串字节没有别的办法。
	msg := err.Error()
	return strings.Contains(msg, "malformed HTTP response") &&
		(strings.Contains(msg, `\x15\x03`) || strings.Contains(msg, `\x16\x03`))
}

// describeBackendError 把转发失败翻译成能照着改配置的话
func describeBackendError(err error) string {
	if err == nil {
		return ""
	}

	var rec tls.RecordHeaderError
	if errors.As(err, &rec) || errors.Is(err, http.ErrSchemeMismatch) {
		return "后端不是 HTTPS，却按 HTTPS 去连了——请取消该站点的「后端是 HTTPS」勾选"
	}
	if isSchemeMismatch(err) {
		return "后端是 HTTPS，却按明文去连了——请勾上该站点的「后端是 HTTPS」"
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "连接内网服务超时（原始错误：" + err.Error() + "）"
	}

	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return "连接内网服务超时（原始错误：" + err.Error() + "）"
	}

	msg := err.Error()
	switch {
	case strings.Contains(msg, "connection refused"):
		return "后端拒绝连接——端口对不上，或者服务没在跑"
	case strings.Contains(msg, "no such host"):
		return "解析不了后端主机名——请改用 IP，或确认本机 DNS 能解析它"
	}
	return msg
}
