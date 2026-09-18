package cert

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"linkstar/modules/cert/model"

	"github.com/sirupsen/logrus"
)

// AddCertificate 新建一张证书配置。
// upload 来源此时还没有 PEM，需要接着调 UploadPEM；
// acme-* 来源交给调度器去签发。
func AddCertificate(c model.Certificate) (model.Certificate, error) {
	if err := normalizeAndValidate(&c); err != nil {
		return c, err
	}

	now := time.Now()
	c.CreatedAt = now
	c.UpdatedAt = now
	// 运行态一律从零开始，不接受前端传进来的值
	c.NotBefore = time.Time{}
	c.NotAfter = time.Time{}
	c.Issuer = ""
	c.LastError = ""
	c.LastIssue = time.Time{}

	err := Runtime.Update(func(cfg *model.CertConfig) error {
		if err := checkNameConflict(cfg, c.Name, 0); err != nil {
			return err
		}
		c.ID = nextID(cfg)
		if c.IsDefault {
			clearOtherDefaults(cfg, c.ID)
		}
		cfg.Certificates = append(cfg.Certificates, c)
		return nil
	})
	if err != nil {
		return c, err
	}

	// path 来源立刻就能加载；upload 还没文件；acme 交给调度器
	switch {
	case c.Source == model.SourcePath && c.Enabled:
		ReloadCertificate(c)
	case c.Source == model.SourceSelfSigned && c.Enabled:
		// 自签就在本地算，毫秒级，没必要让用户等调度器那一轮
		if err := GenerateSelfSigned(c); err != nil {
			logCertError(c, "生成自签证书失败", err)
		}
	}
	triggerScan()
	return c, nil
}

// UpdateCertificate 修改证书配置。运行态字段（有效期/签发者/错误）保留不动。
func UpdateCertificate(c model.Certificate) (model.Certificate, error) {
	if c.ID == 0 {
		return c, errors.New("证书 ID 不能为空")
	}
	if err := normalizeAndValidate(&c); err != nil {
		return c, err
	}

	var updated model.Certificate
	err := Runtime.Update(func(cfg *model.CertConfig) error {
		idx := -1
		for i := range cfg.Certificates {
			if cfg.Certificates[i].ID == c.ID {
				idx = i
				break
			}
		}
		if idx < 0 {
			return fmt.Errorf("证书 %d 不存在", c.ID)
		}
		if err := checkNameConflict(cfg, c.Name, c.ID); err != nil {
			return err
		}

		old := cfg.Certificates[idx]

		// 保留运行态
		c.CreatedAt = old.CreatedAt
		c.UpdatedAt = time.Now()
		c.NotBefore = old.NotBefore
		c.NotAfter = old.NotAfter
		c.Issuer = old.Issuer
		c.LastError = old.LastError
		c.LastIssue = old.LastIssue

		// 改了域名或换了来源，旧证书已经覆盖不到新域名了。
		// 清掉 NotAfter 让调度器判定为「从未签发」，立刻重签。
		// 旧证书仍在内存里继续服务，直到新证书签下来才换指针。
		if (model.IsACME(c.Source) || c.Source == model.SourceSelfSigned) &&
			(old.Source != c.Source || domainsChanged(old.Domains, c.Domains)) {
			c.NotAfter = time.Time{}
			c.NotBefore = time.Time{}
		}

		if c.IsDefault {
			clearOtherDefaults(cfg, c.ID)
		}
		cfg.Certificates[idx] = c
		updated = c
		return nil
	})
	if err != nil {
		return c, err
	}

	ReloadCertificate(updated)
	triggerScan()
	return updated, nil
}

