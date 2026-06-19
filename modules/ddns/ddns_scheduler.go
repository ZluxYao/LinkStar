package ddns

import (
	"context"
	"linkstar/modules/ddns/dns"
	"linkstar/modules/ddns/model"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

// 扫描粒度：每 15s 扫一遍所有记录，看谁到期
const scanInterval = 15 * time.Second

// 调度器主体
type Scheduler struct {
	workers map[uint]*providerWorker // key = ProviderID
	event   chan struct{}            // 事件触发：丢个信号就强制扫一遍（IP 刚变，立即同步）

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	inflightMu sync.Mutex
	inflight   map[uint]bool // recordID -> 已投递、尚未提交结果，防止重复投递
}

// providerWorker = 一个服务商实例 + 它专属的一个队列 + 一个常驻 goroutine
type providerWorker struct {
	client dns.DNSProvider
	queue  chan model.DDNSRecord
}

// NewScheduler 读取当前 providers 快照，每个服务商建一个实例 + 队列 + worker
func NewScheduler() *Scheduler {
	providers := Runtime.Snapshot().Providers

	workers := make(map[uint]*providerWorker)
	for _, p := range providers {
		client := dns.BuildClient(p)
		if client == nil {
			logrus.Warnf("[ddns] 不支持或配置无效的服务商: %s (id=%d)", p.Type, p.ID)
			continue
		}
		workers[p.ID] = &providerWorker{
			client: client,
			queue:  make(chan model.DDNSRecord, 100),
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	return &Scheduler{
		workers:  workers,
		event:    make(chan struct{}, 1),
		ctx:      ctx,
		cancel:   cancel,
		inflight: make(map[uint]bool),
	}
}

// Start 启动所有 worker goroutine 和扫描循环
func (s *Scheduler) Start() {
	for _, w := range s.workers {
		s.wg.Add(1)
		go s.runWorker(w)
	}
	s.wg.Add(1)
	go s.loop()
}

// Stop 取消 ctx 并等待所有 goroutine 退出
func (s *Scheduler) Stop() {
	s.cancel()
	s.wg.Wait()
}

// Trigger 投递一个事件，强制扫一遍（非阻塞）
func (s *Scheduler) Trigger() {
	select {
	case s.event <- struct{}{}:
	default:
	}
}

// runWorker 常驻 goroutine：队列里有记录就消费一条，同步后写回状态
func (s *Scheduler) runWorker(w *providerWorker) {
	defer s.wg.Done()
	for {
		select {
		case <-s.ctx.Done():
			return
		case rec := <-w.queue:
			SyncRecord(w.client, &rec)
			Runtime.commitRecord(&rec)
			s.clearInflight(rec.ID)
		}
	}
}

func (s *Scheduler) loop() {
	defer s.wg.Done()
	ticker := time.NewTicker(scanInterval)
	defer ticker.Stop()

	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			s.scan(false) // 定时：只同步到期的
		case <-s.event:
			s.scan(true) // 事件：强制扫一遍
		}
	}
}

// scan 从 Runtime 取配置快照，把到期（或强扫）的记录投递到对应服务商的队列
func (s *Scheduler) scan(force bool) {
	cfg := Runtime.Snapshot()
	now := time.Now()

	for i := range cfg.Records {
		r := cfg.Records[i] // 副本，worker 改的是副本，结果再 commit 回去

		if !r.Enabled {
			continue
		}
		if !force && !due(&r, cfg.IntervalSec, now) {
			continue
		}

		w := s.workers[r.ProviderID]
		if w == nil {
			continue // 记录指向的服务商不存在
		}

		// 已在途就跳过，避免重复投递同一条
		if !s.markInflight(r.ID) {
			continue
		}

		// 非阻塞投递，满了清掉在途标记，下一轮 due 再投
		select {
		case w.queue <- r:
		default:
			s.clearInflight(r.ID)
			logrus.Warnf("[ddns] 服务商队列已满，下次再投递: providerID=%d recordID=%d", r.ProviderID, r.ID)
		}
	}
}

// markInflight 标记记录在途，返回 false 表示已在途（应跳过）
func (s *Scheduler) markInflight(id uint) bool {
	s.inflightMu.Lock()
	defer s.inflightMu.Unlock()
	if s.inflight[id] {
		return false
	}
	s.inflight[id] = true
	return true
}

func (s *Scheduler) clearInflight(id uint) {
	s.inflightMu.Lock()
	delete(s.inflight, id)
	s.inflightMu.Unlock()
}

// due 判断这条记录是否到期：now - LastCheckAt >= 间隔
func due(r *model.DDNSRecord, interval int, now time.Time) bool {
	if interval <= 0 {
		interval = 300
	}
	return now.Sub(r.LastCheckAt) >= time.Duration(interval)*time.Second
}
