// Package domain 放域名比对的公共规则。
//
// 证书按 SNI 挑证书、反向代理按 Host 选路由，用的是同一套匹配语义。
// RFC 6125 那几条边界（通配只覆盖一层、不覆盖裸域）很容易写错，
// 所以只留一份实现，两边都从这里取。
package domain

import "strings"

// Normalize 把域名规整成可比较形式：去空格、去末尾根点、转小写。
//
// SNI、Host 头、用户在表单里填的域名都可能带大小写或末尾的点，
// 比较前必须先过这里，否则 "Fw.Zlux.Top." 会匹配不上 "fw.zlux.top"。
func Normalize(name string) string {
	return strings.ToLower(strings.TrimSuffix(strings.TrimSpace(name), "."))
}

// IsWildcard pattern 是否是 *.example.com 形式的通配域名
func IsWildcard(pattern string) bool {
	return strings.HasPrefix(pattern, "*.")
}

// MatchWildcard 按 RFC 6125 判断通配符域名是否覆盖 name：
// *.a.com 只覆盖恰好多一层标签的名字，不覆盖 a.com，也不覆盖 x.y.a.com。
//
// 调用方负责先把两边 Normalize。
func MatchWildcard(pattern, name string) bool {
	if !IsWildcard(pattern) {
		return false
	}
	suffix := pattern[1:] // ".a.com"
	if !strings.HasSuffix(name, suffix) {
		return false
	}
	label := name[:len(name)-len(suffix)]
	return label != "" && !strings.Contains(label, ".")
}

// Match 精确匹配或通配匹配，调用方不关心 pattern 是哪种形式时用它
func Match(pattern, name string) bool {
	if IsWildcard(pattern) {
		return MatchWildcard(pattern, name)
	}
	return pattern == name
}
