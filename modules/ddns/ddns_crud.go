package ddns

import (
	"fmt"
	"linkstar/modules/ddns/model"
	"strings"
	"time"
)

// RecordInput 记录可编辑字段（不含 ID 与 Last*/时间戳），add / update 共用
type RecordInput struct {
	Enabled      bool
	ProviderID   uint
	Name         string
	Domain       string
	SubDomain    string
	RecordType   model.DNSRecordType
	IPSourceType model.IPSourceType
	IPSourceArg  string
	TTL          int
	Proxied      bool
}

// ============ Provider 增删改 =============

// AddProvider 校验后追加一个服务商，落盘并重建调度器
func (r *DDNSRuntime) AddProvider(name string, ptype model.DNSProviderType, credential map[string]string) (model.DDNSProvider, error) {
	if name == "" {
		return model.DDNSProvider{}, fmt.Errorf("服务商名称不能为空")
	}
	if !model.IsValidProviderType(ptype) {
		return model.DDNSProvider{}, fmt.Errorf("暂不支持的服务商类型")
	}
	cred, err := buildCredential(ptype, credential, nil)
	if err != nil {
		return model.DDNSProvider{}, err
	}

	var created model.DDNSProvider
	if err := r.Update(func(cfg *model.DDNSConfig) error {
		now := time.Now()
		created = model.DDNSProvider{
			ID:         nextProviderID(cfg),
			Name:       name,
			Type:       ptype,
			Credential: cred,
			CreatedAt:  now,
			UpdatedAt:  now,
		}
		cfg.Providers = append(cfg.Providers, created)
		return nil
	}); err != nil {
		return model.DDNSProvider{}, err
	}

	r.RebuildScheduler()
	return created, nil
}

// UpdateProvider 改名 / 可选改凭证；仅凭证变更时才重建调度器。
// credential 中为空串的字段视为“不修改”，沿用原值。
func (r *DDNSRuntime) UpdateProvider(id uint, name string, credential map[string]string) error {
	if name == "" {
		return fmt.Errorf("服务商名称不能为空")
	}

	found := false
	credChanged := false
	var buildErr error
	if err := r.Update(func(cfg *model.DDNSConfig) error {
		for i := range cfg.Providers {
			if cfg.Providers[i].ID != id {
				continue
			}
			p := &cfg.Providers[i]
			p.Name = name

			// 有任意非空凭证字段才重建凭证（基于旧值补齐留空字段）
			if hasAnyValue(credential) {
				cred, err := buildCredential(p.Type, credential, p.Credential)
				if err != nil {
					buildErr = err
					return nil
				}
				p.Credential = cred
				credChanged = true
			}
			p.UpdatedAt = time.Now()
			found = true
			break
		}
		return nil
	}); err != nil {
		return err
	}

	if buildErr != nil {
		return buildErr
	}
	if !found {
		return fmt.Errorf("服务商不存在")
	}
	if credChanged {
		r.RebuildScheduler()
	}
	return nil
}

// DeleteProvider 删除服务商；仍被记录引用时拒绝，成功后重建调度器
func (r *DDNSRuntime) DeleteProvider(id uint) error {
	found := false
	if err := r.Update(func(cfg *model.DDNSConfig) error {
		var inUse int
		for _, rec := range cfg.Records {
			if rec.ProviderID == id {
				inUse++
			}
		}
		if inUse > 0 {
			return fmt.Errorf("该服务商下还有 %d 条解析记录，请先删除记录", inUse)
		}

		idx := -1
		for i := range cfg.Providers {
			if cfg.Providers[i].ID == id {
				idx = i
				break
			}
		}
		if idx == -1 {
			return nil
		}
		found = true
		cfg.Providers = append(cfg.Providers[:idx], cfg.Providers[idx+1:]...)
		return nil
	}); err != nil {
		return err
	}

	if !found {
		return fmt.Errorf("服务商不存在")
	}
	r.RebuildScheduler()
	return nil
}

// ============ Record 增删改 =============

// AddRecord 校验后追加一条记录，落盘并触发一次同步
func (r *DDNSRuntime) AddRecord(in RecordInput) (model.DDNSRecord, error) {
	if msg := validateRecord(in.Name, in.Domain, in.RecordType, in.IPSourceType); msg != "" {
		return model.DDNSRecord{}, fmt.Errorf("%s", msg)
	}

	var created model.DDNSRecord
	providerOK := false
	if err := r.Update(func(cfg *model.DDNSConfig) error {
		for _, p := range cfg.Providers {
			if p.ID == in.ProviderID {
				providerOK = true
				break
			}
		}
		if !providerOK {
			return nil
		}

		now := time.Now()
		created = model.DDNSRecord{
			Enabled:      in.Enabled,
			ID:           nextRecordID(cfg),
			ProviderID:   in.ProviderID,
			Name:         in.Name,
			Domain:       in.Domain,
			SubDomain:    in.SubDomain,
			RecordType:   in.RecordType,
			IPSourceType: in.IPSourceType,
			IPSourceArg:  in.IPSourceArg,
			TTL:          in.TTL,
			Proxied:      in.Proxied,
			LastStatus:   model.DDNSRecordStatusPending,
			CreatedAt:    now,
			UpdatedAt:    now,
		}
		cfg.Records = append(cfg.Records, created)
		return nil
	}); err != nil {
		return model.DDNSRecord{}, err
	}

	if !providerOK {
		return model.DDNSRecord{}, fmt.Errorf("所选服务商不存在")
	}
	r.TriggerSync()
	return created, nil
}