// RemoveCertificate 删除证书配置，并清理 data/cert/{id}/
func RemoveCertificate(id uint) error {
	if id == 0 {
		return errors.New("证书 ID 不能为空")
	}

	if err := Runtime.Update(func(cfg *model.CertConfig) error {
		for i := range cfg.Certificates {
			if cfg.Certificates[i].ID != id {
				continue
			}
			cfg.Certificates = append(cfg.Certificates[:i], cfg.Certificates[i+1:]...)
			return nil
		}
		return fmt.Errorf("证书 %d 不存在", id)
	}); err != nil {
		return err
	}

	Runtime.Manager.Remove(id)

	// PEM、账户私钥一并清掉。path 来源指向用户自己的文件，不在这个目录下，不受影响。
	if err := os.RemoveAll(certDir(id)); err != nil {
		logrus.Warnf("[cert] 清理证书目录失败 [%d]：%v", id, err)
	}
	return nil
}

// UploadPEM 接收手动上传/粘贴的 PEM，校验后落盘并热加载
func UploadPEM(id uint, certPEM, keyPEM []byte) (model.Certificate, error) {
	c, ok := Runtime.Find(id)
	if !ok {
		return c, fmt.Errorf("证书 %d 不存在", id)
	}
	if c.Source != model.SourceUpload {
		return c, fmt.Errorf("只有「手动上传」来源的证书可以上传 PEM，当前来源为 %s", c.Source)
	}

	if !looksLikePEM(certPEM) {
		return c, errors.New("证书内容不是有效的 PEM 格式（应以 -----BEGIN CERTIFICATE----- 开头）")
	}
	if !looksLikePEM(keyPEM) {
		return c, errors.New("私钥内容不是有效的 PEM 格式")
	}
	leaf, err := ValidatePEM(certPEM, keyPEM)
	if err != nil {
		if t := pemKeyType(keyPEM); t != "" {
			return c, fmt.Errorf("%w（私钥类型：%s）", err, t)
		}
		return c, err
	}
	if time.Now().After(leaf.NotAfter) {
		return c, fmt.Errorf("这张证书已于 %s 过期", leaf.NotAfter.Format("2006-01-02"))
	}

	if err := writePEM(id, certPEM, keyPEM); err != nil {
		return c, err
	}

	if err := loadAndCommit(c); err != nil {
		return c, err
	}
	Runtime.Manager.SetEnabled(c.ID, c.Enabled, c.IsDefault)

	updated, _ := Runtime.Find(id)
	logCertInfo(updated, fmt.Sprintf("已上传，有效期至 %s", leaf.NotAfter.Format("2006-01-02")))
	return updated, nil
}

// IssueNow 手动触发一次签发/续期，异步执行，结果回写 LastError / NotAfter。
// 返回仅表示「已开始」，不等待签发完成——ACME 全流程可能要几分钟。
func IssueNow(id uint) error {
	c, ok := Runtime.Find(id)
	if !ok {
		return fmt.Errorf("证书 %d 不存在", id)
	}
	if !c.Enabled {
		return errors.New("证书已禁用，请先启用")
	}

	// 自签不走 CA，没有配额也没有退避，直接重签一张。
	// 换了网段、IP 变了之后要靠它把新 IP 写进 SAN。
	if c.Source == model.SourceSelfSigned {
		go func() {
			if err := GenerateSelfSigned(c); err != nil {
				logCertError(c, "重新生成自签证书失败", err)
			}
		}()
		return nil
	}

	if !model.IsACME(c.Source) {
		return fmt.Errorf("来源 %s 不支持自动签发", c.Source)
	}

	// 用户主动点的，清掉自动续期失败累积的退避
	if s := Runtime.Scheduler; s != nil {
		s.ClearBackoff(id)
	}

	go func() {
		if err := Issue(context.Background(), c); err != nil {
			logCertError(c, "手动签发失败", err)
			return
		}
		logCertInfo(c, "手动签发成功")
	}()
	return nil
}

