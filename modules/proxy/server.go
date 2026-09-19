package proxy

import (
	"crypto/tls"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"linkstar/modules/cert"
	"linkstar/modules/plainhttp"
	"linkstar/modules/proxy/model"

	"github.com/sirupsen/logrus"
)

// serverLogWriter 把 net/http 自己那套 log 转进 logrus，并补上是哪个端口。
//
// net/http 里最值得看的一条就是 "TLS handshake error"：挑不出证书时，
// 浏览器那头只有一个 ERR_SSL_PROTOCOL_ERROR，原因全在这句话里。
type serverLogWriter struct{ port int }

func (w serverLogWriter) Write(p []byte) (int, error) {
	logrus.Warnf("反代 :%d %s", w.port, strings.TrimRight(string(p), "\n"))
	return len(p), nil
}

// boundServer 一个端口上的监听。
//
// router 是原子指针：改站点表时直接换掉它，不重开监听，
// 正在传的下载 / WebSocket / SSE 一条都不断。
type boundServer struct {
	spec   listenSpec
	router atomic.Pointer[Router]

	mu      sync.Mutex
	srv     *http.Server
	running bool
	lastErr string
}

// serverPool 所有监听端口。
//
// 对应 GoDoxy 的 entrypoint.servers：一个 addr → server 的表。
// 端口不是全局唯一的，站点想自己占一个就自己占一个。
type serverPool struct {
	mu      sync.Mutex
	servers map[int]*boundServer
}

func newServerPool() *serverPool {
	return &serverPool{servers: make(map[int]*boundServer)}
}

// ListenState 一个监听此刻的状态，给页面用
type ListenState struct {
	Port      int    `json:"port"`
	TLS       bool   `json:"tls"`
	CertID    uint   `json:"certId"`
	SiteCount int    `json:"siteCount"`
	Running   bool   `json:"running"`
	LastError string `json:"lastError"`
}

