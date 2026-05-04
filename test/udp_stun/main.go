package main

import (
	"fmt"
	"log"
	"net"
	"time"

	"github.com/pion/stun/v2"
)

const stunServer = "stun.tula.nu:3478"

// 主函数
func main() {
	stunAddr, err := net.ResolveUDPAddr("udp", stunServer)
	if err != nil {
		log.Fatalf("解析 STUN 地址失败: %v", err)
	}

	// 本地端口给 0,系统随机分配; 同一个 socket 既发 STUN 又收外来包是穿透成立的前提
	conn, err := net.ListenUDP("udp", &net.UDPAddr{Port: 0})
	if err != nil {
		log.Fatalf("监听 UDP 失败: %v", err)
	}
	defer conn.Close()
	localPort := conn.LocalAddr().(*net.UDPAddr).Port
	log.Printf("UDP 监听本地端口: %d", localPort)

	// 第一次 STUN 握手, 拿公网映射 (此时 read loop 还没起,可以独占读)
	publicIP, publicPort, err := StunHandshakeUDP(conn, stunAddr)
	if err != nil {
		log.Fatalf("STUN 握手失败: %v", err)
	}
	log.Printf("公网映射: %s:%d  ->  本地 :%d", publicIP, publicPort, localPort)
	log.Printf("用 'nc -u %s %d' 或 test 往这个地址发包测试", publicIP, publicPort)

	// 保活: 每 20s 给 STUN 发一次 binding request, 防止 NAT 映射过期
	// 回包会进入主 read loop, 被 stun.IsMessage 过滤掉
	go keepAlive(conn, stunAddr)

	// 起 UDP 服务循环
	UDPService(conn)
}

// 启动udp服务 端口3334
func UDPService(conn *net.UDPConn) {
	buf := make([]byte, 65535)
	for {
		n, remote, err := conn.ReadFromUDP(buf)
		if err != nil {
			log.Printf("ReadFromUDP 失败: %v", err)
			return
		}
		// 自己发的 STUN 请求会有响应回到这, 不当业务消息
		if stun.IsMessage(buf[:n]) {
			continue
		}
		fmt.Printf("收到 from %s: %s\n", remote, string(buf[:n]))
	}
}

// 保活
func keepAlive(conn *net.UDPConn, stunAddr *net.UDPAddr) {
	ticker := time.NewTicker(20 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		msg := stun.MustBuild(stun.TransactionID, stun.BindingRequest)
		if _, err := conn.WriteToUDP(msg.Raw, stunAddr); err != nil {
			log.Printf("保活发送失败: %v", err)
		}
	}
}
