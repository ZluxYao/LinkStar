package cert

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"testing"
	"time"

	"linkstar/modules/cert/model"
)

// fakeCert 造一张只有 SAN 的假证书。匹配逻辑只读 Leaf.DNSNames，
// 不需要真去生成密钥对。
func fakeCert(names ...string) *tls.Certificate {
	return &tls.Certificate{Leaf: &x509.Certificate{DNSNames: names}}
}

func store(m *Manager, id uint, enabled, isDefault bool, names ...string) {
	m.Store(id, fakeCert(names...), model.Certificate{
		ID: id, Enabled: enabled, IsDefault: isDefault,
	}, "", "", time.Time{}, time.Time{})
}

// certOf 断言拿到的是哪张证书——用第一个 SAN 当身份标识
func certOf(t *testing.T, c *tls.Certificate) string {
	t.Helper()
	if c == nil || c.Leaf == nil || len(c.Leaf.DNSNames) == 0 {
		return ""
	}
	return c.Leaf.DNSNames[0]
}

func TestLookupWildcardFollowsRFC6125(t *testing.T) {
	m := NewManager()
	store(m, 1, true, false, "*.example.com")
	// 必须再放一张不相干的证书：只剩一张时 Lookup 的最后一条兜底规则
	// 会无视 SNI 直接把它返回，那样就测不到匹配逻辑了
	store(m, 2, true, false, "unrelated.example.org")

	// *.example.com 只覆盖恰好多一层标签的名字
	for _, name := range []string{"a.example.com", "www.example.com"} {
		c, err := m.Lookup(0, name)
		if err != nil {
			t.Errorf("%s 应该被 *.example.com 覆盖，却报 %v", name, err)
			continue
		}
		if got := certOf(t, c); got != "*.example.com" {
			t.Errorf("%s 应命中通配符证书，实际拿到 %q", name, got)
		}
	}

	for _, name := range []string{"example.com", "a.b.example.com", "notexample.com", "com"} {
		if _, err := m.Lookup(0, name); !errors.Is(err, ErrNoCertificate) {
			t.Errorf("%s 不该被 *.example.com 覆盖，实际 %v", name, err)
		}
	}
}

// TestLookupSingleCertIgnoresSNI 只有一张证书时无条件返回它。
// 这是刻意的兜底：家用场景通常就一张证书、用户又没勾「默认」，
// 此时发一张名字对不上的证书让浏览器报「证书与域名不符」，
// 比握手直接失败给出的信息更有用。
func TestLookupSingleCertIgnoresSNI(t *testing.T) {
	m := NewManager()
	store(m, 1, true, false, "*.example.com")

	if _, err := m.Lookup(0, "totally.unrelated.org"); err != nil {
		t.Errorf("唯一证书应无条件返回，却报 %v", err)
	}
}

func TestLookupPrefersExactOverWildcard(t *testing.T) {
	m := NewManager()
	store(m, 1, true, false, "*.example.com")
	store(m, 2, true, false, "a.example.com")

	c, err := m.Lookup(0, "a.example.com")
	if err != nil {
		t.Fatalf("查找失败: %v", err)
	}
	if got := certOf(t, c); got != "a.example.com" {
		t.Errorf("精确匹配应该赢过通配符，实际拿到 %q", got)
	}
}

func TestLookupBoundCertIDWins(t *testing.T) {
	m := NewManager()
	store(m, 1, true, true, "default.example.com")
	store(m, 2, true, false, "other.example.com")

	// 显式绑定时不看 SNI
	c, err := m.Lookup(2, "default.example.com")
	if err != nil {
		t.Fatalf("查找失败: %v", err)
	}
	if got := certOf(t, c); got != "other.example.com" {
		t.Errorf("绑定证书应优先于 SNI，实际拿到 %q", got)
	}

	// 绑定的证书被禁用 → 明确报错，不悄悄回落到别的证书
	m.SetEnabled(2, false, false)
	if _, err := m.Lookup(2, "default.example.com"); !errors.Is(err, ErrNoCertificate) {
		t.Errorf("绑定证书禁用后应报 ErrNoCertificate，实际 %v", err)
	}
}

