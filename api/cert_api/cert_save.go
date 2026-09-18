package cert_api

import (
	"linkstar/middleware"
	"linkstar/modules/cert"
	"linkstar/modules/cert/model"
	"linkstar/utils/res"

	"github.com/gin-gonic/gin"
)

// CertSaveRequest 新建/修改证书的请求体，两个接口共用
type CertSaveRequest struct {
	ID        uint             `json:"id"`
	Name      string           `json:"name" binding:"required"`
	Source    model.CertSource `json:"source" binding:"required"`
	Domains   []string         `json:"domains"`
	Enabled   *bool            `json:"enabled"` // 不传视为启用
	IsDefault bool             `json:"isDefault"`

	// source = path
	CertFile string `json:"certFile"`
	KeyFile  string `json:"keyFile"`

	// source = acme-*
	ACME ACMERequest `json:"acme"`
}

type ACMERequest struct {
	Directory  string `json:"directory"`
	Email      string `json:"email"`
	ProviderID uint   `json:"providerId"`
	HTTPPort   uint16 `json:"httpPort"`
	EABKeyID   string `json:"eabKeyId"`
	EABHMAC    string `json:"eabHmac"` // 修改时留空表示沿用原值
	RenewDays  int    `json:"renewDays"`
}

// CertAddView 新建证书
func (CertApi) CertAddView(c *gin.Context) {
	cr := middleware.GetBindRequest[CertSaveRequest](c)

	created, err := cert.AddCertificate(cr.toModel())
	if err != nil {
		res.FailWithMsg(err.Error(), c)
		return
	}
	res.OkWithData(toCertView(created), c)
}

// CertUpdateView 修改证书
func (CertApi) CertUpdateView(c *gin.Context) {
	cr := middleware.GetBindRequest[CertSaveRequest](c)
	if cr.ID == 0 {
		res.FailWithMsg("证书 ID 不能为空", c)
		return
	}

	m := cr.toModel()

	// EAB HMAC 从不下发给前端，留空说明用户没改，沿用原值
	if m.ACME.EABHMAC == "" {
		if old, ok := cert.Runtime.Find(cr.ID); ok {
			m.ACME.EABHMAC = old.ACME.EABHMAC
		}
	}

	updated, err := cert.UpdateCertificate(m)
	if err != nil {
		res.FailWithMsg(err.Error(), c)
		return
	}
	res.OkWithData(toCertView(updated), c)
}

func (r CertSaveRequest) toModel() model.Certificate {
	enabled := true
	if r.Enabled != nil {
		enabled = *r.Enabled
	}
	return model.Certificate{
		ID:        r.ID,
		Name:      r.Name,
		Source:    r.Source,
		Domains:   r.Domains,
		Enabled:   enabled,
		IsDefault: r.IsDefault,
		CertFile:  r.CertFile,
		KeyFile:   r.KeyFile,
		ACME: model.ACMEOptions{
			Directory:  r.ACME.Directory,
			Email:      r.ACME.Email,
			ProviderID: r.ACME.ProviderID,
			HTTPPort:   r.ACME.HTTPPort,
			EABKeyID:   r.ACME.EABKeyID,
			EABHMAC:    r.ACME.EABHMAC,
			RenewDays:  r.ACME.RenewDays,
		},
	}
}
