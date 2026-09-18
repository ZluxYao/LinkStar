package cert

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"linkstar/modules/cert/model"
	"linkstar/utils/domain"
)

// ErrNoCertificate 没有任何可用证书（证书未加载完 / 全部禁用 / SNI 匹配不上）
var ErrNoCertificate = errors.New("没有可用的证书")

// entry 一张已加载的证书。cert 用原子指针持有，
// 续期时只替换指针，不需要重启 STUN 洞、也不影响已建立的连接。
type entry struct {
	cert    atomic.Pointer[tls.Certificate]
	domains []string // 证书实际覆盖的域名（取自 SAN）
	enabled bool
	isDef   bool

	// path 来源用于热重载的 mtime 记录
	certPath  string
	keyPath   string
	certMtime time.Time
	keyMtime  time.Time
}

// Manager 持有全部已加载证书，对外只暴露一个 GetCertificate 回调
type Manager struct {
	mu      sync.RWMutex
	entries map[uint]*entry
}

func NewManager() *Manager {
	return &Manager{entries: make(map[uint]*entry)}
}

// ServerTLSConfig 为某个 STUN 洞构造服务端 TLS 配置（纯字节管道模式）。
// certID 为 0 表示不绑定具体证书，按 SNI 自动匹配。
//
// 返回的 *tls.Config 可被该洞上所有连接共享：GetCertificate 每次握手
// 都会实时回调，因此证书续期后新连接自动使用新证书。
func (m *Manager) ServerTLSConfig(certID uint) *tls.Config {
	// 只报 http/1.1，绝对不能报 h2。
	//
	// 洞口是纯字节管道：TLS 解完就 io.Copy 给内网，中间没人翻译协议。
	// ALPN 是在替内网服务许诺「我说这个协议」——报了 h2，浏览器就直接发
	// HTTP/2 连接前言和二进制帧，而内网那头是个明文 HTTP/1.1 服务，
	// 回过来的是 "HTTP/1.1 200 OK" 文本，浏览器只会看到
	// ERR_HTTP2_PROTOCOL_ERROR。
	//
	// 勾了「转发给内网时也用 HTTPS」也一样：那是另一条独立的 TLS 连接，
	// 拨它的时候没带 NextProtos，后端照样落在 HTTP/1.1。
	//
	// HTTP/1.1 不影响 WebSocket、SSE、chunked——它们本来就是 1.1 的东西，
	// 照样透传。
	//
	// 这段限制只针对字节管道。网关（反向代理）模式下真的有人在翻译协议，
	// 走 ServerTLSConfigH2。
	return m.serverTLSConfig(certID, []string{"http/1.1"})
}

// ServerTLSConfigH2 反向代理网关洞用的服务端 TLS 配置。
//
// 与 ServerTLSConfig 唯一的差别是 ALPN 多报一个 h2：网关后面站着
// httputil.ReverseProxy，它会把 HTTP/2 请求解析成 *http.Request 再用
// HTTP/1.1 发给内网服务，协议有人翻译，这个许诺兑得了。
func (m *Manager) ServerTLSConfigH2(certID uint) *tls.Config {
	return m.serverTLSConfig(certID, []string{"h2", "http/1.1"})
}

func (m *Manager) serverTLSConfig(certID uint, nextProtos []string) *tls.Config {
	return &tls.Config{
		MinVersion: tls.VersionTLS12,
		NextProtos: nextProtos,
		GetCertificate: func(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
			return m.Lookup(certID, hello.ServerName)
		},
	}
}

