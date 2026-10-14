package auth

import "testing"

func Test本机可以直接设置密码其他设备必须带口令(t *testing.T) {
	old := setupToken
	t.Cleanup(func() { setupToken = old })
	setupToken = "abcd1234"

	cases := []struct {
		name   string
		remote string
		token  string
		want   bool
	}{
		{"本机 IPv4", "127.0.0.1:52341", "", true},
		{"本机 IPv6", "[::1]:52341", "", true},
		{"局域网不带口令", "192.168.1.20:52341", "", false},
		{"局域网口令错误", "192.168.1.20:52341", "00000000", false},
		{"局域网口令正确", "192.168.1.20:52341", "abcd1234", true},
		{"没有端口的地址按原样判断", "10.1.1.1", "abcd1234", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := AllowsSetup(c.remote, c.token); got != c.want {
				t.Fatalf("AllowsSetup(%q, %q) = %v，想要 %v", c.remote, c.token, got, c.want)
			}
		})
	}
}

func Test没有口令时其他设备不能设置(t *testing.T) {
	old := setupToken
	t.Cleanup(func() { setupToken = old })
	setupToken = ""
	if AllowsSetup("203.0.113.8:1", "anything") {
		t.Fatal("口令还没生成时，非本机请求不应放行")
	}
	if !AllowsSetup("127.0.0.1:1", "") {
		t.Fatal("本机不应被空口令挡住")
	}
}
