package cert

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"time"

	"linkstar/modules/cert/model"
)

// 自签证书的有效期。
//
// 签的人和验的人是同一台机器，没有吊销、没有 CA、没有谁来查——短有效期
// 换不来任何安全性，只会换来「某天早上突然打不开」。所以直接给十年。
const selfSignedYears = 10

// GenerateSelfSigned 现签一张自签证书，落盘并热加载。
//
// 域名可以一个都不填，这正是它存在的理由：TLS 协议规定服务器必须出示证书，
// 没有证书就没有 HTTPS，所以「只想要加密、没有域名」的人也得有一张。
// 本机所有 IP 都写进 SAN，敲 IP 访问时至少名字能对上，浏览器只剩
// 「不受信任」一条警告，而不是再叠一条 ERR_CERT_COMMON_NAME_INVALID。
//
// 换不来的东西也说清楚：自签没人认，浏览器一定拦一道，要点「继续前往」。
// 想要地址栏干净只能用真证书（ACME 签发或自己上传）。
func GenerateSelfSigned(c model.Certificate) error {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return fmt.Errorf("生成私钥失败: %w", err)
	}

	tpl, err := selfSignedTemplate(c)
	if err != nil {
		return err
	}

	der, err := x509.CreateCertificate(rand.Reader, tpl, tpl, &key.PublicKey, key)
	if err != nil {
		return fmt.Errorf("签发自签证书失败: %w", err)
	}
	keyDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return fmt.Errorf("序列化私钥失败: %w", err)
	}

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER})

	if err := writePEM(c.ID, certPEM, keyPEM); err != nil {
		return err
	}
	if err := loadAndCommit(c); err != nil {
		return err
	}

	// 四个入口都会走到这里（新建、改域名、手动点重新生成、调度器兜底），
	// 日志就只在这一处打，省得每个调用点各说一套话
	logCertInfo(c, fmt.Sprintf("已生成自签证书：%d 个域名 + %d 个本机地址，有效期至 %s",
		len(tpl.DNSNames), len(tpl.IPAddresses), tpl.NotAfter.Format("2006-01-02")))
	return nil
}

// selfSignedTemplate 攒出要签的那张证书长什么样。
// 单独拆出来是为了能直接测——签名那一步是标准库的事，没什么可测的，
// 容易错的是 SAN 里放了什么、有效期从什么时候算起。
func selfSignedTemplate(c model.Certificate) (*x509.Certificate, error) {
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, fmt.Errorf("生成序列号失败: %w", err)
	}

	// 没填域名时 CN 只是个给人看的标签，验证一律看 SAN
	cn := "LinkStar"
	if len(c.Domains) > 0 {
		cn = c.Domains[0]
	}

	now := time.Now()
	return &x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: cn, Organization: []string{"LinkStar 自签证书"}},
		// 往前推一小时：家用软路由 / NAS 开机时钟没同步是常态，
		// 差几分钟就会被判成「证书尚未生效」，比过期还难查
		NotBefore:             now.Add(-time.Hour),
		NotAfter:              now.AddDate(selfSignedYears, 0, 0),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		DNSNames:              append([]string(nil), c.Domains...),
		IPAddresses:           localIPs(),
	}, nil
}

// ensureSelfSigned 没签过就签，快到期了就重签。
// 调度器每轮都会调，所以这里必须便宜且幂等——绝大多数时候直接返回。
func ensureSelfSigned(c model.Certificate) {
	if !c.NotAfter.IsZero() && time.Until(c.NotAfter) > 30*24*time.Hour {
		return
	}
	if err := GenerateSelfSigned(c); err != nil {
		logCertError(c, "生成自签证书失败", err)
	}
}

// localIPs 本机所有对外可达的 IP，外加回环。
//
// 自签证书大多就是拿来敲 IP 访问的，不把 IP 写进 SAN 的话浏览器会
// 额外报一条 ERR_CERT_COMMON_NAME_INVALID——同样是红锁，却让人误以为
// 是配错了而不是「本来就没人认」。
//
// 注意这是签发那一刻的快照：换了网段或者 DHCP 换了地址，需要重新生成
// （证书页上点一次「重新生成」即可）。
func localIPs() []net.IP {
	ips := []net.IP{net.IPv4(127, 0, 0, 1), net.IPv6loopback}

	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return ips
	}
	for _, a := range addrs {
		n, ok := a.(*net.IPNet)
		if !ok || n.IP.IsLoopback() || n.IP.IsLinkLocalUnicast() || n.IP.IsLinkLocalMulticast() {
			continue
		}
		ips = append(ips, n.IP)
	}
	return ips
}
