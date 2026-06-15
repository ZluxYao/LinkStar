package dns

import "linkstar/modules/ddns/model"

// DNSProvider 所有DSN服务器实现这个接口

type DNSProvider interface {
	SetRecord(domain string, subDomain string, recordType model.DNSRecordType, ip string, ttl int, proxied bool) error
}
