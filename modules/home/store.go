package home

import (
	"errors"
	"time"
)

// WithLock 在写锁内执行 mutator，返回错误时不持久化
func (r *HomeRuntime) WithLock(mutator func(cfg *Config) error) error {
	r.mu.Lock()
	if err := mutator(&r.Config); err != nil {
		r.mu.Unlock()
		return err
	}
	r.Config.UpdatedAt = time.Now()
	snapshot := r.Config
	r.mu.Unlock()
	return UpdateConfig(snapshot)
}

// Read 在读锁内执行 reader，常用于生成响应
func (r *HomeRuntime) Read(reader func(cfg *Config)) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	reader(&r.Config)
}

// RemoveAppByStunService 删除某个 stun service 对应的 home app（hook 用）
func (r *HomeRuntime) RemoveAppByStunService(deviceID, serviceID uint) {
	_ = r.WithLock(func(cfg *Config) error {
		out := cfg.Apps[:0]
		for _, app := range cfg.Apps {
			if app.Type == "stun" && app.StunDeviceID == deviceID && app.StunServiceID == serviceID {
				continue
			}
			out = append(out, app)
		}
		cfg.Apps = out
		return nil
	})
}

// HasStunApp 检查是否存在指向该 stun service 的 home app
func (r *HomeRuntime) HasStunApp(deviceID, serviceID uint) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, app := range r.Config.Apps {
		if app.Type == "stun" && app.StunDeviceID == deviceID && app.StunServiceID == serviceID {
			return true
		}
	}
	return false
}

var (
	ErrNotFound = errors.New("not found")
	ErrConflict = errors.New("conflict")
)
