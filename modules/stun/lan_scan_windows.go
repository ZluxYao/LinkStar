//go:build windows

package stun

import (
	"errors"
	"syscall"
)

// Winsock 的错误码，对应 Unix 的 ECONNREFUSED / ECONNRESET。
//
// 这里不能写 syscall.ECONNREFUSED：Windows 上那个常量是 Go 自己造的一个
// 占位值（536870934），而真正的 socket 报错回的是 Winsock 的 10061，
// 两者永远对不上。对不上的后果是「端口关着」被当成「这台机器不在」——
// 扫描一台都扫不出来，还不报任何错。
const (
	wsaeConnReset   = syscall.Errno(10054)
	wsaeConnRefused = syscall.Errno(10061)
)

func isAliveErr(err error) bool {
	return errors.Is(err, wsaeConnRefused) || errors.Is(err, wsaeConnReset)
}
