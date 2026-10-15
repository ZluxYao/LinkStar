//go:build integration

package stun

import "testing"

func TestNatType(t *testing.T) {
	udpResult, tcpResult, err := DetectAndClassifyNat()
	if err != nil {
		t.Fatalf("启动时检测 NAT 类型失败: %v", err)
	}
	if udpResult != nil {
		t.Logf("UDP NAT检测结果: type=%s mapping=%s filtering=%s", udpResult.NatType, udpResult.Mapping, udpResult.Filtering)
	}
	if tcpResult != nil {
		t.Logf("TCP NAT检测结果: type=%s mapping=%s", tcpResult.NatType, tcpResult.Mapping)
	}
}
