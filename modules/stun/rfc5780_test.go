package stun

import (
	"testing"
)

func TestRFC5780UDP(t *testing.T) {
	server, err := selectRFC5780Server(rfc5780StunServer)
	if err != nil {
		t.Fatalf("selectRFC5780Server 失败: %v", err)
	}
	t.Logf("选中服务器: %s primary=%s other=%s delay=%s",
		server.Name, server.Primary, server.Other, server.Delay)

	t.Run("Mapping", func(t *testing.T) {
		behavior, err := detectUDPMapping(server)
		if err != nil {
			t.Fatalf("detectUDPMapping 失败: %v", err)
		}
		t.Logf("UDP Mapping 行为: %s", behavior)
	})

	t.Run("Filtering", func(t *testing.T) {
		behavior, err := detectUDPFiltering(server)
		if err != nil {
			t.Fatalf("detectUDPFiltering 失败: %v", err)
		}
		t.Logf("UDP Filtering 行为: %s", behavior)
	})
}

func TestRFC5780TCP(t *testing.T) {
	server, err := selectRFC5780Server(rfc5780StunServer)
	if err != nil {
		t.Fatalf("selectRFC5780Server 失败: %v", err)
	}
	t.Logf("选中服务器: %s primary=%s other=%s", server.Name, server.Primary, server.Other)

	behavior, err := detectTCPMapping(server)
	if err != nil {
		t.Fatalf("detectTCPMapping 失败: %v", err)
	}
	t.Logf("TCP Mapping 行为: %s", behavior)
}
