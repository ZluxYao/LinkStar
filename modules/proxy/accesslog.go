package proxy

import (
	"bufio"
	"net"
	"net/http"
	"time"

	"github.com/sirupsen/logrus"
)

// withAccessLog 访问日志包装。
//
// 默认关着：家里就这么几个服务，日志刷屏比没日志更烦人。
// 开关每个请求现读，用户在页面上一点就生效，不用重开监听。
func withAccessLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !Runtime.accessLogEnabled() {
			next.ServeHTTP(w, r)
			return
		}

		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)

		logrus.Infof("[proxy] %s %s %s%s → %d %s (%s)",
			clientIP(r), r.Method, r.Host, r.URL.RequestURI(),
			rec.status, r.Proto, time.Since(start).Round(time.Millisecond))
	})
}

func (r *ProxyRuntime) accessLogEnabled() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.Config.AccessLog
}

// clientIP 记真实来访者。
//
// 反代自己就是最外面那一层，RemoteAddr 就是对端——
// 所以不看 X-Forwarded-For（那是客户端能随便伪造的）。
func clientIP(r *http.Request) string {
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return host
	}
	return r.RemoteAddr
}

// statusRecorder 只为拿到状态码。
//
// 不能简单套一层就算完：WebSocket 要 Hijack，SSE 要 Flush，
// 少实现一个接口就会把功能打掉——这正是「加访问日志加坏了反代」的经典原因。
type statusRecorder struct {
	http.ResponseWriter
	status  int
	written bool
}

func (s *statusRecorder) WriteHeader(code int) {
	if !s.written {
		s.status = code
		s.written = true
	}
	s.ResponseWriter.WriteHeader(code)
}

func (s *statusRecorder) Write(b []byte) (int, error) {
	s.written = true
	return s.ResponseWriter.Write(b)
}

func (s *statusRecorder) Flush() {
	if f, ok := s.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func (s *statusRecorder) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	h, ok := s.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, http.ErrNotSupported
	}
	s.status = http.StatusSwitchingProtocols
	return h.Hijack()
}

// Unwrap 让 http.ResponseController 能拿到真身，
// SetReadDeadline / SetWriteDeadline 这类调用才不会被这层包装挡住
func (s *statusRecorder) Unwrap() http.ResponseWriter {
	return s.ResponseWriter
}
