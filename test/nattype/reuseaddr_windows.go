//go:build windows

package main

import "syscall"

// setReuseAddr 在socket创建后、connect前设置SO_REUSEADDR。
// TCP Mapping交叉验证需要"两条连接绑定同一个本地端口"(目的地址不同即可共存)，
// 没有这个选项，第二次bind会直接报端口占用。
// 之所以拆成build-tag文件：Windows的fd类型是syscall.Handle，Unix是int，
// syscall.SetsockoptInt的签名不同，单文件无法跨平台编译。
func setReuseAddr(network, address string, c syscall.RawConn) error {
	var serr error
	err := c.Control(func(fd uintptr) {
		serr = syscall.SetsockoptInt(syscall.Handle(fd), syscall.SOL_SOCKET, syscall.SO_REUSEADDR, 1)
	})
	if err != nil {
		return err
	}
	return serr
}