// Lookup 按「绑定 ID → SNI 精确 → SNI 通配 → 默认证书 → 唯一证书」顺序取证书
func (m *Manager) Lookup(certID uint, serverName string) (*tls.Certificate, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// ① 服务显式绑定了证书
	if certID != 0 {
		e, ok := m.entries[certID]
		if !ok || !e.enabled {
			return nil, fmt.Errorf("%w: 绑定的证书 %d 不可用", ErrNoCertificate, certID)
		}
		if c := e.cert.Load(); c != nil {
			return c, nil
		}
		return nil, fmt.Errorf("%w: 证书 %d 尚未加载", ErrNoCertificate, certID)
	}

	name := domain.Normalize(serverName)

	// ② SNI 精确匹配
	if name != "" {
		if c := m.matchLocked(name, false); c != nil {
			return c, nil
		}
		// ③ SNI 通配匹配
		if c := m.matchLocked(name, true); c != nil {
			return c, nil
		}
	}

	// ④ 默认证书兜底（SNI 为空的 IP 直连会走到这里）
	for _, e := range m.entries {
		if e.enabled && e.isDef {
			if c := e.cert.Load(); c != nil {
				return c, nil
			}
		}
	}

	// ⑤ 只有一张可用证书时直接用它
	var only *tls.Certificate
	count := 0
	for _, e := range m.entries {
		if !e.enabled {
			continue
		}
		if c := e.cert.Load(); c != nil {
			only = c
			count++
		}
	}
	if count == 1 {
		return only, nil
	}

	if serverName == "" {
		return nil, fmt.Errorf("%w: 客户端未发送 SNI 且无默认证书", ErrNoCertificate)
	}
	return nil, fmt.Errorf("%w: 没有覆盖 %s 的证书", ErrNoCertificate, serverName)
}

// matchLocked 调用前需持有读锁。wildcard=false 精确匹配，true 只匹配通配符条目。
func (m *Manager) matchLocked(name string, wildcard bool) *tls.Certificate {
	for _, e := range m.entries {
		if !e.enabled {
			continue
		}
		for _, d := range e.domains {
			if domain.IsWildcard(d) != wildcard {
				continue
			}
			if !domain.Match(d, name) {
				continue
			}
			if c := e.cert.Load(); c != nil {
				return c
			}
		}
	}
	return nil
}

// Store 加载一张证书并放入/替换管理器中的条目
func (m *Manager) Store(id uint, c *tls.Certificate, cfg model.Certificate, certPath, keyPath string, certMtime, keyMtime time.Time) {
	domains := certDomains(c)
	// 证书里没解析出 SAN 时退回用户填写的域名
	if len(domains) == 0 {
		domains = normalizeDomains(cfg.Domains)
	}

	m.mu.Lock()
	e, ok := m.entries[id]
	if !ok {
		e = &entry{}
		m.entries[id] = e
	}
	e.domains = domains
	e.enabled = cfg.Enabled
	e.isDef = cfg.IsDefault
	e.certPath = certPath
	e.keyPath = keyPath
	e.certMtime = certMtime
	e.keyMtime = keyMtime
	m.mu.Unlock()

	// 指针最后换，保证换上去的一定是完整状态
	e.cert.Store(c)
}

// SetEnabled 更新启用/默认标记，不触碰已加载的证书内容
func (m *Manager) SetEnabled(id uint, enabled, isDefault bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if e, ok := m.entries[id]; ok {
		e.enabled = enabled
		e.isDef = isDefault
	}
}

// Remove 移除一张证书
func (m *Manager) Remove(id uint) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.entries, id)
}

// mtimes 读取某条目记录的文件修改时间，用于 path 来源热重载比对
func (m *Manager) mtimes(id uint) (certPath, keyPath string, certMtime, keyMtime time.Time, ok bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	e, exist := m.entries[id]
	if !exist {
		return "", "", time.Time{}, time.Time{}, false
	}
	return e.certPath, e.keyPath, e.certMtime, e.keyMtime, true
}

// certDomains 从证书 SAN 里取出全部覆盖域名
func certDomains(c *tls.Certificate) []string {
	if c == nil || c.Leaf == nil {
		return nil
	}
	return normalizeDomains(c.Leaf.DNSNames)
}

func normalizeDomains(in []string) []string {
	out := make([]string, 0, len(in))
	seen := make(map[string]bool, len(in))
	for _, d := range in {
		d = domain.Normalize(d)
		if d == "" || seen[d] {
			continue
		}
		seen[d] = true
		out = append(out, d)
	}
	return out
}

// parseLeaf 解析并填充 Leaf，后续 SNI 匹配与到期判断都依赖它
func parseLeaf(c *tls.Certificate) error {
	if c.Leaf != nil {
		return nil
	}
	if len(c.Certificate) == 0 {
		return errors.New("证书链为空")
	}
	leaf, err := x509.ParseCertificate(c.Certificate[0])
	if err != nil {
		return fmt.Errorf("解析证书失败: %w", err)
	}
	c.Leaf = leaf
	return nil
}
