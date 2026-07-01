package webhook

import (
	"fmt"
	"time"
)

// Snapshot 返回 webhook 配置快照，Templates 切片独立，可安全脱离锁使用
func (r *WebhookRuntime) Snapshot() Config {
	r.mu.RLock()
	defer r.mu.RUnlock()
	cfg := r.Config
	cfg.Templates = append([]WebhookTemplate(nil), r.Config.Templates...)
	return cfg
}

// Update 在写锁内修改配置并持久化，mutator 返回错误时不落盘
func (r *WebhookRuntime) Update(mutator func(cfg *Config) error) error {
	r.mu.Lock()
	if err := mutator(&r.Config); err != nil {
		r.mu.Unlock()
		return err
	}
	snapshot := r.Config
	r.mu.Unlock()
	return SaveConfig(snapshot)
}

// ListTemplates 返回全部 webhook 模板
func (r *WebhookRuntime) ListTemplates() []WebhookTemplate {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return append([]WebhookTemplate(nil), r.Config.Templates...)
}

// GetTemplate 按 ID 获取一个 webhook 模板
func (r *WebhookRuntime) GetTemplate(id string) (WebhookTemplate, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, t := range r.Config.Templates {
		if t.ID == id {
			return t, true
		}
	}
	return WebhookTemplate{}, false
}

// AddTemplate 校验后新增一个自定义模板并持久化
func (r *WebhookRuntime) AddTemplate(in WebhookTemplate) (WebhookTemplate, error) {
	if in.Name == "" {
		return WebhookTemplate{}, fmt.Errorf("模板名称不能为空")
	}
	if in.ID == "" {
		in.ID = fmt.Sprintf("custom-%d", time.Now().UnixNano())
	}
	now := time.Now()
	in.CreatedAt = now
	in.UpdatedAt = now
	in.Builtin = false

	var created WebhookTemplate
	err := r.Update(func(cfg *Config) error {
		for _, t := range cfg.Templates {
			if t.ID == in.ID {
				return fmt.Errorf("模板ID已存在")
			}
		}
		created = in
		cfg.Templates = append(cfg.Templates, created)
		return nil
	})
	return created, err
}

// UpdateTemplate 按 ID 更新自定义模板，内置模板不允许修改
func (r *WebhookRuntime) UpdateTemplate(id string, patch WebhookTemplate) (WebhookTemplate, error) {
	if id == "" {
		return WebhookTemplate{}, fmt.Errorf("模板ID不能为空")
	}
	var updated WebhookTemplate
	err := r.Update(func(cfg *Config) error {
		for i := range cfg.Templates {
			if cfg.Templates[i].ID != id {
				continue
			}
			if cfg.Templates[i].Builtin {
				return fmt.Errorf("内置模板不允许修改")
			}
			if patch.Name != "" {
				cfg.Templates[i].Name = patch.Name
			}
			cfg.Templates[i].Description = patch.Description
			cfg.Templates[i].Config = patch.Config
			cfg.Templates[i].UpdatedAt = time.Now()
			updated = cfg.Templates[i]
			return nil
		}
		return fmt.Errorf("模板不存在")
	})
	return updated, err
}

// DeleteTemplate 按 ID 删除自定义模板，内置模板不允许删除
func (r *WebhookRuntime) DeleteTemplate(id string) error {
	return r.Update(func(cfg *Config) error {
		idx := -1
		for i := range cfg.Templates {
			if cfg.Templates[i].ID == id {
				if cfg.Templates[i].Builtin {
					return fmt.Errorf("内置模板不允许删除")
				}
				idx = i
				break
			}
		}
		if idx == -1 {
			return fmt.Errorf("模板不存在")
		}
		cfg.Templates = append(cfg.Templates[:idx], cfg.Templates[idx+1:]...)
		return nil
	})
}
