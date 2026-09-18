package stun

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"errors"
	"io"
	"math/big"
	"net"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"linkstar/modules/cert"
	"linkstar/modules/cert/model"
)

// testCert 现造一张自签证书。用 P-256 而不是 RSA，生成快得多。
func testCert(t *testing.T, dnsName string) tls.Certificate {
	t.Helper()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("生成密钥失败: %v", err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: dnsName},
		DNSNames:     []string{dnsName},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IsCA:         true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("签发证书失败: %v", err)
	}
	leaf, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatalf("解析证书失败: %v", err)
	}
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key, Leaf: leaf}
}

// backend 一个把收到的字节原样回吐的内网服务。
// accepts 用来断言「握手失败时根本不该拨后端」。
type backend struct {
	addr    string
	accepts *atomic.Int32
}

func newBackend(t *testing.T, cert *tls.Certificate) *backend {
	t.Helper()

	var ln net.Listener
	var err error
	if cert != nil {
		ln, err = tls.Listen("tcp", "127.0.0.1:0", &tls.Config{Certificates: []tls.Certificate{*cert}})
	} else {
		ln, err = net.Listen("tcp", "127.0.0.1:0")
	}
	if err != nil {
		t.Fatalf("启动后端失败: %v", err)
	}
	t.Cleanup(func() { ln.Close() })

	b := &backend{addr: ln.Addr().String(), accepts: &atomic.Int32{}}
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			b.accepts.Add(1)
			go func() {
				defer c.Close()
				_, _ = io.Copy(c, c)
			}()
		}
	}()
	return b
}

// newHole 模拟一个 STUN 洞：LinkStar 自己 Accept，把裸连接交给 ForwardTCP。
func newHole(t *testing.T, target string, opt ForwardOptions) string {
	t.Helper()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("启动洞口失败: %v", err)
	}
	t.Cleanup(func() { ln.Close() })

	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go ForwardTCP(c, target, "tcp", opt)
		}
	}()
	return ln.Addr().String()
}

func roundTrip(t *testing.T, c net.Conn, msg string) string {
	t.Helper()

	if err := c.SetDeadline(time.Now().Add(5 * time.Second)); err != nil {
		t.Fatalf("设置超时失败: %v", err)
	}
	if _, err := io.WriteString(c, msg); err != nil {
		t.Fatalf("写入失败: %v", err)
	}
	buf := make([]byte, len(msg))
	if _, err := io.ReadFull(c, buf); err != nil {
		t.Fatalf("读取失败: %v", err)
	}
	return string(buf)
}

// TestForwardTCPPlain 不开 TLS 时行为和改动前一致
func TestForwardTCPPlain(t *testing.T) {
	be := newBackend(t, nil)
	hole := newHole(t, be.addr, ForwardOptions{})

	c, err := net.Dial("tcp", hole)
	if err != nil {
		t.Fatalf("连接洞口失败: %v", err)
	}
	defer c.Close()

	if got := roundTrip(t, c, "hello"); got != "hello" {
		t.Errorf("回包 = %q, 想要 %q", got, "hello")
	}
}

// TestForwardTCPTerminatesTLS 洞口终结 TLS：外面是 https，后端拿到的是明文。
// 同时验证「不解析 HTTP」这个前提——转发的就是裸字节流。
func TestForwardTCPTerminatesTLS(t *testing.T) {
	cert := testCert(t, "fw.example.com")
	be := newBackend(t, nil)

	pool := x509.NewCertPool()
	pool.AddCert(cert.Leaf)

	hole := newHole(t, be.addr, ForwardOptions{
		TLSConfig: &tls.Config{Certificates: []tls.Certificate{cert}},
	})

	c, err := tls.Dial("tcp", hole, &tls.Config{RootCAs: pool, ServerName: "fw.example.com"})
	if err != nil {
		t.Fatalf("TLS 握手失败: %v", err)
	}
	defer c.Close()

	// 非 HTTP 的任意字节流也该原样穿过去
	payload := "\x00\x01not-http\xff"
	if got := roundTrip(t, c, payload); got != payload {
		t.Errorf("回包 = %q, 想要 %q", got, payload)
	}
	if n := be.accepts.Load(); n != 1 {
		t.Errorf("后端应被拨号 1 次，实际 %d", n)
	}
}

