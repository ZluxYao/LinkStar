//go:build !windows

package stun

import (
	"errors"
	"syscall"
)

// 这边这两个常量就是真的 errno，直接比就行；
// Windows 那边不是，原因写在 lan_scan_windows.go。
func isAliveErr(err error) bool {
	return errors.Is(err, syscall.ECONNREFUSED) || errors.Is(err, syscall.ECONNRESET)
}
