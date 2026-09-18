package proxy

import (
	"crypto/tls"
	"net"
	"time"

	"linkstar/modules/proxy/model"
)

// ProbeResult 拨一次后端的结果。
//
// 存在的理由：「后端是 HTTPS」这个勾选框是整张表单里最容易填错的一项，
// 而填错之后用户看到的只有一页 502。转发时有自动纠正（schemeFixTransport），
// 但那是补救；让用户在保存前就看见真实情况，比事后纠正更省事。
type ProbeResult struct {
	Reachable bool   `json:"reachable"`
	Scheme    string `json:"scheme"`   // 实际探到的协议：http / https
	Mismatch  bool   `json:"mismatch"` // 和用户勾的不一致
	LatencyMS int64  `json:"latencyMs"`
	Message   string `json:"message"`
}

// ProbeBackend 拨一次后端：先看端口通不通，再看它说不说 TLS
func ProbeBackend(site model.Site) ProbeResult {
	target := parseBackend(site)
	addr := backendDialAddr(target.Host, target.Scheme)

	start := time.Now()
	conn, err := net.DialTimeout("tcp", addr, backendDialTimeout)
	latency := time.Since(start).Milliseconds()
	if err != nil {
		return ProbeResult{
			LatencyMS: latency,
			Message:   describeBackendError(err),
		}
	}
	_ = conn.Close()

	scheme := detectBackendScheme(addr)
	out := ProbeResult{
		Reachable: true,
		Scheme:    scheme,
		LatencyMS: latency,
		Mismatch:  scheme != target.Scheme,
		Message:   "端口通，探到的是 " + scheme,
	}
	if out.Mismatch {
		if scheme == "https" {
			out.Message = "端口通，但它说的是 TLS——请勾上「后端是 HTTPS」"
		} else {
			out.Message = "端口通，但它是明文的——请取消「后端是 HTTPS」勾选"
		}
	}
	return out
}

// detectBackendScheme 再握一次 TLS 手：握上了就是 HTTPS，握不上就当明文。
//
// 用新连接而不是复用上面那条——TLS 握手会把连接写脏，失败后没法再当明文用。
// 少数强制要求 SNI 的后端会拒掉这次不带 SNI 的握手而被判成 http；
// 这只是个给用户看的提示，判错了转发时的自动纠正还会兜一次。
func detectBackendScheme(addr string) string {
	dialer := &net.Dialer{Timeout: backendDialTimeout}
	conn, err := tls.DialWithDialer(dialer, "tcp", addr, &tls.Config{InsecureSkipVerify: true})
	if err != nil {
		return "http"
	}
	_ = conn.Close()
	return "https"
}

// backendDialAddr 补上省略的端口。用户写 "nas" 或 "https://nas" 都是合法的
func backendDialAddr(host, scheme string) string {
	if _, _, err := net.SplitHostPort(host); err == nil {
		return host
	}
	if scheme == "https" {
		return net.JoinHostPort(host, "443")
	}
	return net.JoinHostPort(host, "80")
}