// TestForwardTCPUsesGetCertificate 每次握手实时回调取证书——
// 续期只换指针、洞不用重启，全靠这条性质
func TestForwardTCPUsesGetCertificate(t *testing.T) {
	first := testCert(t, "old.example.com")
	second := testCert(t, "new.example.com")

	var active atomic.Pointer[tls.Certificate]
	active.Store(&first)

	be := newBackend(t, nil)
	hole := newHole(t, be.addr, ForwardOptions{
		TLSConfig: &tls.Config{
			GetCertificate: func(*tls.ClientHelloInfo) (*tls.Certificate, error) {
				return active.Load(), nil
			},
		},
	})

	dialCN := func(t *testing.T) string {
		t.Helper()
		// 这里只关心服务端发了哪张证书，不做校验
		c, err := tls.Dial("tcp", hole, &tls.Config{InsecureSkipVerify: true})
		if err != nil {
			t.Fatalf("握手失败: %v", err)
		}
		defer c.Close()
		// 走一次收发再返回：否则转发协程可能还没拨到后端，
		// 测试就结束并把后端监听关掉了，留下一条无关的报错日志
		roundTrip(t, c, "ping")
		return c.ConnectionState().PeerCertificates[0].Subject.CommonName
	}

	if got := dialCN(t); got != "old.example.com" {
		t.Fatalf("续期前应拿到旧证书，实际 %q", got)
	}

	// 洞一直开着，只换指针
	active.Store(&second)

	if got := dialCN(t); got != "new.example.com" {
		t.Errorf("换指针后新连接应拿到新证书，实际 %q", got)
	}
}

// TestForwardTCPHandshakeFailureSkipsBackend 握手失败就不该白拨一次内网
func TestForwardTCPHandshakeFailureSkipsBackend(t *testing.T) {
	cert := testCert(t, "fw.example.com")
	be := newBackend(t, nil)
	hole := newHole(t, be.addr, ForwardOptions{
		TLSConfig: &tls.Config{Certificates: []tls.Certificate{cert}},
	})

	c, err := net.Dial("tcp", hole)
	if err != nil {
		t.Fatalf("连接洞口失败: %v", err)
	}
	defer c.Close()

	// 不是 ClientHello，握手必然失败
	if _, err := io.WriteString(c, "GET / HTTP/1.1\r\nHost: x\r\n\r\n"); err != nil {
		t.Fatalf("写入失败: %v", err)
	}
	if err := c.SetReadDeadline(time.Now().Add(5 * time.Second)); err != nil {
		t.Fatalf("设置超时失败: %v", err)
	}
	// 服务端会回 TLS alert 然后关闭；这里只要等连接结束
	_, _ = io.Copy(io.Discard, c)

	if n := be.accepts.Load(); n != 0 {
		t.Errorf("握手失败不该拨后端，实际拨了 %d 次", n)
	}
}

// TestForwardTCPHandshakeDeadline 连上来不说话的连接必须被超时踢掉，
// 否则端口扫描器开一堆空连接就能把 goroutine 钉死
func TestForwardTCPHandshakeDeadline(t *testing.T) {
	old := tlsHandshakeTimeout
	tlsHandshakeTimeout = 150 * time.Millisecond
	t.Cleanup(func() { tlsHandshakeTimeout = old })

	cert := testCert(t, "fw.example.com")
	be := newBackend(t, nil)
	hole := newHole(t, be.addr, ForwardOptions{
		TLSConfig: &tls.Config{Certificates: []tls.Certificate{cert}},
	})

	c, err := net.Dial("tcp", hole)
	if err != nil {
		t.Fatalf("连接洞口失败: %v", err)
	}
	defer c.Close()

	// 一个字节都不发
	if err := c.SetReadDeadline(time.Now().Add(3 * time.Second)); err != nil {
		t.Fatalf("设置超时失败: %v", err)
	}
	if _, err := c.Read(make([]byte, 1)); !errors.Is(err, io.EOF) {
		t.Errorf("超时后洞口应关闭连接（EOF），实际 %v", err)
	}
	if n := be.accepts.Load(); n != 0 {
		t.Errorf("握手超时不该拨后端，实际拨了 %d 次", n)
	}
}

// TestForwardTCPBackendHTTPS 内网侧本身是 HTTPS（通常自签，不校验）
func TestForwardTCPBackendHTTPS(t *testing.T) {
	backendCert := testCert(t, "selfsigned.internal")
	be := newBackend(t, &backendCert)
	hole := newHole(t, be.addr, ForwardOptions{BackendHTTPS: true})

	c, err := net.Dial("tcp", hole)
	if err != nil {
		t.Fatalf("连接洞口失败: %v", err)
	}
	defer c.Close()

	if got := roundTrip(t, c, "hello"); got != "hello" {
		t.Errorf("回包 = %q, 想要 %q", got, "hello")
	}
}

