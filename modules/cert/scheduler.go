package cert

import (
	"context"
	"sync"
	"time"

	"linkstar/modules/cert/model"

	"github.com/sirupsen/logrus"
)

// 扫描粒度：每 60s 扫一遍所有证书（路径 mtime 变化 + ACME 到期续期）
const scanInterval = 60 * time.Second

// 续期失败后的退避区间，避免反复失败把 CA 的限流打满
const (
	minBackoff = 5 * time.Minute
	maxBackoff = 6 * time.Hour
)

type Scheduler struct {
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
	event  chan struct{} // 手动触发一次扫描

	mu       sync.Mutex
	nextTry  map[uint]time.Time // certID -> 下次允许续期的时间
	backoff  map[uint]time.Duration
	inflight map[uint]bool // 正在签发，防止重复触发
}

func NewScheduler() *Scheduler {
	ctx, cancel := context.WithCancel(context.Background())
	return &Scheduler{
		ctx:      ctx,
		cancel:   cancel,
		event:    make(chan struct{}, 1),
		nextTry:  make(map[uint]time.Time),
		backoff:  make(map[uint]time.Duration),
		inflight: make(map[uint]bool),
	}
}

func (s *Scheduler) Start() {
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		ticker := time.NewTicker(scanInterval)
		defer ticker.Stop()

		for {
			select {
			case <-s.ctx.Done():
				return
			case <-ticker.C:
				s.scan()
			case <-s.event:
				s.scan()
			}
		}
	}()
}

func (s *Scheduler) Stop() {
	s.cancel()
	s.wg.Wait()
}

// Trigger 立即扫描一次（配置变更后调用）
func (s *Scheduler) Trigger() {
	select {
	case s.event <- struct{}{}:
	default:
	}
}

func (s *Scheduler) scan() {
	for _, c := range Runtime.Snapshot().Certificates {
		if !c.Enabled {
			continue
		}
		select {
		case <-s.ctx.Done():
			return
		default:
		}

		switch c.Source {
		case model.SourcePath:
			checkPathReload(c)
		case model.SourceACMEDNS, model.SourceACMEHTTP:
			s.maybeRenew(c)
		case model.SourceSelfSigned:
			ensureSelfSigned(c)
		case model.SourceUpload:
			// 纯手动，调度器不介入
		}
	}
}

// maybeRenew 到期前 RenewDays 天触发续期
func (s *Scheduler) maybeRenew(c model.Certificate) {
	if !s.due(c) {
		return
	}

	s.mu.Lock()
	if s.inflight[c.ID] {
		s.mu.Unlock()
		return
	}
	s.inflight[c.ID] = true
	s.mu.Unlock()

	go func() {
		defer func() {
			s.mu.Lock()
			delete(s.inflight, c.ID)
			s.mu.Unlock()
		}()

		if err := Issue(s.ctx, c); err != nil {
			s.penalize(c.ID)
			logCertError(c, "自动续期失败", err)
			return
		}
		s.reset(c.ID)
	}()
}

// due 判断这张证书现在该不该续期
func (s *Scheduler) due(c model.Certificate) bool {
	s.mu.Lock()
	next, hasBackoff := s.nextTry[c.ID]
	s.mu.Unlock()
	if hasBackoff && time.Now().Before(next) {
		return false // 处在退避窗口内
	}

	// 还没签过（NotAfter 为零值）→ 立即签
	if c.NotAfter.IsZero() {
		return true
	}
	threshold := time.Duration(c.ACME.RenewThreshold()) * 24 * time.Hour
	return time.Until(c.NotAfter) < threshold
}

// penalize 续期失败，指数退避
func (s *Scheduler) penalize(id uint) {
	s.mu.Lock()
	defer s.mu.Unlock()

	d := s.backoff[id]
	if d == 0 {
		d = minBackoff
	} else {
		d *= 2
	}
	if d > maxBackoff {
		d = maxBackoff
	}
	s.backoff[id] = d
	s.nextTry[id] = time.Now().Add(d)
	logrus.Warnf("[cert] 证书 %d 续期失败，%s 后重试", id, d)
}

func (s *Scheduler) reset(id uint) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.backoff, id)
	delete(s.nextTry, id)
}

// ClearBackoff 手动点「立即签发」时清掉退避，让用户的操作立刻生效
func (s *Scheduler) ClearBackoff(id uint) {
	s.reset(id)
}
