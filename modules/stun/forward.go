package stun

import (
	"io"
	"net"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

// ForwardTCP
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
		dst.Close()
	}()

	_, _ = io.Copy(src, dst)
	src.Close()
}

const udpSessionTimeout = 30 * time.Second

var udpSessions sync.Map // remoteAddr.String() → net.Conn

// ForwardUDP 维护 session 表，同一客户端复用同一内部连接，持续转发回包。
func ForwardUDP(localConn *net.UDPConn, remoteAddr *net.UDPAddr, data []byte, targetAddr string) {
	key := remoteAddr.String()

	if v, ok := udpSessions.Load(key); ok {
		v.(net.Conn).Write(data)
		return
	}

	dst, err := net.Dial("udp", targetAddr)
	if err != nil {
		logrus.Errorf("连接内网目标失败 [%s]: %v", targetAddr, err)
		return
	}

	if actual, loaded := udpSessions.LoadOrStore(key, dst); loaded {
		dst.Close()
		actual.(net.Conn).Write(data)
		return
	}

	dst.Write(data)

	go func() {
		buf := make([]byte, 65535)
		for {
			dst.SetReadDeadline(time.Now().Add(udpSessionTimeout))
			n, err := dst.Read(buf)
			if err != nil {
				break
			}
			localConn.WriteToUDP(buf[:n], remoteAddr)
		}
		udpSessions.Delete(key)
		dst.Close()
	}()
}
