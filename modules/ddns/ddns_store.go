package ddns

import (
	"linkstar/modules/ddns/model"
	"sync"

	"github.com/sirupsen/logrus"
)

// schedMu 单独串行化调度器的启停/重建，避免和 config 写锁 mu 嵌套死锁
var schedMu sync.Mutex

// Snapshot 返回配置的深拷贝快照（providers / records 切片独立），可安全脱离锁使用
func (r *DDNSRuntime) Snapshot() model.DDNSConfig {
	r.mu.RLock()
	defer r.mu.RUnlock()
	cfg := r.Config
	cfg.Providers = append([]model.DDNSProvider(nil), r.Config.Providers...)
	cfg.Records = append([]model.DDNSRecord(nil), r.Config.Records...)
	return cfg
}

// Update 在写锁内执行 mutator 并持久化，mutator 返回错误时不落盘
func (r *DDNSRuntime) Update(fn func(cfg *model.DDNSConfig) error) error {
	r.mu.Lock()
	if err := fn(&r.Config); err != nil {
		r.mu.Unlock()
		return err
	}
	snapshot := r.Config
	r.mu.Unlock()
	return SaveConfig(snapshot)
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
	if err := SaveConfig(snapshot); err != nil {
		logrus.Warn("[ddns] 写回同步状态失败：", err)
	}
}
