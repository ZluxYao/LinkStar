package stun

import (
	"crypto/tls"
	"errors"
	"io"
	"net"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

// tlsHandshakeTimeout 握手必须有超时兜底：
// 不带 TLS 时连接一进来就立刻拨号，而 TLS 握手要等对端先说话，
// 端口扫描器开个连接不发任何数据就能钉住一个 goroutine。
//
// 用 var 而不是 const，只为让测试能把它调短——运行时没有任何地方改它。
var tlsHandshakeTimeout = 10 * time.Second

// ForwardOptions 单个 STUN 洞的转发选项
type ForwardOptions struct {
	// TLSConfig 非 nil 时由 LinkStar 在洞口终结 TLS。
	// 一个洞固定对应一个后端服务，身份由端口决定，因此不需要解析 HTTP——
	// 握手完成后继续按字节流转发即可，WebSocket / SSE / HTTP2 天然透传。
	TLSConfig *tls.Config

	// BackendHTTPS 拨内网时用 tls.Dial 而不是裸 dial，等价于 nginx 的 proxy_pass https://。
	//
	// 注意它不是「内网是不是 HTTPS」的描述，而是「LinkStar 要不要自己去和内网握手」，
	// 只在 TLSConfig != nil（洞口终结了 TLS、LinkStar 真的要重新建一条连接）时才成立。
	// TLSConfig 为 nil 时这个洞是纯字节管道，握手是浏览器和内网服务之间的事，
	// 此时再置 true 就成了双层 TLS——浏览器的 ClientHello 被当明文塞进 LinkStar
	// 自己那条 TLS，必然握手失败。配置层（normalizeBackendHTTPS / normalizeServices）
	// 已经挡掉这个组合，这里保持诚实：给什么就做什么。
	BackendHTTPS bool
}

// ForwardTCP 把洞上的连接转发到内网服务
func ForwardTCP(src net.Conn, targetAddr string, protocol string, opt ForwardOptions) {
	defer src.Close()

	// 先完成握手再拨后端：握手失败就不该白拨一次内网
	if opt.TLSConfig != nil {
		tlsConn := tls.Server(src, opt.TLSConfig)
		if err := tlsConn.SetDeadline(time.Now().Add(tlsHandshakeTimeout)); err != nil {
			return
		}
		if err := tlsConn.Handshake(); err != nil {
			logrus.Debugf("TLS 握手失败 [%s -> %s]: %v", src.RemoteAddr(), targetAddr, err)
			return
		}
		if err := tlsConn.SetDeadline(time.Time{}); err != nil {
			return
		}
		// tls.Conn 实现了 net.Conn，下面的转发逻辑完全不用变
		src = tlsConn
	}

	var dst net.Conn
	var err error
	if opt.BackendHTTPS {
		// 内网服务通常是自签证书，本来就无从校验
		dst, err = tls.DialWithDialer(
			&net.Dialer{Timeout: 3 * time.Second},
			protocol,
			targetAddr,
			&tls.Config{InsecureSkipVerify: true},
		)
	} else {
		dst, err = net.DialTimeout(protocol, targetAddr, 3*time.Second)
	}
	if err != nil {
		logrus.Errorf("连接内网目标失败 [%s]: %s", targetAddr, describeDialError(err))
		return
	}
	defer dst.Close()

	go func() {
		_, _ = io.Copy(dst, src)
		dst.Close()
	}()

	_, _ = io.Copy(src, dst)
	src.Close()
}

// describeDialError 把拨号错误翻译成能直接照着改配置的话。
//
// 勾了「转发给内网时也用 HTTPS」但服务其实是明文，是这里最常见的误配置，
// 而 Go 原样抛出的 "tls: first record does not look like a TLS handshake"
// 完全没提该去动哪个开关。
func describeDialError(err error) string {
	var rec tls.RecordHeaderError
	if errors.As(err, &rec) {
		return "内网服务不是 HTTPS，却按 HTTPS 去连了——请取消该服务的「转发给内网时也用 HTTPS」勾选（原始错误：" + err.Error() + "）"
	}
	return err.Error()
}

const udpSessionTimeout = 30 * time.Second

var udpSessions sync.Map // remoteAddr.String() → net.Conn

// ForwardUDP 维护 session 表，同一客户端复用同一内部连接，持续转发回包。
func ForwardUDP(localConn *net.UDPConn, remoteAddr *net.UDPAddr, data []byte, targetAddr string) {
	key := remoteAddr.String()

	if v, ok := udpSessions.Load(key); ok {
		v.(net.Conn).Write(data)
		return
	}

	dst, err := net.Dial("udp", targetAddr)
	if err != nil {
		logrus.Errorf("连接内网目标失败 [%s]: %v", targetAddr, err)
		return
	}

	if actual, loaded := udpSessions.LoadOrStore(key, dst); loaded {
		dst.Close()
		actual.(net.Conn).Write(data)
		return
	}

	dst.Write(data)

	go func() {
		buf := make([]byte, 65535)
		for {
			dst.SetReadDeadline(time.Now().Add(udpSessionTimeout))
			n, err := dst.Read(buf)
			if err != nil {
				break
			}
			localConn.WriteToUDP(buf[:n], remoteAddr)
		}
		udpSessions.Delete(key)
		dst.Close()
	}()
}
