package cert

import (
	"linkstar/modules/cert/model"

	"github.com/sirupsen/logrus"
)

// Snapshot 返回配置的深拷贝快照，可安全脱离锁使用
func (r *CertRuntime) Snapshot() model.CertConfig {
	r.mu.RLock()
	defer r.mu.RUnlock()
	cfg := r.Config
	cfg.Certificates = append([]model.Certificate(nil), r.Config.Certificates...)
	return cfg
}

// Find 按 ID 取一份证书配置的拷贝
func (r *CertRuntime) Find(id uint) (model.Certificate, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, c := range r.Config.Certificates {
		if c.ID == id {
			return c, true
		}
	}
	return model.Certificate{}, false
}

// Update 在写锁内执行 mutator 并持久化，mutator 返回错误时不落盘
func (r *CertRuntime) Update(fn func(cfg *model.CertConfig) error) error {
	r.mu.Lock()
	if err := fn(&r.Config); err != nil {
		r.mu.Unlock()
		return err
	}
	snapshot := r.Config
	r.mu.Unlock()
	return SaveConfig(snapshot)
}

// commitStatus 把加载/签发后的运行态字段按 ID 写回实时配置并持久化
func (r *CertRuntime) commitStatus(c model.Certificate) {
	r.mu.Lock()
	for i := range r.Config.Certificates {
		if r.Config.Certificates[i].ID != c.ID {
			continue
		}
		t := &r.Config.Certificates[i]
		t.NotBefore = c.NotBefore
		t.NotAfter = c.NotAfter
		t.Issuer = c.Issuer
		t.LastError = c.LastError
		t.LastIssue = c.LastIssue
		// ACME 续期会重算域名，以签发结果为准
		if len(c.Domains) > 0 {
			t.Domains = append([]string(nil), c.Domains...)
		}
		break
	}
	snapshot := r.Config
	r.mu.Unlock()

	if err := SaveConfig(snapshot); err != nil {
		logrus.Warn("[cert] 写回证书状态失败：", err)
	}
}

// nextID 返回下一个可用的证书 ID
func nextID(cfg *model.CertConfig) uint {
	var max uint
	for _, c := range cfg.Certificates {
		if c.ID > max {
			max = c.ID
		}
	}
	return max + 1
}
