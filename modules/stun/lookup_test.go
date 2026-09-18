package stun

import (
	"testing"

	"linkstar/modules/stun/model"
)

// TestPublicScheme 外网访问用 http 还是 https，取决于洞口那一层 TLS 是谁的。
//
// 真踩过的坑：内网是个已经带证书的 HTTPS 服务，用户勾了「转发给内网时也用 HTTPS」
// 却没勾「洞口终结 TLS」。这时洞口是在替内网加密——外面那一段是明文，
// LinkStar 却把链接写成 https://，点开必然 ERR_SSL_PROTOCOL_ERROR。
func TestPublicScheme(t *testing.T) {
	cases := []struct {
		name         string
		tlsTerminate bool
		backendHTTPS bool
		https        bool
		want         string
	}{
		{"什么都不勾", false, false, false, "http"},
		{"只勾展示：纯管道，外面看到的就是内网服务本身", false, false, true, "https"},
		{"终结 TLS：洞口自己出示证书", true, false, false, "https"},
		{"终结 TLS + 内网也是 HTTPS：外面仍是洞口那张证书", true, true, false, "https"},
		{"只勾内网 HTTPS：洞口替内网加密，外面是明文", false, true, false, "http"},
		{"只勾内网 HTTPS，展示勾也开着：展示勾骗不了协议", false, true, true, "http"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := PublicScheme(&model.Service{
				TLSTerminate: c.tlsTerminate,
				BackendHTTPS: c.backendHTTPS,
				Https:        c.https,
			})
			if got != c.want {
				t.Errorf("PublicScheme = %q, 想要 %q", got, c.want)
			}
		})
	}

	if got := PublicScheme(nil); got != "http" {
		t.Errorf("服务为 nil 时 = %q, 想要 http", got)
	}
}

// TestInternalScheme 内网直连只看服务本身，和洞口怎么配没关系
func TestInternalScheme(t *testing.T) {
	if got := InternalScheme(&model.Service{TLSTerminate: true}); got != "http" {
		t.Errorf("洞口终结 TLS 不代表内网是 HTTPS，实际 %q", got)
	}
	if got := InternalScheme(&model.Service{BackendHTTPS: true}); got != "https" {
		t.Errorf("内网确实说 TLS，实际 %q", got)
	}
}