// UpdateRecord 校验后覆盖记录字段；目标地址类字段变更时清空 LastIP 触发重推
func (r *DDNSRuntime) UpdateRecord(id uint, in RecordInput) error {
	if msg := validateRecord(in.Name, in.Domain, in.RecordType, in.IPSourceType); msg != "" {
		return fmt.Errorf("%s", msg)
	}

	providerOK := false
	found := false
	if err := r.Update(func(cfg *model.DDNSConfig) error {
		for _, p := range cfg.Providers {
			if p.ID == in.ProviderID {
				providerOK = true
				break
			}
		}
		if !providerOK {
			return nil
		}

		for i := range cfg.Records {
			if cfg.Records[i].ID != id {
				continue
			}
			rec := &cfg.Records[i]

			// 影响最终解析结果的字段变了，清空 LastIP 触发重推
			if rec.Domain != in.Domain || rec.SubDomain != in.SubDomain ||
				rec.RecordType != in.RecordType || rec.IPSourceType != in.IPSourceType ||
				rec.IPSourceArg != in.IPSourceArg || rec.ProviderID != in.ProviderID ||
				rec.TTL != in.TTL || rec.Proxied != in.Proxied {
				rec.LastIP = ""
				rec.LastStatus = model.DDNSRecordStatusPending
				rec.LastCheckAt = time.Time{}
			}

			rec.Enabled = in.Enabled
			rec.ProviderID = in.ProviderID
			rec.Name = in.Name
			rec.Domain = in.Domain
			rec.SubDomain = in.SubDomain
			rec.RecordType = in.RecordType
			rec.IPSourceType = in.IPSourceType
			rec.IPSourceArg = in.IPSourceArg
			rec.TTL = in.TTL
			rec.Proxied = in.Proxied
			rec.UpdatedAt = time.Now()
			found = true
			break
		}
		return nil
	}); err != nil {
		return err
	}

	if !providerOK {
		return fmt.Errorf("所选服务商不存在")
	}
	if !found {
		return fmt.Errorf("记录不存在")
	}
	r.TriggerSync()
	return nil
}

// DeleteRecord 按 ID 删除记录（无副作用）
func (r *DDNSRuntime) DeleteRecord(id uint) error {
	found := false
	if err := r.Update(func(cfg *model.DDNSConfig) error {
		idx := -1
		for i := range cfg.Records {
			if cfg.Records[i].ID == id {
				idx = i
				break
			}
		}
		if idx == -1 {
			return nil
		}
		found = true
		cfg.Records = append(cfg.Records[:idx], cfg.Records[idx+1:]...)
		return nil
	}); err != nil {
		return err
	}

	if !found {
		return fmt.Errorf("记录不存在")
	}
	return nil
}

// ============ 设置 =============

// UpdateSettings 改全局同步间隔
func (r *DDNSRuntime) UpdateSettings(intervalSec int) error {
	if intervalSec < 30 {
		return fmt.Errorf("同步间隔不能小于 30 秒")
	}

	return r.Update(func(cfg *model.DDNSConfig) error {
		cfg.IntervalSec = intervalSec
		return nil
	})
}

// ============ 辅助 =============

// buildCredential 按服务商类型所需字段构建凭证 map。
// in 为本次提交的字段值，old 为原凭证（更新时用于补齐留空字段，新增时传 nil）。
// 任一必填字段最终为空则报错。
func buildCredential(ptype model.DNSProviderType, in map[string]string, old map[string]interface{}) (map[string]interface{}, error) {
	keys := model.ProviderCredentialKeys[ptype]
	cred := map[string]interface{}{}
	for _, k := range keys {
		v := strings.TrimSpace(in[k])
		if v == "" && old != nil {
			if ov, ok := old[k].(string); ok {
				v = ov
			}
		}
		if v == "" {
			return nil, fmt.Errorf("凭证字段 %s 不能为空", k)
		}
		cred[k] = v
	}
	return cred, nil
}

// hasAnyValue 是否存在任意非空字段
func hasAnyValue(m map[string]string) bool {
	for _, v := range m {
		if strings.TrimSpace(v) != "" {
			return true
		}
	}
	return false
}

// validateRecord 公共字段校验，返回空串表示通过
func validateRecord(name, domain string, recordType model.DNSRecordType, ipSource model.IPSourceType) string {
	if name == "" {
		return "记录名称不能为空"
	}
	if domain == "" {
		return "主域名不能为空"
	}
	if recordType != model.DNSRecordTypeA && recordType != model.DNSRecordTypeAAAA {
		return "记录类型仅支持 A / AAAA"
	}
	switch ipSource {
	case model.IPSourceSTUN, model.IPSourceWeb, model.IPSourceDNS, model.IPSourceInterface:
	default:
		return "无效的 IP 来源类型"
	}
	return ""
}

// nextProviderID / nextRecordID 取当前最大 ID + 1（在 Update 写锁内调用）
func nextProviderID(cfg *model.DDNSConfig) uint {
	var max uint
	for _, p := range cfg.Providers {
		if p.ID > max {
			max = p.ID
		}
	}
	return max + 1
}

func nextRecordID(cfg *model.DDNSConfig) uint {
	var max uint
	for _, r := range cfg.Records {
		if r.ID > max {
			max = r.ID
		}
	}
	return max + 1
}
