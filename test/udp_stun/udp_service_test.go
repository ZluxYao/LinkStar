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
const (
	targetAddr   = "14.19.73.174:21741" // 从 main 输出复制
	sendInterval = 2 * time.Second       // 发送间隔
	testDuration = 30 * time.Second      // 持续发多久
)

func TestUDPService(t *testing.T) {
	raddr, err := net.ResolveUDPAddr("udp", targetAddr)
	if err != nil {
		t.Fatalf("解析 %s: %v", targetAddr, err)
	}
	conn, err := net.DialUDP("udp", nil, raddr)
	if err != nil {
		t.Fatalf("dial udp: %v", err)
	}
	defer conn.Close()

	t.Logf("开始每 %v 发一次, 持续 %v -> %s", sendInterval, testDuration, targetAddr)

	ticker := time.NewTicker(sendInterval)
	defer ticker.Stop()
	deadline := time.After(testDuration)
	seq := 0

	for {
		select {
		case <-deadline:
			t.Logf("发送结束, 共发 %d 包", seq)
			return
		case ts := <-ticker.C:
			seq++
			msg := fmt.Sprintf("#%d hello @%s", seq, ts.Format("15:04:05"))
			if _, err = conn.Write([]byte(msg)); err != nil {
				t.Errorf("write #%d: %v", seq, err)
				return
			}
			t.Logf("发 #%d: %s", seq, msg)
		}
	}
}
