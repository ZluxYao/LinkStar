package stun

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/pion/stun"
)

// TestRunReturnsAfterCancel 停掉一个打洞服务，Run 要马上返回。
//
// 原来 Run 最后是死等 errCh：保活 goroutine 收到取消后安静地 return nil，
// 什么也不往 errCh 里放；Accept 循环卡在 Accept 上，监听器要等 Run 返回后
// 的 defer 才关 —— 互相等，Run 永远不返回。调度器等 12 秒放弃，旧的监听器、
// STUN 连接一直挂着。网卡一变要把所有服务重启一遍时，每个服务都要白等这 12 秒。
func TestRunReturnsAfterCancel(t *testing.T) {
	stunAddr := startFakeSTUNServer(t)

	oldOutbound, oldService, oldLocalIP := currentOutboundIface.Load(), Runtime.STUNService, Runtime.Network.LocalIP
	t.Cleanup(func() {
		currentOutboundIface.Store(oldOutbound)
		Runtime.STUNService, Runtime.Network.LocalIP = oldService, oldLocalIP
	})
	currentOutboundIface.Store(&OutboundIface{Name: "lo", LocalIP: "127.0.0.1"})
	Runtime.STUNService = &STUNService{BestSTUNServer: stunAddr}
	Runtime.Network.LocalIP = "127.0.0.1"

	ctx, cancel := context.WithCancel(context.Background())
	alive := make(chan struct{}, 1)
	done := make(chan error, 1)
	go func() {
		done <- NewSTUNRunner().Run(ctx, STUNRequest{
			ServiceName: "test", TargetIP: "127.0.0.1", InternalPort: 9, Protocol: "tcp",
		}, func(s STUNState) {
			if s.State == STUNAlive {
				select {
				case alive <- struct{}{}:
				default:
				}
			}
		})
	}()

	select {
	case <-alive:
	case err := <-done:
		t.Fatalf("还没打通就返回了: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("5 秒内没打通")
	}

	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("取消后 2 秒 Run 还没返回")
	}
}

// startFakeSTUNServer 本机起一个 STUN over TCP，把看到的对端地址原样回过去
func startFakeSTUNServer(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				defer conn.Close()
				buf := make([]byte, 1024)
				for {
					n, err := conn.Read(buf)
					if err != nil {
						return
					}
					req := &stun.Message{Raw: append([]byte(nil), buf[:n]...)}
					if req.Decode() != nil {
						return
					}
					peer := conn.RemoteAddr().(*net.TCPAddr)
					resp := stun.MustBuild(
						stun.NewTransactionIDSetter(req.TransactionID),
						stun.BindingSuccess,
						&stun.XORMappedAddress{IP: peer.IP, Port: peer.Port},
					)
					if _, err := conn.Write(resp.Raw); err != nil {
						return
					}
				}
			}()
		}
	}()
	return ln.Addr().String()
}