// newHTTPBackend 一个只会说 HTTP 的后端：一连上就回一行响应。
// cert 非 nil 时它自己就是 HTTPS 服务。
//
// 不能复用 newBackend 的回显后端——回显会把 ClientHello 原样退回来，
// Go 报的是「该收 ServerHello 却收到 ClientHello」；真实的服务回的是
// "HTTP/1.1 ..."，首字节 'H' 不是合法的 TLS record type，这才是
// tls.RecordHeaderError。用错后端就测不到用户真正踩的那条路径。
func newHTTPBackend(t *testing.T, cert *tls.Certificate) string {
	t.Helper()

	var ln net.Listener
	var err error
	if cert != nil {
		ln, err = tls.Listen("tcp", "127.0.0.1:0", &tls.Config{Certificates: []tls.Certificate{*cert}})
	} else {
		ln, err = net.Listen("tcp", "127.0.0.1:0")
	}
	if err != nil {
		t.Fatalf("监听后端失败: %v", err)
	}
	t.Cleanup(func() { ln.Close() })

	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				defer c.Close()
				// 先把请求读掉再回。带着没读完的接收缓冲直接 Close，
				// Windows 会发 RST 而不是 FIN，客户端拿到的是
				// "connection was aborted" 而不是那句 HTTP 响应——
				// 整包一起跑、机器忙的时候就会随机挂在这里。
				_ = c.SetReadDeadline(time.Now().Add(2 * time.Second))
				_, _ = c.Read(make([]byte, 4096))
				_ = c.SetReadDeadline(time.Time{})
				_, _ = c.Write([]byte("HTTP/1.1 400 Bad Request\r\nContent-Length: 0\r\n\r\n"))
			}()
		}
	}()
	return ln.Addr().String()
}

// TestDescribeDialErrorOnPlaintextBackend 勾了「转发给内网时也用 HTTPS」但服务其实是明文。
//
// 这是真出过的事故：Https 字段本来只管链接怎么展示，一度被拿去决定怎么拨号，
// 结果所有勾过它的老配置升级后全部转发失败，日志里只有 Go 那句
// "tls: first record does not look like a TLS handshake"——没提该去改哪个开关。
func TestDescribeDialErrorOnPlaintextBackend(t *testing.T) {
	addr := newHTTPBackend(t, nil)

	// 洞口拨这个后端必然失败，走的就是 forward.go 里那条错误分支
	hole := newHole(t, addr, ForwardOptions{BackendHTTPS: true})
	c, err := net.Dial("tcp", hole)
	if err != nil {
		t.Fatalf("连接洞口失败: %v", err)
	}
	defer c.Close()
	if _, err := io.ReadAll(c); err != nil && !errors.Is(err, io.EOF) {
		t.Errorf("拨后端失败后洞口应直接关连接，实际 %v", err)
	}

	// 真正要断言的是日志那句话能不能照着改配置
	_, dialErr := tls.DialWithDialer(
		&net.Dialer{Timeout: 3 * time.Second},
		"tcp", addr,
		&tls.Config{InsecureSkipVerify: true},
	)
	if dialErr == nil {
		t.Fatal("明文后端不该能完成 TLS 握手")
	}
	var rec tls.RecordHeaderError
	if !errors.As(dialErr, &rec) {
		t.Fatalf("想要 tls.RecordHeaderError，实际 %T: %v", dialErr, dialErr)
	}
	msg := describeDialError(dialErr)
	if !strings.Contains(msg, "转发给内网时也用 HTTPS") {
		t.Errorf("错误提示没指出该关哪个开关: %s", msg)
	}
	if !strings.Contains(msg, dialErr.Error()) {
		t.Errorf("错误提示丢了原始错误，排查时会缺线索: %s", msg)
	}
}

