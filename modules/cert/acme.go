package cert

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"linkstar/modules/cert/model"

	"github.com/sirupsen/logrus"
	"golang.org/x/crypto/acme"
)

// 单张证书的签发总超时
const issueTimeout = 10 * time.Minute

// Issue 为一张证书执行 ACME 签发/续期。成功后写盘 + 热替换 Manager 里的指针。
func Issue(ctx context.Context, c model.Certificate) error {
	if !model.IsACME(c.Source) {
		return fmt.Errorf("证书来源 %s 不支持 ACME 签发", c.Source)
	}
	domains := normalizeDomains(c.Domains)
	if len(domains) == 0 {
		return errors.New("未填写任何域名")
	}

	ctx, cancel := context.WithTimeout(ctx, issueTimeout)
	defer cancel()

	logCertInfo(c, fmt.Sprintf("开始签发：%s", strings.Join(domains, ", ")))

	certPEM, keyPEM, err := runACME(ctx, c, domains)
	if err != nil {
		c.LastError = err.Error()
		Runtime.commitStatus(c)
		return err
	}

	if err := writePEM(c.ID, certPEM, keyPEM); err != nil {
		c.LastError = err.Error()
		Runtime.commitStatus(c)
		return err
	}

	c.LastIssue = time.Now()
	if err := loadAndCommit(c); err != nil {
		return err
	}

	logCertInfo(c, "签发完成")
	return nil
}

func runACME(ctx context.Context, c model.Certificate, domains []string) (certPEM, keyPEM []byte, err error) {
	accountKey, err := loadOrCreateAccountKey(c.ID)
	if err != nil {
		return nil, nil, err
	}

	client := &acme.Client{
		Key:          accountKey,
		DirectoryURL: c.ACME.DirectoryURL(),
		UserAgent:    "LinkStar",
	}

	if err := register(ctx, client, c); err != nil {
		return nil, nil, err
	}

	order, err := client.AuthorizeOrder(ctx, acme.DomainIDs(domains...))
	if err != nil {
		return nil, nil, fmt.Errorf("创建订单失败: %w", err)
	}

	if order.Status != acme.StatusReady {
		if err := solveOrder(ctx, client, c, order, domains); err != nil {
			return nil, nil, err
		}
		if order, err = client.WaitOrder(ctx, order.URI); err != nil {
			return nil, nil, fmt.Errorf("等待订单就绪失败: %w", err)
		}
	}

	// 每次签发都用新的证书私钥
	certKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, fmt.Errorf("生成证书私钥失败: %w", err)
	}

	csr, err := makeCSR(certKey, domains)
	if err != nil {
		return nil, nil, err
	}

	der, _, err := client.CreateOrderCert(ctx, order.FinalizeURL, csr, true)
	if err != nil {
		return nil, nil, fmt.Errorf("签发证书失败: %w", err)
	}

	certPEM, err = encodeChain(der)
	if err != nil {
		return nil, nil, err
	}
	keyPEM, err = encodeECKey(certKey)
	if err != nil {
		return nil, nil, err
	}
	return certPEM, keyPEM, nil
}

// register 注册 ACME 账户。账户已存在是正常情况（账户私钥被复用）。
func register(ctx context.Context, client *acme.Client, c model.Certificate) error {
	acct := &acme.Account{}
	if c.ACME.Email != "" {
		acct.Contact = []string{"mailto:" + c.ACME.Email}
	}
	if c.ACME.EABKeyID != "" && c.ACME.EABHMAC != "" {
		hmacKey, err := decodeEABKey(c.ACME.EABHMAC)
		if err != nil {
			return fmt.Errorf("外部账户绑定密钥无效: %w", err)
		}
		acct.ExternalAccountBinding = &acme.ExternalAccountBinding{
			KID: c.ACME.EABKeyID,
			Key: hmacKey,
		}
	}

	_, err := client.Register(ctx, acct, acme.AcceptTOS)
	if err != nil && !errors.Is(err, acme.ErrAccountAlreadyExists) {
		return fmt.Errorf("注册 ACME 账户失败: %w", err)
	}
	return nil
}