// normalizeAndValidate 规整字段并做来源相关的必填校验
func normalizeAndValidate(c *model.Certificate) error {
	c.Name = strings.TrimSpace(c.Name)
	if c.Name == "" {
		return errors.New("证书名称不能为空")
	}
	if !model.IsValidSource(c.Source) {
		return fmt.Errorf("不支持的证书来源：%s", c.Source)
	}

	c.Domains = normalizeDomains(c.Domains)
	for _, d := range c.Domains {
		if err := validateDomain(d); err != nil {
			return err
		}
	}

	switch c.Source {
	case model.SourcePath:
		c.CertFile = strings.TrimSpace(c.CertFile)
		c.KeyFile = strings.TrimSpace(c.KeyFile)
		if c.CertFile == "" || c.KeyFile == "" {
			return errors.New("文件路径来源需要同时填写证书和私钥路径")
		}

	case model.SourceACMEDNS, model.SourceACMEHTTP:
		if len(c.Domains) == 0 {
			return errors.New("自动签发至少需要填写一个域名")
		}
		c.ACME.Email = strings.TrimSpace(c.ACME.Email)
		c.ACME.Directory = strings.TrimSpace(c.ACME.Directory)
		if c.ACME.RenewDays <= 0 {
			c.ACME.RenewDays = model.DefaultRenewDays
		}
		if c.Source == model.SourceACMEDNS {
			if c.ACME.ProviderID == 0 {
				return errors.New("DNS-01 需要选择一个 DNS 服务商")
			}
		} else {
			if c.ACME.HTTPPort == 0 {
				c.ACME.HTTPPort = model.DefaultHTTPPort
			}
			// 有通配符就直接拦下来，别等 CA 拒绝了再报一个看不懂的错
			for _, d := range c.Domains {
				if strings.HasPrefix(d, "*.") {
					return errors.New("HTTP-01 不支持通配符域名，通配符证书请改用 DNS-01")
				}
			}
		}
	}
	return nil
}

// validateDomain 只做基本形态校验，通配符仅允许出现在最左侧一层
func validateDomain(d string) error {
	name := d
	if strings.HasPrefix(name, "*.") {
		name = name[2:]
		if strings.Contains(name, "*") {
			return fmt.Errorf("域名 %s 不合法：通配符只能出现在最左侧一层", d)
		}
	} else if strings.Contains(name, "*") {
		return fmt.Errorf("域名 %s 不合法：通配符只能写成 *.example.com", d)
	}
	if name == "" || !strings.Contains(name, ".") || strings.HasPrefix(name, ".") || strings.HasSuffix(name, ".") {
		return fmt.Errorf("域名 %s 不合法", d)
	}
	if strings.ContainsAny(name, " /:\\") {
		return fmt.Errorf("域名 %s 不合法", d)
	}
	return nil
}

func checkNameConflict(cfg *model.CertConfig, name string, selfID uint) error {
	for _, e := range cfg.Certificates {
		if e.ID != selfID && strings.EqualFold(e.Name, name) {
			return fmt.Errorf("已存在同名证书：%s", name)
		}
	}
	return nil
}

// clearOtherDefaults 默认证书只能有一张
func clearOtherDefaults(cfg *model.CertConfig, keepID uint) {
	for i := range cfg.Certificates {
		if cfg.Certificates[i].ID != keepID {
			cfg.Certificates[i].IsDefault = false
		}
	}
}

// domainsChanged 比较域名集合是否变化。按集合比而非按顺序比——
// 仅仅调换顺序不该触发重签，那会白白消耗 CA 的签发配额。
func domainsChanged(a, b []string) bool {
	if len(a) != len(b) {
		return true
	}
	set := make(map[string]bool, len(a))
	for _, d := range a {
		set[d] = true
	}
	for _, d := range b {
		if !set[d] {
			return true
		}
	}
	return false
}

// triggerScan 让调度器立刻扫一遍（模块可能还没初始化完，Scheduler 为 nil 是正常的）
func triggerScan() {
	if s := Runtime.Scheduler; s != nil {
		s.Trigger()
	}
}
