package plainhttp

import (
	"bufio"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"io"
	"math/big"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"
)

// dialAndRead 往连接上写一段原文，把对面回的东西全读回来
func dialAndRead(t *testing.T, addr, raw string) string {
	t.Helper()
	c, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatalf("拨号失败: %v", err)
	}
	defer c.Close()
	_ = c.SetDeadline(time.Now().Add(5 * time.Second))
	if _, err := io.WriteString(c, raw); err != nil {
		t.Fatalf("写失败: %v", err)
	}
	b, _ := io.ReadAll(c)
	return string(b)
}

// TestRedirectsPlainHTTP 明文 GET 打到 HTTPS 端口上，应该收到 302 而不是被掐断。
// 这是「地址栏敲域名:端口 → 浏览器按 http 发 → 页面打不开 + 不安全」那个问题的回归测试。
func TestRedirectsPlainHTTP(t *testing.T) {
	ln := newTLSListener(t)
	defer ln.Close()

	_, port, _ := net.SplitHostPort(ln.Addr().String())
	resp := dialAndRead(t, ln.Addr().String(),
		"GET /admin?a=1 HTTP/1.1\r\nHost: home.example.com:"+port+"\r\n\r\n")

	if !strings.HasPrefix(resp, "HTTP/1.1 302 ") {
		t.Fatalf("期望 302，实际收到:\n%s", resp)
	}
	want := "Location: https://home.example.com:" + port + "/admin?a=1\r\n"
	if !strings.Contains(resp, want) {
		t.Fatalf("Location 不对，期望含 %q，实际:\n%s", want, resp)
	}
}

// TestHostWithoutPortGetsListenPort Host 头没带端口时，拿本地监听端口补上，
// 不能傻跳到 443——那上面多半什么都没有。
func TestHostWithoutPortGetsListenPort(t *testing.T) {
	ln := newTLSListener(t)
	defer ln.Close()

	_, port, _ := net.SplitHostPort(ln.Addr().String())
	resp := dialAndRead(t, ln.Addr().String(), "GET / HTTP/1.1\r\nHost: home.example.com\r\n\r\n")

	want := "Location: https://home.example.com:" + port + "/\r\n"
	if !strings.Contains(resp, want) {
		t.Fatalf("期望含 %q，实际:\n%s", want, resp)
	}
}

// TestTLSStillWorks 偷看第一个字节不能把真正的 TLS 连接弄坏。
func TestTLSStillWorks(t *testing.T) {
	ln := newTLSListener(t)
	defer ln.Close()

	c, err := tls.Dial("tcp", ln.Addr().String(), &tls.Config{InsecureSkipVerify: true})
	if err != nil {
		t.Fatalf("TLS 握手失败: %v", err)
	}
	defer c.Close()
	_ = c.SetDeadline(time.Now().Add(5 * time.Second))

	if _, err := io.WriteString(c, "GET / HTTP/1.1\r\nHost: x\r\n\r\n"); err != nil {
		t.Fatalf("写失败: %v", err)
	}
	resp, err := http.ReadResponse(bufio.NewReader(c), nil)
	if err != nil {
		t.Fatalf("读响应失败: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if string(body) != "ok" {
		t.Fatalf("期望 body=ok，实际 %q", body)
	}
}

// TestNonHTTPGarbageGetsNothing 说的既不是 TLS 也不是 HTTP 的客户端，
// 什么都不该收到——给它喷一段 HTTP 只会把它的报错搞乱。
func TestNonHTTPGarbageGetsNothing(t *testing.T) {
	ln := newTLSListener(t)
	defer ln.Close()

	if resp := dialAndRead(t, ln.Addr().String(), "\x00\x01\x02not a protocol\r\n\r\n"); resp != "" {
		t.Fatalf("期望什么都不回，实际收到 %q", resp)
	}
}

// newTLSListener 起一个「明文兜底 + TLS + 固定回 ok」的监听，和 proxy 里的串法一致
func newTLSListener(t *testing.T) net.Listener {
	t.Helper()

	raw, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("监听失败: %v", err)
	}

	ln := tls.NewListener(Listener(raw, 2*time.Second), &tls.Config{
		Certificates: []tls.Certificate{selfSigned(t)},
	})

	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "ok")
	})}
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() { _ = srv.Close() })

	return ln
}

// selfSigned 测试用的临时证书，不走 cert 包——这个包本身不该依赖它
func selfSigned(t *testing.T) tls.Certificate {
	t.Helper()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("生成私钥失败: %v", err)
	}
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "localhost"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:              []string{"localhost"},
		IPAddresses:           []net.IP{net.ParseIP("127.0.0.1")},
		IsCA:                  true,
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("签发证书失败: %v", err)
	}
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}
}
