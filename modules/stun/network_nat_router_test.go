package stun

import "testing"

func TestClassifyIP(t *testing.T) {
	tests := []struct {
		name string
		ip   string
		want string
	}{
		{name: "private", ip: "192.168.1.1", want: IPTypePrivate},
		{name: "cgn gateway", ip: "100.72.0.1", want: IPTypeCGN},
		{name: "public", ip: "114.114.114.114", want: IPTypePublic},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := classifyIP(tt.ip); got != tt.want {
				t.Fatalf("classifyIP(%q) = %q, want %q", tt.ip, got, tt.want)
			}
		})
	}
}
