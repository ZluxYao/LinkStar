package stun

import (
	"encoding/binary"
	"testing"
)

// TestProcRouteIPv4 /proc/net/route 的地址按本机字节序打印，小端和大端机器印出来不一样。
// mipsle 路由器、x86、ARM 都是小端；大端的 mips 也有人在用。
func TestProcRouteIPv4(t *testing.T) {
	littleEndian := binary.NativeEndian.Uint16([]byte{1, 0}) == 1
	cases := map[string]string{"00000000": ""} // 0.0.0.0 是「没有网关」
	if littleEndian {
		cases["0101A8C0"] = "192.168.1.1"
		cases["0164A8C0"] = "192.168.100.1"
	} else {
		cases["C0A80101"] = "192.168.1.1"
		cases["C0A86401"] = "192.168.100.1"
	}
	for in, want := range cases {
		if got := procRouteIPv4(in); got != want {
			t.Errorf("procRouteIPv4(%q) = %q，want %q", in, got, want)
		}
	}
}
