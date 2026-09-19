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

// StartScheduler 首次建好调度器并跑起来。
//
// 和 RebuildScheduler 共用一把锁：STUN 拿到公网 IP 会从另一个 goroutine 调
// TriggerSync，和这里的赋值是并发的。
func (r *DDNSRuntime) StartScheduler() {
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

// noteSyncResult 把一次同步的结果告诉调度器，好决定下次重试的快慢。
// 「单条立即同步」走的不是队列，不主动说一声的话退避计数就一直停在那。
func (r *DDNSRuntime) noteSyncResult(id uint, ok bool) {
	schedMu.Lock()
	s := r.Scheduler
	schedMu.Unlock()
	if s != nil {
		s.noteResult(id, ok)
	}
}