// solveOrder 逐个完成订单里尚未通过的授权
func solveOrder(ctx context.Context, client *acme.Client, c model.Certificate, order *acme.Order, domains []string) error {
	for _, authzURL := range order.AuthzURLs {
		authz, err := client.GetAuthorization(ctx, authzURL)
		if err != nil {
			return fmt.Errorf("读取授权失败: %w", err)
		}
		if authz.Status == acme.StatusValid {
			continue // CA 侧仍在有效期内，跳过
		}

		switch c.Source {
		case model.SourceACMEDNS:
			err = solveDNS01(ctx, client, c, authz, domains)
		case model.SourceACMEHTTP:
			err = solveHTTP01(ctx, client, c, authz)
		default:
			err = fmt.Errorf("不支持的验证方式: %s", c.Source)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func solveDNS01(ctx context.Context, client *acme.Client, c model.Certificate, authz *acme.Authorization, domains []string) error {
	chal := pickChallenge(authz, "dns-01")
	if chal == nil {
		return fmt.Errorf("%s 没有可用的 dns-01 验证方式", authz.Identifier.Value)
	}

	writer, err := resolveDNSSolver(c.ACME.ProviderID)
	if err != nil {
		return err
	}

	value, err := client.DNS01ChallengeRecord(chal.Token)
	if err != nil {
		return fmt.Errorf("计算 DNS 挑战值失败: %w", err)
	}

	root := rootDomainOf(authz.Identifier.Value, domains)
	fqdn := challengeFQDN(authz.Identifier.Value)

	if err := writer.AddTXTRecord(root, fqdn, value); err != nil {
		return fmt.Errorf("写入 TXT 记录失败: %w", err)
	}
	// 无论成败都要清理，避免 _acme-challenge 下堆积垃圾记录
	defer func() {
		if err := writer.RemoveTXTRecord(root, fqdn, value); err != nil {
			logrus.Warnf("[cert] 清理 TXT 记录 %s 失败：%v", fqdn, err)
		}
	}()

	logrus.Infof("[cert] 已写入 TXT %s，等待生效", fqdn)
	if err := waitTXTPropagation(ctx, root, fqdn, value); err != nil {
		return err
	}

	return acceptAndWait(ctx, client, chal, authz)
}

func solveHTTP01(ctx context.Context, client *acme.Client, c model.Certificate, authz *acme.Authorization) error {
	chal := pickChallenge(authz, "http-01")
	if chal == nil {
		return fmt.Errorf("%s 没有可用的 http-01 验证方式", authz.Identifier.Value)
	}

	body, err := client.HTTP01ChallengeResponse(chal.Token)
	if err != nil {
		return fmt.Errorf("计算 HTTP 挑战值失败: %w", err)
	}
	path := client.HTTP01ChallengePath(chal.Token)
	port := c.ACME.ChallengePort()

	mux := http.NewServeMux()
	mux.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte(body))
	})

	ln, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		return fmt.Errorf("监听 %d 端口失败（http-01 需要该端口空闲且能被公网访问）: %w", port, err)
	}

	srv := &http.Server{Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	go func() { _ = srv.Serve(ln) }()
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}()

	logrus.Infof("[cert] http-01 验证服务已在 :%d 启动", port)
	return acceptAndWait(ctx, client, chal, authz)
}

func acceptAndWait(ctx context.Context, client *acme.Client, chal *acme.Challenge, authz *acme.Authorization) error {
	if _, err := client.Accept(ctx, chal); err != nil {
		return fmt.Errorf("提交验证失败: %w", err)
	}
	if _, err := client.WaitAuthorization(ctx, authz.URI); err != nil {
		return fmt.Errorf("%s 验证未通过: %w", authz.Identifier.Value, err)
	}
	return nil
}

func pickChallenge(authz *acme.Authorization, typ string) *acme.Challenge {
	for _, ch := range authz.Challenges {
		if ch.Type == typ {
			return ch
		}
	}
	return nil
}

func makeCSR(key crypto.Signer, domains []string) ([]byte, error) {
	tpl := &x509.CertificateRequest{DNSNames: domains}
	// CN 已被废弃且长度上限 64，仅在放得下时填，避免部分 CA 校验失败
	if len(domains[0]) <= 64 {
		tpl.Subject = pkix.Name{CommonName: domains[0]}
	}
	csr, err := x509.CreateCertificateRequest(rand.Reader, tpl, key)
	if err != nil {
		return nil, fmt.Errorf("生成 CSR 失败: %w", err)
	}
	return csr, nil
}

// loadOrCreateAccountKey ACME 账户私钥按证书持久化复用。
// 每次续期都新建账户会撞上 Let's Encrypt 的账户创建限流。
func loadOrCreateAccountKey(id uint) (crypto.Signer, error) {
	p := accountKeyPath(id)

	if data, err := os.ReadFile(p); err == nil {
		block, _ := pem.Decode(data)
		if block != nil {
			if key, err := x509.ParseECPrivateKey(block.Bytes); err == nil {
				return key, nil
			}
		}
		logrus.Warnf("[cert] 账户私钥 %s 无法解析，将重新生成", p)
	}

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("生成 ACME 账户私钥失败: %w", err)
	}

	if _, err := ensureCertDir(id); err != nil {
		return nil, err
	}
	pemBytes, err := encodeECKey(key)
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(p, pemBytes, 0600); err != nil {
		return nil, fmt.Errorf("保存 ACME 账户私钥失败: %w", err)
	}
	return key, nil
}

func encodeECKey(key *ecdsa.PrivateKey) ([]byte, error) {
	der, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return nil, fmt.Errorf("序列化私钥失败: %w", err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: der}), nil
}

// decodeEABKey 外部账户绑定的 HMAC 密钥，CA 通常以 base64url 下发（可能带或不带 padding）
func decodeEABKey(s string) ([]byte, error) {
	s = strings.TrimSpace(s)
	if b, err := base64.RawURLEncoding.DecodeString(s); err == nil {
		return b, nil
	}
	if b, err := base64.URLEncoding.DecodeString(s); err == nil {
		return b, nil
	}
	b, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		return nil, errors.New("不是合法的 base64 字符串")
	}
	return b, nil
}

func encodeChain(der [][]byte) ([]byte, error) {
	if len(der) == 0 {
		return nil, errors.New("CA 返回的证书链为空")
	}
	var out []byte
	for _, b := range der {
		out = append(out, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: b})...)
	}
	return out, nil
}
