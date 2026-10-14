package stun

import (
	"net"
	"sync"
	"testing"
	"time"
)

func Test不同UDP目标不会共用同一条会话(t *testing.T) {
	recv := func(t *testing.T) (*net.UDPConn, func() []string) {
		t.Helper()
		ln, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
		if err != nil {
			t.Fatal(err)
		}
		var mu sync.Mutex
		var got []string
		go func() {
			buf := make([]byte, 64)
			for {
				n, _, err := ln.ReadFromUDP(buf)
				if err != nil {
					return
				}
				mu.Lock()
				got = append(got, string(append([]byte(nil), buf[:n]...)))
				mu.Unlock()
			}
		}()
		return ln, func() []string {
			mu.Lock()
			defer mu.Unlock()
			return append([]string(nil), got...)
		}
	}
	a, gotA := recv(t)
	b, gotB := recv(t)
	t.Cleanup(func() {
		a.Close()
		b.Close()
	})

	local, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { local.Close() })

	remote := &net.UDPAddr{IP: net.IPv4(203, 0, 113, 9), Port: 40000}
	udpSessions.Delete(udpSessionKey(local.LocalAddr(), remote, a.LocalAddr().String()))
	udpSessions.Delete(udpSessionKey(local.LocalAddr(), remote, b.LocalAddr().String()))

	ForwardUDP(local, remote, []byte("to-A"), a.LocalAddr().String())
	time.Sleep(50 * time.Millisecond)
	ForwardUDP(local, remote, []byte("to-B"), b.LocalAddr().String())
	time.Sleep(50 * time.Millisecond)

	has := func(items []string, want string) bool {
		for _, item := range items {
			if item == want {
				return true
			}
		}
		return false
	}
	if !has(gotA(), "to-A") || has(gotA(), "to-B") {
		t.Fatalf("A 应只收到 to-A，实际 %q", gotA())
	}
	if !has(gotB(), "to-B") {
		t.Fatalf("B 应收到 to-B，实际 %q", gotB())
	}
}
