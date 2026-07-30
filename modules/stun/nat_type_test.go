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
		t.Logf("TCP NAT检测结果7: type=%s mapping=%s", tcpResult.NatType, tcpResult.Mapping)
	}
}

func TestClassifyTCPNatTypeEIMFallsBackToPortRestricted(t *testing.T) {
	status := &NATStatus{Mapping: MappingEndpointIndependent}

	got := ClassifyTCPNatType(status, "192.168.1.10", "203.0.113.10")
	if got != PortRestricted {
		t.Fatalf("ClassifyTCPNatType() = %q, want %q", got, PortRestricted)
	}
}

func TestClassifyTCPNatTypePublicDirect(t *testing.T) {
	status := &NATStatus{Mapping: MappingEndpointIndependent}

	got := ClassifyTCPNatType(status, "203.0.113.10", "203.0.113.10")
	if got != OpenInternet {
		t.Fatalf("ClassifyTCPNatType() = %q, want %q", got, OpenInternet)
	}
}
