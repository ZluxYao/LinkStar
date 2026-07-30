//go:build !windows

package main

import "syscall"

// setReuseAddr 在socket创建后、connect前设置SO_REUSEADDR。
// TCP Mapping交叉验证需要"两条连接绑定同一个本地端口"(目的地址不同即可共存)，
// Linux上要求先后两个socket都设了SO_REUSEADDR，第二次bind才会放行，
// 所以所有TCP探测统一走这个Control。
// 之所以拆成build-tag文件：Unix的fd类型是int，Windows是syscall.Handle，
// syscall.SetsockoptInt的签名不同，单文件无法跨平台编译。
func setReuseAddr(network, address string, c syscall.RawConn) error {
	var serr error
	err := c.Control(func(fd uintptr) {
		serr = syscall.SetsockoptInt(int(fd), syscall.SOL_SOCKET, syscall.SO_REUSEADDR, 1)
	})
	if err != nil {
		return err
	}
	return serr
}
