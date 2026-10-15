package stun

import "testing"

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
