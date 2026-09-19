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

// failRetryBase 上一次没同步成的记录，隔多久再试第一次。
//
// 之后每失败一次翻一倍（30s → 1min → 2min → 4min……），封顶在配置的正常间隔。
// 这么分两头是因为失败有两种：一种是「还没轮到」——最典型的是开机时 STUN 还没
// 拿到公网 IP，这时按默认间隔算就是干等 5 分钟，域名一直指着旧地址；另一种是
// 「一直就是错的」——token 填错、域名不在这个账号下，这种每 15s 试一遍等于拿
// 服务商的 API 当沙包打，很容易被限流。头几次退得快，越错越慢，两头都照顾到。
const failRetryBase = 30 * time.Second

// failRetryMaxShift 失败次数的记账上限，纯粹防止移位把时长算溢出成负数
const failRetryMaxShift = 16

// 调度器主体
type Scheduler struct {
	workers map[uint]*providerWorker // key = ProviderID
	event   chan struct{}            // 事件触发：丢个信号就强制扫一遍（IP 刚变，立即同步）

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	inflightMu sync.Mutex
	inflight   map[uint]bool // recordID -> 已投递、尚未提交结果，防止重复投递

	failMu   sync.Mutex
	failures map[uint]int // recordID -> 连续失败次数，只活在内存里，重建调度器就清零
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
		failures: make(map[uint]int),
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
			s.noteResult(rec.ID, rec.LastStatus != model.DDNSRecordStatusFailed)
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
	interval := normalInterval(cfg.IntervalSec)

	for i := range cfg.Records {
		r := cfg.Records[i] // 副本，worker 改的是副本，结果再 commit 回去

		if !r.Enabled {
			continue
		}
		if !force && !due(&r, s.retryAfter(r.ID, interval), now) {
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

// noteResult 记下这条记录这次跑成什么样，连续失败次数决定下次隔多久再试
func (s *Scheduler) noteResult(id uint, ok bool) {
	s.failMu.Lock()
	defer s.failMu.Unlock()
	if ok {
		delete(s.failures, id)
		return
	}
	if s.failures[id] < failRetryMaxShift {
		s.failures[id]++
	}
}

// retryAfter 这条记录这次该等多久再跑：一直是好的就按正常间隔，
// 上次没成就按退避提前，但最长不超过正常间隔
func (s *Scheduler) retryAfter(id uint, interval time.Duration) time.Duration {
	s.failMu.Lock()
	n := s.failures[id]
	s.failMu.Unlock()

	if n <= 0 {
		return interval
	}
	wait := failRetryBase << (n - 1)
	if wait > interval {
		return interval
	}
	return wait
}

// normalInterval 配置里的同步间隔，没填按 5 分钟
func normalInterval(sec int) time.Duration {
	if sec <= 0 {
		sec = 300
	}
	return time.Duration(sec) * time.Second
}

// due 判断这条记录是否到期：距上一次尝试已经过了 wait
func due(r *model.DDNSRecord, wait time.Duration, now time.Time) bool {
	return now.Sub(r.LastCheckAt) >= wait
}
