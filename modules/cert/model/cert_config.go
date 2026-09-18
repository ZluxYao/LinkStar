package model

import "time"

// CertSource 证书来源
type CertSource string

const (
	SourceUpload   CertSource = "upload"     // 手动上传/粘贴 PEM
	SourcePath     CertSource = "path"       // 读文件系统路径（外部 certbot / acme.sh 续期）
	SourceACMEDNS  CertSource = "acme-dns01" // ACME DNS-01，复用 DDNS 服务商写 TXT
	SourceACMEHTTP CertSource = "acme-http01"
	// SourceSelfSigned LinkStar 自己签一张。给「只要加密、没有域名、
	// 也不想为证书操心」的人用——浏览器会警告不受信任，但 TLS 能握上手。
	SourceSelfSigned CertSource = "self-signed"
)

// IsValidSource 该来源是否受支持
func IsValidSource(s CertSource) bool {
	switch s {
	case SourceUpload, SourcePath, SourceACMEDNS, SourceACMEHTTP, SourceSelfSigned:
		return true
	}
	return false
}

// IsACME 是否需要走 ACME 签发流程
func IsACME(s CertSource) bool {
	return s == SourceACMEDNS || s == SourceACMEHTTP
}

// Let's Encrypt 正式 / 测试环境
const (
	LEProduction = "https://acme-v02.api.letsencrypt.org/directory"
	LEStaging    = "https://acme-staging-v02.api.letsencrypt.org/directory"
)

// 默认值
const (
	DefaultRenewDays = 30
	DefaultHTTPPort  = 80
)

// CertConfig 证书模块配置
type CertConfig struct {
	Certificates []Certificate `json:"certificates"`

	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// Certificate 一张证书的配置 + 运行态
// 注意：PEM 内容不落在这个文件里，存放于 data/cert/{id}/
type Certificate struct {
	ID        uint       `json:"id"`
	Name      string     `json:"name"`      // 备注名
	Source    CertSource `json:"source"`    // 来源
	Domains   []string   `json:"domains"`   // 覆盖域名，如 ["zlux.top", "*.zlux.top"]
	Enabled   bool       `json:"enabled"`   // 是否启用
	IsDefault bool       `json:"isDefault"` // SNI 匹配不上时的兜底证书

	// source = path 时使用
	CertFile string `json:"certFile"`
	KeyFile  string `json:"keyFile"`

	// source = acme-* 时使用
	ACME ACMEOptions `json:"acme"`

	// 运行态，由加载/签发结果回写
	NotBefore time.Time `json:"notBefore"`
	NotAfter  time.Time `json:"notAfter"`
	Issuer    string    `json:"issuer"`
	LastError string    `json:"lastError"`
	LastIssue time.Time `json:"lastIssueAt"`

	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// ACMEOptions ACME 签发参数
type ACMEOptions struct {
	Directory  string `json:"directory"`  // ACME 目录地址，空则用 LEProduction
	Email      string `json:"email"`      // 账户联系邮箱
	ProviderID uint   `json:"providerId"` // dns-01：复用 DDNS 里已配好的服务商 ID
	HTTPPort   uint16 `json:"httpPort"`   // http-01 监听端口，默认 80
	EABKeyID   string `json:"eabKeyId"`   // 外部账户绑定（ZeroSSL 等需要）
	EABHMAC    string `json:"eabHmac"`
	RenewDays  int    `json:"renewDays"` // 剩余天数低于此值触发续期，默认 30
}

// RenewThreshold 返回有效的续期阈值天数
func (o ACMEOptions) RenewThreshold() int {
	if o.RenewDays <= 0 {
		return DefaultRenewDays
	}
	return o.RenewDays
}

// DirectoryURL 返回有效的 ACME 目录地址
func (o ACMEOptions) DirectoryURL() string {
	if o.Directory == "" {
		return LEProduction
	}
	return o.Directory
}

// ChallengePort 返回有效的 http-01 监听端口
func (o ACMEOptions) ChallengePort() uint16 {
	if o.HTTPPort == 0 {
		return DefaultHTTPPort
	}
	return o.HTTPPort
}
