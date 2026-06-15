package ddns

import (
	"fmt"
	"linkstar/modules/ddns/dns"
	"linkstar/modules/ddns/model"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

// schedMu 单独串行化调度器的启停/重建，避免和 config 写锁 mu 嵌套死锁
var schedMu sync.Mutex

// View 在读锁内读取配置（生成 API 响应用），reader 收到的是实时引用，只读不要改
func (r *DDNSRuntime) View(reader func(cfg *model.DDNSConfig)) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	reader(&r.Config)
}

// Snapshot 返回配置的深拷贝快照（records / providers 切片独立），可安全脱离锁使用
func (r *DDNSRuntime) Snapshot() model.DDNSConfig {
	r.mu.RLock()
	defer r.mu.RUnlock()
	cfg := r.Config
	cfg.Providers = append([]model.DDNSProvider(nil), r.Config.Providers...)
	cfg.Records = append([]model.DDNSRecord(nil), r.Config.Records...)
	return cfg
}

// Apply 在写锁内执行 mutator 并持久化，mutator 返回错误时不落盘
func (r *DDNSRuntime) Apply(mutator func(cfg *model.DDNSConfig) error) error {
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

// commitRecord 把一条记录同步后的 Last* 字段按 ID 写回实时配置并持久化
func (r *DDNSRuntime) commitRecord(rec *model.DDNSRecord) {
	r.mu.Lock()
	for i := range r.Config.Records {
		if r.Config.Records[i].ID != rec.ID {
			continue
		}
		c := &r.Config.Records[i]
		c.LastIP = rec.LastIP
		c.LastStatus = rec.LastStatus
		c.LastMessage = rec.LastMessage
		c.LastCheckAt = rec.LastCheckAt
		c.LastSyncAt = rec.LastSyncAt
		break
	}
	snapshot := r.Config
	r.mu.Unlock()
	if err := UpdateConfig(snapshot); err != nil {
		logrus.Warn("[ddns] 写回同步状态失败：", err)
	}
}

// NextProviderID / NextRecordID 取当前最大 ID + 1（在 Apply 写锁内调用）
func NextProviderID(cfg *model.DDNSConfig) uint {
	var max uint
	for _, p := range cfg.Providers {
		if p.ID > max {
			max = p.ID
		}
	}
	return max + 1
}

func NextRecordID(cfg *model.DDNSConfig) uint {
	var max uint
	for _, r := range cfg.Records {
		if r.ID > max {
			max = r.ID
		}
	}
	return max + 1
}

// RebuildScheduler 在 providers 变更或总开关切换后调用：停掉旧调度器，按当前配置重建
func (r *DDNSRuntime) RebuildScheduler() {
	schedMu.Lock()
	defer schedMu.Unlock()

	if r.Scheduler != nil {
		r.Scheduler.Stop()
		r.Scheduler = nil
	}

	r.mu.RLock()
	enabled := r.Config.Enabled
	r.mu.RUnlock()
	if !enabled {
		logrus.Info("[ddns] 总开关关闭，调度器不启动")
		return
	}

	s := NewScheduler()
	r.Scheduler = s
	s.Start()
	s.Trigger()
}

// TriggerSync 触发一次强制全量扫描（“立即同步全部”按钮用）
func (r *DDNSRuntime) TriggerSync() {
	schedMu.Lock()
	s := r.Scheduler
	schedMu.Unlock()
	if s != nil {
		s.Trigger()
	}
}

// SyncRecordNow 同步指定记录（“单条立即同步”按钮用），同步执行以便返回即时结果
func (r *DDNSRuntime) SyncRecordNow(id uint) error {
	r.mu.RLock()
	var rec model.DDNSRecord
	var provider model.DDNSProvider
	found := false
	for i := range r.Config.Records {
		if r.Config.Records[i].ID == id {
			rec = r.Config.Records[i]
			for _, p := range r.Config.Providers {
				if p.ID == rec.ProviderID {
					provider = p
					break
				}
			}
			found = true
			break
		}
	}
	r.mu.RUnlock()

	if !found {
		return fmt.Errorf("记录不存在")
	}
	if !rec.Enabled {
		return fmt.Errorf("该记录已停用，请先启用")
	}

	client := dns.BuildClient(provider)
	if client == nil {
		return fmt.Errorf("记录所属服务商无效或未配置")
	}

	SyncRecord(client, &rec)
	r.commitRecord(&rec)

	if rec.LastStatus == model.DDNSRecordStatusFailed {
		return fmt.Errorf("%s", rec.LastMessage)
	}
	return nil
}