// Apply 把监听调整成配置描述的样子。
//
// 三种情况分开处理，尽量少动正在跑的连接：
//   - 端口没了 → 关掉
//   - 端口在、监听参数没变 → 只换转发表指针，连接一条不断
//   - 端口在、参数变了（改了 TLS 或证书）→ 只重开这一个端口，不影响别的
//
// 某个端口起不来不影响别的端口，错误合并返回。
func (p *serverPool) Apply(cfg model.ProxyConfig) error {
	plan := planBindings(cfg)

	wanted := make(map[int]binding, len(plan))
	for _, b := range plan {
		wanted[b.Port] = b
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	for port, s := range p.servers {
		if _, ok := wanted[port]; !ok {
			s.stop()
			delete(p.servers, port)
		}
	}

	var errs []error
	for _, b := range plan {
		router := NewRouter(b.Sites)

		if s, ok := p.servers[b.Port]; ok {
			if s.running && s.spec == b.spec() {
				s.router.Store(router) // 热换表
				continue
			}
			s.stop()
			delete(p.servers, b.Port)
		}

		s := &boundServer{spec: b.spec()}
		s.router.Store(router)
		if err := s.start(); err != nil {
			// 起不来也留在表里：状态和原因要在页面上看得到，
			// 下一次 Apply 会再试一遍
			p.servers[b.Port] = s
			errs = append(errs, err)
			continue
		}
		p.servers[b.Port] = s
	}
	return errors.Join(errs...)
}

// StopAll 关掉全部监听
func (p *serverPool) StopAll() {
	p.mu.Lock()
	defer p.mu.Unlock()
	for port, s := range p.servers {
		s.stop()
		delete(p.servers, port)
	}
}

// States 当前全部监听的状态，按端口排好
func (p *serverPool) States() []ListenState {
	p.mu.Lock()
	servers := make([]*boundServer, 0, len(p.servers))
	for _, s := range p.servers {
		servers = append(servers, s)
	}
	p.mu.Unlock()

	out := make([]ListenState, 0, len(servers))
	for _, s := range servers {
		s.mu.Lock()
		state := ListenState{
			Port:      s.spec.Port,
			TLS:       s.spec.TLS,
			CertID:    s.spec.CertID,
			Running:   s.running,
			LastError: s.lastErr,
		}
		s.mu.Unlock()
		if r := s.router.Load(); r != nil {
			state.SiteCount = r.Size()
		}
		out = append(out, state)
	}
	sortStates(out)
	return out
}

func (s *boundServer) start() error {
	addr := ":" + strconv.Itoa(s.spec.Port)

	ln, err := net.Listen("tcp", addr)
	if err != nil {
		msg := describeListenError(s.spec.Port, err)
		s.mu.Lock()
		s.lastErr = msg
		s.running = false
		s.mu.Unlock()
		return errors.New(msg)
	}

	if s.spec.TLS {
		// 先接住明文 HTTP：地址栏敲 example.com:8443 时浏览器默认按 http 发，
		// 不管的话 TLS 握手直接失败、连接被掐断，用户看到的是错误页 +「不安全」。
		// 这层在 tls.NewListener 外面，先它一步拿到裸连接。
		ln = plainhttp.Listener(ln, plainhttp.DefaultTimeout)
		// 这里 ALPN 可以报 h2：TLS 在这终结，后面站着 ReverseProxy，
		// 它会把 HTTP/2 请求解析成 *http.Request 再用 HTTP/1.1 发给内网，
		// 协议真的有人翻译。（对照 cert.ServerTLSConfig 上那段注释——
		// 那说的是 STUN 裸管道，没人翻译，所以只能报 http/1.1。）
		ln = tls.NewListener(ln, cert.Runtime.Manager.ServerTLSConfigH2(s.spec.CertID))
	}

	srv := &http.Server{
		Handler: newHandler(s.router.Load),
		// 连上却不发请求头的连接会一直占着 goroutine。
		// 这个端口可能被打洞暴露到公网，必须设。
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       60 * time.Second,
		// 握手失败是这里最难查的一类故障：浏览器只显示 ERR_SSL_PROTOCOL_ERROR，
		// 而真正的原因（挑不出证书、客户端没发 SNI）只在 net/http 内部。
		// 不接这根线的话它会走 Go 的默认 logger 打到 stderr，和 LinkStar 自己的
		// 日志不在一处，等于没有。
		ErrorLog: log.New(serverLogWriter{port: s.spec.Port}, "", 0),
	}

	s.mu.Lock()
	s.srv = srv
	s.running = true
	s.lastErr = ""
	s.mu.Unlock()

	go func() {
		err := srv.Serve(ln)

		s.mu.Lock()
		defer s.mu.Unlock()
		if s.srv != srv {
			return // 已经被换成新的一轮监听了，这条退出是我们自己关的
		}
		s.running = false
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			s.lastErr = err.Error()
			logrus.Errorf("[proxy] 端口 %d 监听异常退出：%s", s.spec.Port, err)
		}
	}()

	scheme := "http"
	if s.spec.TLS {
		scheme = "https"
	}
	logrus.Infof("[proxy] 已监听 %s（%s）", addr, scheme)
	return nil
}

// stop 停掉这个端口的监听。
//
// 用 Close 不用 Shutdown：Shutdown 会等 hijack 走的连接（WebSocket）自己结束，
// 而用户就是要它现在停，等下去只会把停止流程挂死。
func (s *boundServer) stop() {
	s.mu.Lock()
	srv := s.srv
	s.srv = nil
	s.running = false
	s.mu.Unlock()

	if srv != nil {
		_ = srv.Close()
		logrus.Infof("[proxy] 端口 %d 已停止监听", s.spec.Port)
	}
}

func sortStates(states []ListenState) {
	for i := 1; i < len(states); i++ {
		for j := i; j > 0 && states[j].Port < states[j-1].Port; j-- {
			states[j], states[j-1] = states[j-1], states[j]
		}
	}
}

// ListenStates 反向代理此刻开着哪些端口、各自跑没跑起来
func ListenStates() []ListenState { return Runtime.pool.States() }

// describeListenError 把 bind 失败翻译成能照着改的话。
// Windows 和 Linux 的错误文案不一样，两边都认。
func describeListenError(port int, err error) string {
	msg := err.Error()
	switch {
	case strings.Contains(msg, "address already in use"),
		strings.Contains(msg, "Only one usage of each socket address"):
		return fmt.Sprintf("端口 %d 已被占用——换一个端口，或者把占着它的程序停掉", port)
	case strings.Contains(msg, "permission denied"),
		strings.Contains(msg, "forbidden by its access permissions"):
		return fmt.Sprintf("没权限监听端口 %d——1024 以下的端口要管理员权限，建议换个高位端口", port)
	}
	return fmt.Sprintf("监听端口 %d 失败：%s", port, msg)
}
