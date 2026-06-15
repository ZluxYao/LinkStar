package ddns

import (
	"context"
	"linkstar/modules/ddns/dns"
	"linkstar/modules/ddns/model"
	"time"

	"github.com/sirupsen/logrus"
)

// 扫描粒度：每 15s 扫一遍所有记录，看谁到期
const scanInterval = 15 * time.Second

type Scheduler struct {
	workers map[uint]*providerWorker // key = ProviderID
	event   chan struct{}            // 等事件触发，丢个信号就强制扫,强制同步更新
}

// providerWorker = 一个服务商实例 + 它专属的一个队列 + 一个常驻 goroutine
type providerWorker struct {
	client dns.DNSProvider
	queue  chan *model.DDNSRecord
}

// run 常驻 goroutine： 队列里面又记录就消费一条
func (w *providerWorker) run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case r := <-w.queue:
			SyncRecord(w.client, r)
		}
	}
}

// NewScheduler 启动时遍历 Providers，每个建一个实例 + 队列 + worker
func NewScheduler(ctx context.Context, cfg *model.DDNSConfig) *Scheduler {
	// 创建DNS服务商
	workers := make(map[uint]*providerWorker)
	for _, p := range cfg.Providers {
		client := dns.BuildClient(p)
		if client == nil {
			logrus.Warnf("[ddns] 不支持或配置无效的服务商: %s (id=%d)", p.Type, p.ID)
			continue
		}

		w := &providerWorker{
			client: client,
			queue:  make(chan *model.DDNSRecord, 100),
		}

		workers[p.ID] = w

		go w.run(ctx) // worker 启动，阻塞在队列上等任务

	}

	return &Scheduler{
		workers: workers,
		event:   make(chan struct{}, 1),
	}
}

func (s *Scheduler) Trigger() {
	select {
	case s.event <- struct{}{}:
	default:
	}
}

func (s *Scheduler) Run(ctx context.Context, cfg *model.DDNSConfig) {
	ticker := time.NewTicker(scanInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.scan(cfg, false) //定时：只同步到期的
		case <-s.event:
			s.scan(cfg, true) // 事件：强制扫一遍（IP 刚变，立即同步）
		}

	}
}

// scan 遍历所有记录，把到期的丢进对应服务商的队列
func (s *Scheduler) scan(cfg *model.DDNSConfig, force bool) {
	now := time.Now()

	for i := range cfg.Records {

		// 跳过未开启
		r := &cfg.Records[i]
		if !r.Enabled {
			continue
		}

		// 如果不是强扫或者未到事件跳过
		if !force && !due(r, cfg.IntervalSec, now) {
			continue
		}

		w := s.workers[r.ProviderID]
		if w == nil {
			continue // 记录指向的服务商不存在
		}

		// 非阻塞投递，满了下一次
		select {
		case w.queue <- r:
		default:
			logrus.Warnf("DDNS 提供商队列满了下次在投递: providerID=%d recordID=%d", r.ProviderID, r.ID)
		}

	}
}

// due 判断这条记录是否到期：now - LastCheckAt >= 间隔
func due(r *model.DDNSRecord, interval int, now time.Time) bool {
	if interval <= 0 {
		interval = 300
	}
	return now.Sub(r.LastCheckAt) >= time.Duration(interval)*time.Second
}
