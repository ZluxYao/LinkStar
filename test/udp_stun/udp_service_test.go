package main

import (
	"fmt"
	"net"
	"testing"
	"time"
)

// 往 main 里 STUN 穿透出来的公网地址发包
//
// 用法:
//  1. 终端A: go run .                       (拿到输出里的 公网IP:Port)
//  2. 终端B: PUBLIC_ADDR=公网IP:Port go test -v -run TestUDPService
//     (Windows PowerShell: $env:PUBLIC_ADDR='公网IP:Port'; go test -v -run TestUDPService)
//  3. 回到终端A 看是否打印 "收到 from ..."
func TestUDPService(t *testing.T) {
	addr := "183.6.155.130:65442"
	// addr := "127.0.0.1:3334"
	if addr == "" {
		t.Skip("跳过: 设置 PUBLIC_ADDR=公网IP:端口 后再跑 (从 main 输出复制)")
	}

	raddr, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		t.Fatalf("解析 %s: %v", addr, err)
	}
	conn, err := net.DialUDP("udp", nil, raddr)
	if err != nil {
		t.Fatalf("dial udp: %v", err)
	}
	defer conn.Close()

	msg := fmt.Sprintf("hello stun hole @%s", time.Now().Format("15:04:05"))
	if _, err = conn.Write([]byte(msg)); err != nil {
		t.Fatalf("write: %v", err)
	}
	t.Logf("已发送 %q -> %s, 去 main 终端看是否打印 '收到 from ...'", msg, addr)
}
