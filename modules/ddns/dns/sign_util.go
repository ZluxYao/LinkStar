package dns

// 阿里云/百度/华为等云厂商签名时使用的 RFC3986 转义
// 与标准库 url.QueryEscape 不同:不转义 - _ . ~,空格不转为 +

func signShouldEscape(c byte) bool {
	if 'A' <= c && c <= 'Z' || 'a' <= c && c <= 'z' || '0' <= c && c <= '9' ||
		c == '_' || c == '-' || c == '~' || c == '.' {
		return false
	}
	return true
}

func signEscape(s string) string {
	hexCount := 0
	for i := 0; i < len(s); i++ {
		if signShouldEscape(s[i]) {
			hexCount++
		}
	}
	if hexCount == 0 {
		return s
	}

	const upperhex = "0123456789ABCDEF"
	t := make([]byte, len(s)+2*hexCount)
	j := 0
	for i := 0; i < len(s); i++ {
		c := s[i]
		if signShouldEscape(c) {
			t[j] = '%'
			t[j+1] = upperhex[c>>4]
			t[j+2] = upperhex[c&15]
			j += 3
		} else {
			t[j] = c
			j++
		}
	}
	return string(t)
}
