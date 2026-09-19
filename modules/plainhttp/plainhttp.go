// Package plainhttp 处理一种情况：明文 HTTP 的请求打到了 HTTPS 端口上。
//
// 这件事比看上去常见得多。浏览器地址栏里敲 example.com:25888（不带 https://），
// Chrome 会先按 http 试；书签、别人发来的链接、curl 忘了写 s，都会这样。
//
// 默认行为是 tls.Server 握手失败、连接被一声不吭地掐断，浏览器只显示
// ERR_EMPTY_RESPONSE。用户看到的是一个错误页 + 地址栏「不安全」，
// 而服务端其实证书好好的——这个误会没法靠看日志解开。
//
// nginx 遇到这种连接回 497，Caddy 回一句 "Client sent an HTTP request to an
// HTTPS server."。这里更进一步：直接 302 到同地址的 https，用户什么都不用做。
package plainhttp

import (
	"bufio"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// tlsRecordHandshake TLS 记录层握手类型。任何 TLS 版本的 ClientHello
// 第一个字节都是 0x16；HTTP 请求的第一个字节一定是方法名的大写字母。
// 判定只看这一个字节就够了。
const tlsRecordHandshake = 0x16

// DefaultTimeout 偷看第一个字节、以及读完请求头的超时。
// 必须有：连上来不说话的连接（端口扫描）否则会一直钉着一个 goroutine。
const DefaultTimeout = 10 * time.Second

// peekedConn 把偷看掉的字节补回去。tls.Server 从它 Read，读到的还是完整的 ClientHello。
type peekedConn struct {
	net.Conn
	r *bufio.Reader
}

func (c *peekedConn) Read(p []byte) (int, error) { return c.r.Read(p) }

// Sniff 偷看连接的第一个字节，判断对面说的是 TLS 还是明文。
//
// 返回的 net.Conn 可以直接交给 tls.Server / tls.NewListener——偷看的字节已经补回去了。
// 出错时返回原连接，调用方负责关。
func Sniff(c net.Conn, timeout time.Duration) (net.Conn, bool, error) {
	if timeout > 0 {
		if err := c.SetReadDeadline(time.Now().Add(timeout)); err != nil {
			return c, false, err
		}
	}
	br := bufio.NewReader(c)
	head, err := br.Peek(1)
	if timeout > 0 {
		// 无论成败都得把 deadline 撤掉，否则后面的 TLS 握手会莫名其妙超时
		if e := c.SetReadDeadline(time.Time{}); e != nil && err == nil {
			err = e
		}
	}
	if err != nil {
		return c, false, err
	}
	return &peekedConn{Conn: c, r: br}, head[0] == tlsRecordHandshake, nil
}

// Redirect 从一条明文连接上读出请求，回 302 指向同地址的 https，然后关掉连接。
//
// 读不出 HTTP 请求就什么都不发——这个端口上跑的不一定是网页，
// 对一个说着别的协议的客户端喷一段 HTTP 只会把它的报错搞得更难懂。
func Redirect(c net.Conn, timeout time.Duration) {
	defer c.Close()

	if timeout > 0 {
		if err := c.SetDeadline(time.Now().Add(timeout)); err != nil {
			return
		}
	}

	req, err := http.ReadRequest(bufio.NewReader(c))
	if err != nil {
		return
	}

	target := "https://" + redirectHost(req.Host, c.LocalAddr()) + requestPath(req)
	body := fmt.Sprintf(
		"<!DOCTYPE html><html lang=\"zh-CN\"><head><meta charset=\"utf-8\">"+
			"<title>302 请改用 HTTPS</title></head><body>"+
			"<p>这个端口只收 HTTPS。正在跳转到 <a href=\"%s\">%s</a>。</p>"+
			"</body></html>\n",
		htmlAttr(target), htmlText(target),
	)

	fmt.Fprintf(c, "HTTP/1.1 302 Found\r\n"+
		"Location: %s\r\n"+
		"Content-Type: text/html; charset=utf-8\r\n"+
		"Content-Length: %d\r\n"+
		"Cache-Control: no-store\r\n"+
		"Connection: close\r\n"+
		"\r\n%s", target, len(body), body)
}

// requestPath 原样带上路径和查询串。RequestURI 是客户端发来的原文，
// 不能用 req.URL.String() —— 那会把编码重新规范化一遍。
func requestPath(req *http.Request) string {
	uri := req.RequestURI
	if uri == "" || uri == "*" {
		return "/"
	}
	// 绝对形式的请求行（代理写法）：http://host/path
	if i := strings.Index(uri, "://"); i >= 0 {
		if j := strings.IndexByte(uri[i+3:], '/'); j >= 0 {
			return uri[i+3+j:]
		}
		return "/"
	}
	if !strings.HasPrefix(uri, "/") {
		return "/"
	}
	return uri
}

// redirectHost 决定跳到哪个域名和端口。
//
// 首选 Host 头：它带着客户端真正敲进地址栏的端口，这正是要跳回去的地方。
// Host 头没写端口（HTTP 默认 80）时才拿本地监听端口补——这在 STUN 打洞下
// 只是个近似值（公网端口未必等于本机监听端口），但比跳到 443 强。
func redirectHost(host string, local net.Addr) string {
	host = strings.TrimSpace(host)
	if host == "" {
		if local != nil {
			host = local.String()
		}
		if host == "" {
			return "localhost"
		}
	}
	if hasPort(host) {
		return host
	}
	port := ""
	if local != nil {
		if _, p, err := net.SplitHostPort(local.String()); err == nil {
			port = p
		}
	}
	if port == "" || port == "443" {
		return host
	}
	return net.JoinHostPort(host, port)
}

// hasPort 认得 IPv6 字面量：最后一个冒号得在右方括号之后才算端口。
func hasPort(host string) bool {
	return strings.LastIndexByte(host, ':') > strings.LastIndexByte(host, ']')
}

var (
	attrEsc = strings.NewReplacer(`&`, "&amp;", `"`, "&quot;", `<`, "&lt;", `>`, "&gt;")
	textEsc = strings.NewReplacer(`&`, "&amp;", `<`, "&lt;", `>`, "&gt;")
)

func htmlAttr(s string) string { return attrEsc.Replace(s) }
func htmlText(s string) string { return textEsc.Replace(s) }

// Listener 包在 tls.NewListener 外面（注意是外面，先它一步拿到裸连接）：
// 明文 HTTP 连接就地回 302 并吃掉，只有真正的 TLS 连接才会从 Accept 出来。
//
//	ln = plainhttp.Listener(ln, plainhttp.DefaultTimeout)
//	ln = tls.NewListener(ln, tlsConfig)
//
// 偷看放在各自的 goroutine 里，不能在 Accept 里同步做——一条连上来不说话的
// 连接会把整个 accept 循环卡住 timeout 那么久。
func Listener(ln net.Listener, timeout time.Duration) net.Listener {
	l := &listener{
		Listener: ln,
		timeout:  timeout,
		conns:    make(chan net.Conn),
		done:     make(chan struct{}),
	}
	go l.loop()
	return l
}

type listener struct {
	net.Listener
	timeout time.Duration
	conns   chan net.Conn
	done    chan struct{}
	once    sync.Once

	mu  sync.Mutex
	err error
}

func (l *listener) loop() {
	for {
		c, err := l.Listener.Accept()
		if err != nil {
			l.mu.Lock()
			if l.err == nil {
				l.err = err
			}
			l.mu.Unlock()
			l.once.Do(func() { close(l.done) })
			return
		}
		go l.handle(c)
	}
}

func (l *listener) handle(c net.Conn) {
	conn, isTLS, err := Sniff(c, l.timeout)
	if err != nil {
		_ = c.Close()
		return
	}
	if !isTLS {
		Redirect(conn, l.timeout)
		return
	}
	select {
	case l.conns <- conn:
	case <-l.done:
		_ = conn.Close()
	}
}

func (l *listener) Accept() (net.Conn, error) {
	select {
	case c := <-l.conns:
		return c, nil
	case <-l.done:
		l.mu.Lock()
		err := l.err
		l.mu.Unlock()
		if err == nil {
			err = net.ErrClosed
		}
		return nil, err
	}
}

func (l *listener) Close() error {
	l.once.Do(func() { close(l.done) })
	return l.Listener.Close()
}
