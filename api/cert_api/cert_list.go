package cert_api

import (
	"math"
	"time"

	"linkstar/modules/cert"
	"linkstar/modules/cert/model"
	"linkstar/utils/res"

	"github.com/gin-gonic/gin"
)

// CertView 证书对外视图。
// PEM 内容永远不出接口——私钥不该离开这台机器。
type CertView struct {
	ID        uint             `json:"id"`
	Name      string           `json:"name"`
	Source    model.CertSource `json:"source"`
	Domains   []string         `json:"domains"`
	Enabled   bool             `json:"enabled"`
	IsDefault bool             `json:"isDefault"`

	CertFile string `json:"certFile"`
	KeyFile  string `json:"keyFile"`

	ACME ACMEView `json:"acme"`

	NotBefore string `json:"notBefore,omitempty"`
	NotAfter  string `json:"notAfter,omitempty"`
	Issuer    string `json:"issuer,omitempty"`
	LastError string `json:"lastError,omitempty"`
	LastIssue string `json:"lastIssueAt,omitempty"`

	// 前端直接用的派生字段
	Loaded   bool `json:"loaded"`   // 当前是否已装载进内存，能真正对外提供服务
	DaysLeft int  `json:"daysLeft"` // 剩余有效天数，未签发为 0
	Expired  bool `json:"expired"`

	CreatedAt string `json:"createdAt,omitempty"`
	UpdatedAt string `json:"updatedAt,omitempty"`
}

// ACMEView ACME 参数视图，EAB HMAC 属于密钥，只回是否已配置
type ACMEView struct {
	Directory  string `json:"directory"`
	Email      string `json:"email"`
	ProviderID uint   `json:"providerId"`
	HTTPPort   uint16 `json:"httpPort"`
	EABKeyID   string `json:"eabKeyId"`
	HasEABHMAC bool   `json:"hasEabHmac"`
	RenewDays  int    `json:"renewDays"`
}

// CertListView 列出全部证书
func (CertApi) CertListView(c *gin.Context) {
	cfg := cert.Runtime.Snapshot()

	list := make([]CertView, 0, len(cfg.Certificates))
	for _, item := range cfg.Certificates {
		list = append(list, toCertView(item))
	}
	res.OkWithList(list, int64(len(list)), c)
}

func toCertView(m model.Certificate) CertView {
	// 没有域名时给空切片而不是 nil——nil 序列化成 null，前端对它做 .map 会直接崩
	domains := m.Domains
	if domains == nil {
		domains = []string{}
	}
	v := CertView{
		ID:        m.ID,
		Name:      m.Name,
		Source:    m.Source,
		Domains:   domains,
		Enabled:   m.Enabled,
		IsDefault: m.IsDefault,
		CertFile:  m.CertFile,
		KeyFile:   m.KeyFile,
		Issuer:    m.Issuer,
		LastError: m.LastError,
		ACME: ACMEView{
			Directory:  m.ACME.Directory,
			Email:      m.ACME.Email,
			ProviderID: m.ACME.ProviderID,
			HTTPPort:   m.ACME.HTTPPort,
			EABKeyID:   m.ACME.EABKeyID,
			HasEABHMAC: m.ACME.EABHMAC != "",
			RenewDays:  m.ACME.RenewThreshold(),
		},
	}
	if v.Domains == nil {
		v.Domains = []string{}
	}

	v.NotBefore = formatTime(m.NotBefore)
	v.NotAfter = formatTime(m.NotAfter)
	v.LastIssue = formatTime(m.LastIssue)
	v.CreatedAt = formatTime(m.CreatedAt)
	v.UpdatedAt = formatTime(m.UpdatedAt)

	if !m.NotAfter.IsZero() {
		remain := time.Until(m.NotAfter)
		v.Expired = remain <= 0
		// 向下取整：还剩 0.9 天就显示 0 天，不做善意的四舍五入
		v.DaysLeft = int(math.Floor(remain.Hours() / 24))
		if v.DaysLeft < 0 {
			v.DaysLeft = 0
		}
	}

	// 以内存里的实际状态为准：配置里写着启用，不代表真的装载成功了
	if _, err := cert.Runtime.Manager.Lookup(m.ID, ""); err == nil {
		v.Loaded = true
	}
	return v
}

func formatTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format("2006-01-02T15:04:05Z07:00")
}
