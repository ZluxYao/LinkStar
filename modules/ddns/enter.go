package ddns

import (
	"linkstar/modules/ddns/model"
	"sync"
)

type DDNSRuntime struct {
	mu        sync.RWMutex
	Config    model.DDNSConfig
	Scheduler *Scheduler
}

var Runtime = &DDNSRuntime{}

// RebuildScheduler 在 providers 变更后调用：停掉旧调度器，按当前配置重建
func (r *DDNSRuntime) RebuildScheduler() {
	schedMu.Lock()
	defer schedMu.Unlock()

	if r.Scheduler != nil {
		r.Scheduler.Stop()
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