func TestLookupFallbacks(t *testing.T) {
	t.Run("SNI 匹配不上走默认证书", func(t *testing.T) {
		m := NewManager()
		store(m, 1, true, false, "a.example.com")
		store(m, 2, true, true, "fallback.example.com")

		c, err := m.Lookup(0, "unknown.example.org")
		if err != nil {
			t.Fatalf("查找失败: %v", err)
		}
		if got := certOf(t, c); got != "fallback.example.com" {
			t.Errorf("应回落默认证书，实际 %q", got)
		}
	})

	t.Run("IP 直连没有 SNI 也走默认证书", func(t *testing.T) {
		m := NewManager()
		store(m, 1, true, false, "a.example.com")
		store(m, 2, true, true, "fallback.example.com")

		c, err := m.Lookup(0, "")
		if err != nil {
			t.Fatalf("查找失败: %v", err)
		}
		if got := certOf(t, c); got != "fallback.example.com" {
			t.Errorf("无 SNI 应回落默认证书，实际 %q", got)
		}
	})

	t.Run("只有一张证书时直接用它", func(t *testing.T) {
		m := NewManager()
		store(m, 1, true, false, "only.example.com")

		if _, err := m.Lookup(0, ""); err != nil {
			t.Errorf("唯一证书应被直接使用，却报 %v", err)
		}
	})

	t.Run("多张证书又没默认证书就该失败", func(t *testing.T) {
		m := NewManager()
		store(m, 1, true, false, "a.example.com")
		store(m, 2, true, false, "b.example.com")

		if _, err := m.Lookup(0, ""); !errors.Is(err, ErrNoCertificate) {
			t.Errorf("想要 ErrNoCertificate，实际 %v", err)
		}
	})
}

func TestLookupSkipsDisabledAndUnloaded(t *testing.T) {
	m := NewManager()
	store(m, 1, false, false, "a.example.com")

	if _, err := m.Lookup(0, "a.example.com"); !errors.Is(err, ErrNoCertificate) {
		t.Errorf("禁用的证书不该被匹配，实际 %v", err)
	}
	// 禁用的证书也不该被「只剩一张」的兜底规则捡回来
	if _, err := m.Lookup(0, ""); !errors.Is(err, ErrNoCertificate) {
		t.Errorf("全部禁用时无 SNI 也该失败，实际 %v", err)
	}

	// 模块是并发初始化的：证书还没加载完时必须报错而不是 panic
	m2 := NewManager()
	m2.mu.Lock()
	m2.entries[1] = &entry{domains: []string{"a.example.com"}, enabled: true}
	m2.mu.Unlock()
	if _, err := m2.Lookup(0, "a.example.com"); !errors.Is(err, ErrNoCertificate) {
		t.Errorf("未加载的证书应报 ErrNoCertificate，实际 %v", err)
	}
}

func TestLookupNormalizesSNI(t *testing.T) {
	m := NewManager()
	store(m, 1, true, false, "a.example.com")

	// 大小写和 FQDN 末尾点都不该影响匹配
	for _, name := range []string{"A.Example.COM", "a.example.com."} {
		if _, err := m.Lookup(0, name); err != nil {
			t.Errorf("%s 应该匹配上，却报 %v", name, err)
		}
	}
}

func TestRemoveDropsCertificate(t *testing.T) {
	m := NewManager()
	store(m, 1, true, false, "a.example.com")
	store(m, 2, true, false, "b.example.com")
	// 留够三张，删掉一张后不会触发「只剩一张就无条件返回」的兜底
	store(m, 3, true, false, "c.example.com")

	m.Remove(1)
	if _, err := m.Lookup(0, "a.example.com"); !errors.Is(err, ErrNoCertificate) {
		t.Errorf("删除后不该还能查到，实际 %v", err)
	}
	c, err := m.Lookup(0, "b.example.com")
	if err != nil {
		t.Fatalf("其它证书不该受影响，实际 %v", err)
	}
	if got := certOf(t, c); got != "b.example.com" {
		t.Errorf("想要 b.example.com，实际 %q", got)
	}
}

// TestStoreHotSwapKeepsIdentity 续期时只换指针，条目本身不重建——
// 这是「洞不用重启、连接不用断」的前提
func TestStoreHotSwapKeepsIdentity(t *testing.T) {
	m := NewManager()
	store(m, 1, true, false, "a.example.com")

	m.mu.RLock()
	before := m.entries[1]
	m.mu.RUnlock()

	store(m, 1, true, false, "a.example.com")

	m.mu.RLock()
	after := m.entries[1]
	m.mu.RUnlock()

	if before != after {
		t.Error("续期应复用同一个 entry，只替换内部的原子指针")
	}
}

// TestServerTLSConfigNeverOffersH2 洞口不能替内网服务许诺 HTTP/2。
//
// 这是真出过的事故：ALPN 报了 h2，浏览器就按 HTTP/2 发连接前言和二进制帧，
// 而洞口只是 io.Copy，内网那头是明文 HTTP/1.1，回的是 "HTTP/1.1 200 OK"
// 文本，浏览器直接 ERR_HTTP2_PROTOCOL_ERROR——证书装好了反而全白屏。
//
// 中间没人翻译协议，ALPN 就不能报比内网实际会说的更多的东西。
func TestServerTLSConfigNeverOffersH2(t *testing.T) {
	cfg := NewManager().ServerTLSConfig(0)

	for _, p := range cfg.NextProtos {
		if p == "h2" || p == "h2c" {
			t.Fatalf("ALPN 不能报 %q：洞口不解析协议，内网说的是 HTTP/1.1", p)
		}
	}
	if len(cfg.NextProtos) != 1 || cfg.NextProtos[0] != "http/1.1" {
		t.Errorf("NextProtos = %v, 想要 [http/1.1]", cfg.NextProtos)
	}
}
