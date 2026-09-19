package ddns

import (
	"linkstar/modules/ddns/model"
	"testing"
)

func TestParseFixedIP(t *testing.T) {
	cases := []struct {
		name    string
		arg     string
		rt      model.DNSRecordType
		want    string
		wantErr bool
	}{
		{"入口重定向的占位地址", "192.0.2.1", model.DNSRecordTypeA, "192.0.2.1", false},
		{"前后有空格", "  10.0.0.5  ", model.DNSRecordTypeA, "10.0.0.5", false},
		{"IPv6 写进 AAAA", "2001:db8::1", model.DNSRecordTypeAAAA, "2001:db8::1", false},
		{"IPv6 缩写会被规范化", "2001:0db8:0000::1", model.DNSRecordTypeAAAA, "2001:db8::1", false},
		{"留空", "", model.DNSRecordTypeA, "", true},
		{"不是 IP", "example.com", model.DNSRecordTypeA, "", true},
		{"A 记录填了 IPv6", "2001:db8::1", model.DNSRecordTypeA, "", true},
		{"AAAA 记录填了 IPv4", "192.0.2.1", model.DNSRecordTypeAAAA, "", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := parseFixedIP(c.arg, c.rt)
			if c.wantErr {
				if err == nil {
					t.Fatalf("应该报错，却拿到了 %q", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("不该报错: %v", err)
			}
			if got != c.want {
				t.Fatalf("想要 %q，拿到 %q", c.want, got)
			}
		})
	}
}

// 自定义来源必须原样落库：这类记录存在的意义就是「别动它」，
// 一旦被探测逻辑覆盖，入口重定向那条占位记录就会被写成真公网 IP。
func TestCustomSourceGoesThroughResolveIP(t *testing.T) {
	rec := &model.DDNSRecord{
		RecordType:   model.DNSRecordTypeA,
		IPSourceType: model.IPSourceCustom,
		IPSourceArg:  "192.0.2.1",
	}
	got, err := resolveIP(rec)
	if err != nil {
		t.Fatalf("解析自定义来源失败: %v", err)
	}
	if got != "192.0.2.1" {
		t.Fatalf("想要 192.0.2.1，拿到 %q", got)
	}
}
