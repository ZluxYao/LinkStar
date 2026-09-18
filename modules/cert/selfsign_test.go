package cert

import (
	"crypto/x509"
	"net"
	"testing"
	"time"

	"linkstar/modules/cert/model"
)

// 自签证书唯一的用处就是「没有域名也能开 HTTPS」，所以必须能靠 IP 验过。
// 不把本机 IP 写进 SAN 的话，浏览器会在「不受信任」之外再叠一条
// ERR_CERT_COMMON_NAME_INVALID，看着像配错了。
func TestSelfSignedTemplateAlwaysCoversLoopback(t *testing.T) {
	tpl, err := selfSignedTemplate(model.Certificate{ID: 1, Name: "无域名"})
	if err != nil {
		t.Fatalf("攒模板失败: %v", err)
	}

	if len(tpl.DNSNames) != 0 {
		t.Errorf("没填域名却写进了 DNS SAN: %v", tpl.DNSNames)
	}
	if !hasIP(tpl.IPAddresses, net.IPv4(127, 0, 0, 1)) {
		t.Errorf("SAN 里没有 127.0.0.1: %v", tpl.IPAddresses)
	}
	if !hasIP(tpl.IPAddresses, net.IPv6loopback) {
		t.Errorf("SAN 里没有 ::1: %v", tpl.IPAddresses)
	}
	if tpl.Subject.CommonName != "LinkStar" {
		t.Errorf("没填域名时 CN 应为 LinkStar，实际 %q", tpl.Subject.CommonName)
	}
}

func TestSelfSignedTemplateUsesFirstDomainAsCN(t *testing.T) {
	tpl, err := selfSignedTemplate(model.Certificate{
		ID:      2,
		Domains: []string{"nas.home.lan", "media.home.lan"},
	})
	if err != nil {
		t.Fatalf("攒模板失败: %v", err)
	}

	if tpl.Subject.CommonName != "nas.home.lan" {
		t.Errorf("CN 应取第一个域名，实际 %q", tpl.Subject.CommonName)
	}
	if len(tpl.DNSNames) != 2 || tpl.DNSNames[1] != "media.home.lan" {
		t.Errorf("域名没有全部进 SAN: %v", tpl.DNSNames)
	}
	// 填了域名也不能把 IP 丢了：这类证书多半还要拿来敲 IP 访问
	if !hasIP(tpl.IPAddresses, net.IPv4(127, 0, 0, 1)) {
		t.Errorf("填了域名之后 SAN 里丢了 127.0.0.1: %v", tpl.IPAddresses)
	}
}

// NotBefore 往前推是给时钟没同步的 NAS / 软路由留的余量。
// 这一条错了的表现是「刚签出来的证书报尚未生效」，比过期还难查。
func TestSelfSignedTemplateBackdatesNotBefore(t *testing.T) {
	tpl, err := selfSignedTemplate(model.Certificate{ID: 3})
	if err != nil {
		t.Fatalf("攒模板失败: %v", err)
	}

	now := time.Now()
	if !tpl.NotBefore.Before(now.Add(-30 * time.Minute)) {
		t.Errorf("NotBefore 没有往前推足够余量: %s", tpl.NotBefore)
	}
	if tpl.NotAfter.Before(now.AddDate(selfSignedYears, 0, -1)) {
		t.Errorf("有效期不足 %d 年: %s", selfSignedYears, tpl.NotAfter)
	}
	if tpl.ExtKeyUsage[0] != x509.ExtKeyUsageServerAuth {
		t.Errorf("缺 serverAuth，TLS 用不了: %v", tpl.ExtKeyUsage)
	}
}

// ensureSelfSigned 是调度器每 60 秒调一次的，还早着就必须原地返回；
// 判断写反的话会变成每分钟重签一张，连着的 TLS 会被换掉。
func TestEnsureSelfSignedSkipsWhenFarFromExpiry(t *testing.T) {
	ensureSelfSigned(model.Certificate{
		ID:       9999,
		Enabled:  true,
		Source:   model.SourceSelfSigned,
		NotAfter: time.Now().AddDate(1, 0, 0),
	})

	// 真签了就会 Store 进 Manager，Lookup 指名要它就能拿到
	if _, err := Runtime.Manager.Lookup(9999, ""); err == nil {
		t.Error("离到期还有一年却重签了")
	}
}

// 和上一条配对：没签过的时候必须真的签出来并装载。
// 少了这条，上面那个「没签」的断言就可能是因为压根签不动而恒成立。
func TestEnsureSelfSignedIssuesWhenMissing(t *testing.T) {
	t.Chdir(t.TempDir()) // PEM 落在 data/cert/{id}/，别写进源码目录
	defer Runtime.Manager.Remove(9998)

	ensureSelfSigned(model.Certificate{
		ID:      9998,
		Enabled: true,
		Source:  model.SourceSelfSigned,
		Domains: []string{"nas.home.lan"},
	})

	got, err := Runtime.Manager.Lookup(9998, "")
	if err != nil {
		t.Fatalf("从未签发过却没有签: %v", err)
	}
	if got.Leaf == nil || got.Leaf.Subject.CommonName != "nas.home.lan" {
		t.Errorf("装载进来的不是刚签的那张: %+v", got.Leaf)
	}
	// 敲 IP 访问是这类证书的主场景，验证链必须认 127.0.0.1
	if err := got.Leaf.VerifyHostname("127.0.0.1"); err != nil {
		t.Errorf("证书对不上 127.0.0.1: %v", err)
	}
}

func hasIP(list []net.IP, want net.IP) bool {
	for _, ip := range list {
		if ip.Equal(want) {
			return true
		}
	}
	return false
}
