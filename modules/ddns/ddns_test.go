package ddns

import (
	"fmt"
	"linkstar/modules/ddns/dns"
	"linkstar/modules/ddns/model"
	"os"
	"testing"
)

//	go test ./modules/ddns/ -v -run TestSyncRecord
//
// 测试同步记录
func TestSyncRecord(t *testing.T) {
	token := os.Getenv("CF_ApiToken")
	if token == "" {
		t.Skip("未设置 token")
	}
	cf := dns.NewCloudflare(token)

	recordIpv4 := model.DDNSRecord{
		Domain:       "zlux.top",
		SubDomain:    "test",
		TTL:          60,
		RecordType:   model.DNSRecordTypeA,
		IPSourceType: model.IPSourceWeb,
		Proxied:      false,
	}

	SyncRecord(cf, &recordIpv4)
}

// 测试Web获取ip
func TestResolveIP(t *testing.T) {
	recordIpv4 := model.DDNSRecord{
		RecordType:   model.DNSRecordTypeA,
		IPSourceType: model.IPSourceWeb,
	}
	ipv4, err := resolveIP(&recordIpv4)
	fmt.Println(ipv4, err)

	recordIpv6 := model.DDNSRecord{
		RecordType:   model.DNSRecordTypeAAAA,
		IPSourceType: model.IPSourceWeb,
	}
	ipv6, err := resolveIP(&recordIpv6)
	fmt.Println(ipv6, err)
}

// go test ./modules/ddns/ -v -run TestDefaultWebIPSources
func TestDefaultWebIPSources(t *testing.T) {
	initDefaultIPSources()
}