// TestForwardTCPNegotiatesHTTP11 浏览器优先要 h2，洞口必须把它谈回 http/1.1。
//
// 端到端复现那次事故：证书装好了、握手也成功了，但 ALPN 报了 h2，
// 浏览器就发 HTTP/2 前言，内网明文服务回 "HTTP/1.1 ..." 文本，
// 结果是 ERR_HTTP2_PROTOCOL_ERROR——比没证书还糟。
//
// 这里连的是真的 cert.Manager 造出来的配置，不是测试里现编的，
// 免得改了 manager.go 而测试还在自说自话。
func TestForwardTCPNegotiatesHTTP11(t *testing.T) {
	holeCert := testCert(t, "fw.example.com")

	mgr := cert.NewManager()
	mgr.Store(1, &holeCert, model.Certificate{ID: 1, Enabled: true, IsDefault: true},
		"", "", time.Time{}, time.Time{})

	// 内网是个只会 HTTP/1.1 的明文服务
	backend := newHTTPBackend(t, nil)
	hole := newHole(t, backend, ForwardOptions{TLSConfig: mgr.ServerTLSConfig(1)})

	// 按浏览器的样子来：h2 排在前面
	c, err := tls.Dial("tcp", hole, &tls.Config{
		InsecureSkipVerify: true,
		ServerName:         "fw.example.com",
		NextProtos:         []string{"h2", "http/1.1"},
	})
	if err != nil {
		t.Fatalf("握手失败: %v", err)
	}
	defer c.Close()

	if got := c.ConnectionState().NegotiatedProtocol; got != "http/1.1" {
		t.Fatalf("协商到 %q——洞口不解析协议，报 h2 等于替内网撒谎", got)
	}

	// 谈成 1.1 之后，字节管道要真的能跑一个来回
	if _, err := c.Write([]byte("GET / HTTP/1.1\r\nHost: fw.example.com\r\n\r\n")); err != nil {
		t.Fatalf("发请求失败: %v", err)
	}
	buf := make([]byte, 64)
	n, err := c.Read(buf)
	if err != nil {
		t.Fatalf("读响应失败: %v", err)
	}
	if !strings.HasPrefix(string(buf[:n]), "HTTP/1.1 ") {
		t.Errorf("响应 = %q, 想要 HTTP/1.1 开头", buf[:n])
	}
}

// TestForwardTCPBackendHTTPSWithoutTerminateBreaksHandshake
// 内网服务已经是 HTTPS（比如反向代理那 6660），洞口又勾了「转发给内网时也用 HTTPS」
// 但没勾「洞口终结 TLS」——这是浏览器报 ERR_SSL_PROTOCOL_ERROR 的那种配法。
//
// 洞口不终结，浏览器的 ClientHello 对 LinkStar 来说只是一串字节；
// LinkStar 却先自己和内网握了一次手，把这串字节当明文塞进自己那条 TLS 里。
// 内网服务读到的是「一个 ClientHello 长相的 HTTP 请求」，回一句 HTTP/1.1 400，
// 这句话原样传回浏览器——首字节 'H' 不是合法的 TLS record type，
// 浏览器等不到 ServerHello，连接就废了。
func TestForwardTCPBackendHTTPSWithoutTerminateBreaksHandshake(t *testing.T) {
	backendCert := testCert(t, "inner.example.com")
	backend := newHTTPBackend(t, &backendCert)

	hole := newHole(t, backend, ForwardOptions{BackendHTTPS: true}) // 没有 TLSConfig = 没勾终结

	_, err := tls.Dial("tcp", hole, &tls.Config{InsecureSkipVerify: true})
	if err == nil {
		t.Fatal("双层 TLS 竟然握手成功了——那这个配法就不该再警告用户")
	}
	// 断言错误类型，而不只是「有错」：Chrome 把这一类翻译成 ERR_SSL_PROTOCOL_ERROR
	var rec tls.RecordHeaderError
	if !errors.As(err, &rec) {
		t.Errorf("想要 tls.RecordHeaderError（浏览器侧的 ERR_SSL_PROTOCOL_ERROR），实际 %T: %v", err, err)
	}
}

// TestForwardTCPPassthroughToHTTPSBackend 同一个内网 HTTPS 服务，两个勾都不勾。
//
// 和上一条配对：洞口当纯字节管道，浏览器的 TLS 直达内网服务，
// 拿到的是内网服务自己那张证书。少了这条，上面那个「握手失败」的断言
// 就可能是因为后端压根起不来而恒成立。
func TestForwardTCPPassthroughToHTTPSBackend(t *testing.T) {
	backendCert := testCert(t, "inner.example.com")
	backend := newHTTPBackend(t, &backendCert)

	hole := newHole(t, backend, ForwardOptions{}) // 两个勾都不勾

	c, err := tls.Dial("tcp", hole, &tls.Config{InsecureSkipVerify: true})
	if err != nil {
		t.Fatalf("纯管道转发下握手应该成功: %v", err)
	}
	defer c.Close()

	// 证书是内网服务发的，说明 LinkStar 确实一个字节都没动
	if got := c.ConnectionState().PeerCertificates[0].Subject.CommonName; got != "inner.example.com" {
		t.Errorf("拿到的证书 CN = %q，想要内网服务自己那张", got)
	}

	if _, err := c.Write([]byte("GET / HTTP/1.1\r\nHost: inner.example.com\r\n\r\n")); err != nil {
		t.Fatalf("发请求失败: %v", err)
	}
	buf := make([]byte, 64)
	n, err := c.Read(buf)
	if err != nil {
		t.Fatalf("读响应失败: %v", err)
	}
	if !strings.HasPrefix(string(buf[:n]), "HTTP/1.1 ") {
		t.Errorf("响应 = %q, 想要 HTTP/1.1 开头", buf[:n])
	}
}
