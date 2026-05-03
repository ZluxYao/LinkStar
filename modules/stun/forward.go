package stun

import (
	"io"
	"net"
	"time"

	"github.com/sirupsen/logrus"
)

// 双向复制
func ForwardTCP(src net.Conn, targetAddr string, protocol string) {
	defer src.Close()

	dst, err := net.DialTimeout(protocol, targetAddr, 3*time.Second)
	if err != nil {
		logrus.Errorf("连接内网目标失败 [%s]: %v", targetAddr, err)
		return
	}
	defer dst.Close()

	go func() {
		_, _ = io.Copy(dst, src)
		dst.Close() // src断了，关掉dst，让下面的io.Copy立刻返回
	}()

	_, _ = io.Copy(src, dst)
	src.Close() // dst断了，关掉src，让上面的io.Copy立刻返回
}

// ForwardUDP
func ForwardUDP(localConn *net.UDPConn, remoteAddr *net.UDPAddr, data []byte, targetAddr string) {
	// 1. 把包转发给内网目标
	dst, err := net.Dial("udp", targetAddr)
	if err != nil {
		return
	}
	defer dst.Close()

	dst.Write(data)

	// 2. 等内网目标的回包，再转发回外网客户端
	buf := make([]byte, 65535)
	dst.SetReadDeadline(time.Now().Add(5 * time.Second))
	n, err := dst.Read(buf)
	if err != nil {
		return
	}

	// 3. 用原始 localConn 把回包写回给 remoteAddr
	localConn.WriteToUDP(buf[:n], remoteAddr)
}
