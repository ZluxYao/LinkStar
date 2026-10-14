package ddns

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func Test网页回显不是IP或不是200就拒绝(t *testing.T) {
	cases := []struct {
		name    string
		status  int
		body    string
		version int
	}{
		{"HTTP 500 的 HTML", http.StatusInternalServerError, "<html>not-an-ip</html>", 4},
		{"HTTP 200 但正文不是 IP", http.StatusOK, "not-an-ip", 4},
		{"私网地址", http.StatusOK, "192.168.0.8", 4},
		{"回环地址", http.StatusOK, "127.0.0.1", 4},
		{"链路本地地址", http.StatusOK, "169.254.1.1", 4},
		{"组播地址", http.StatusOK, "224.0.0.1", 4},
		{"运营商级 NAT", http.StatusOK, "100.64.1.1", 4},
		{"0.0.0.0/8 保留地址", http.StatusOK, "0.0.0.1", 4},
		{"240.0.0.0/4 保留地址", http.StatusOK, "240.0.0.1", 4},
		{"IPv6 不能当作 IPv4", http.StatusOK, "2001:db8::1", 4},
		{"IPv4 不能当作 IPv6", http.StatusOK, "203.0.113.10", 6},
		{"IPv6 唯一本地地址", http.StatusOK, "fc00::1", 6},
		{"IPv6 回环地址", http.StatusOK, "::1", 6},
		{"IPv6 链路本地地址", http.StatusOK, "fe80::1", 6},
		{"IPv6 组播地址", http.StatusOK, "ff02::1", 6},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(c.status)
				_, _ = w.Write([]byte(c.body))
			}))
			defer srv.Close()
			if ip, err := fetchIPFromWeb(srv.URL, c.version); err == nil {
				t.Fatalf("%s 不应被当成 IP，实际 %q", c.body, ip)
			}
		})
	}
}

func newWebIPSource(t *testing.T, body string, status int) *webIPSource {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return &webIPSource{URL: srv.URL}
}

func Test唯一合法来源可以直接采用(t *testing.T) {
	ip, err := fetchIPFromWebList([]*webIPSource{
		newWebIPSource(t, "203.0.113.10", http.StatusOK),
	}, 4)
	if err != nil || ip != "203.0.113.10" {
		t.Fatalf("只有一个有效来源时应采用 203.0.113.10，实际 %q %v", ip, err)
	}
}

func Test跳过无效来源后采用剩下的合法IP(t *testing.T) {
	ip, err := fetchIPFromWebList([]*webIPSource{
		newWebIPSource(t, "<html>not-an-ip</html>", http.StatusInternalServerError),
		newWebIPSource(t, "192.168.0.8", http.StatusOK),
		newWebIPSource(t, "203.0.113.10", http.StatusOK),
	}, 4)
	if err != nil || ip != "203.0.113.10" {
		t.Fatalf("无效来源不应占票，应采用唯一合法 IP，实际 %q %v", ip, err)
	}
}

func Test两个来源一致时采用(t *testing.T) {
	ip, err := fetchIPFromWebList([]*webIPSource{
		newWebIPSource(t, "203.0.113.10", http.StatusOK),
		newWebIPSource(t, "203.0.113.10", http.StatusOK),
	}, 4)
	if err != nil || ip != "203.0.113.10" {
		t.Fatalf("两个来源一致时应采用该 IP，实际 %q %v", ip, err)
	}
}

func Test两个来源一致即可采用即使还有一个不同(t *testing.T) {
	ip, err := fetchIPFromWebList([]*webIPSource{
		newWebIPSource(t, "203.0.113.10", http.StatusOK),
		newWebIPSource(t, "203.0.113.11", http.StatusOK),
		newWebIPSource(t, "203.0.113.10", http.StatusOK),
	}, 4)
	if err != nil || ip != "203.0.113.10" {
		t.Fatalf("两个源都给出 203.0.113.10 时应采用它，实际 %q %v", ip, err)
	}
}

func Test来源互相矛盾时采用最先成功的(t *testing.T) {
	first := newWebIPSource(t, "203.0.113.10", http.StatusOK)
	first.AvgLatency = time.Millisecond
	second := newWebIPSource(t, "203.0.113.11", http.StatusOK)
	second.AvgLatency = 2 * time.Millisecond

	// 评分越低越先试。两边都只有一票时，用最先成功的那个，而不是整次失败。
	ip, err := fetchIPFromWebList([]*webIPSource{first, second}, 4)
	if err != nil || ip != "203.0.113.10" {
		t.Fatalf("来源互相矛盾时应采用最先成功的 203.0.113.10，实际 %q %v", ip, err)
	}
}
