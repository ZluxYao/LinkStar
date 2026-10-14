package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"net"

	"github.com/sirupsen/logrus"
)

// setupToken 首次设置密码时，非本机请求必须带上的口令。
// 本机（回环地址）不需要：坐在这台机器前的人打开后台就能设。
// 从局域网别的设备来设密码时，得先看到启动日志里的这一串，避免同网段任何人抢先设下密码。
var setupToken string

// ensureSetupToken 未初始化时生成口令并写进日志。已经初始化就不必再生成。
func ensureSetupToken() {
	if setupToken != "" {
		return
	}
	buf := make([]byte, 4)
	if _, err := rand.Read(buf); err != nil {
		logrus.Error("生成首次设置口令失败，其他设备将无法设置密码：", err)
		return
	}
	setupToken = hex.EncodeToString(buf)
	logrus.Warnf("首次设置管理密码：本机浏览器可直接设置；从其他设备设置时要填口令 %s。这个口令只写在日志里", setupToken)
}

// AllowsSetup 这次设置密码的请求能不能放行。
// remoteAddr 用请求的 RemoteAddr，不看 X-Forwarded-For，避免伪造来源。
func AllowsSetup(remoteAddr, given string) bool {
	if isLoopback(remoteAddr) {
		return true
	}
	if setupToken == "" || given == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(given), []byte(setupToken)) == 1
}

func isLoopback(remoteAddr string) bool {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		host = remoteAddr
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
