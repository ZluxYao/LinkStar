package stun

import (
	"context"
	"fmt"
	"linkstar/modules/stun/model"
	"sync"
	"time"
)

// EventKind 区分阶段变化和纯日志，订阅者可选择性处理
type EventKind int

const (
	EventPhaseChanged EventKind = iota
	EventLogAppended
)

// StateEvent 对外的数据
type StateEvent struct {
	Kind         EventKind    `json:"kind"`         // 事件种类
	Key          string       `json:"key"`          // 唯一键，格式 "deviceID-serviceID"
	DeviceName   string       `json:"deviceName"`   // 所属设备名称
	ServiceName  string       `json:"serviceName"`  // 服务名称
	Phase        ServicePhase `json:"phase"`        // 当前阶段（枚举值）
	PhaseStr     string       `json:"phaseStr"`     // 当前阶段（可读字符串）
	ExternalPort uint16       `json:"externalPort"` // STUN 映射后的外部端口，0 表示尚未获取
	RestartCount int          `json:"restartCount"` // 累计重启次数
	LastError    string       `json:"lastError"`    // 最近一次错误描述
	Logs         []ServiceLog `json:"logs"`         // 最近的服务日志（最多 maxServiceLogs 条）
	UpdatedAt    time.Time    `json:"updatedAt"`    // 最后一次状态更新时间
}

const maxServiceLogs = 15

// ServiceLog 每个服务日志
type ServiceLog struct {
	Message   string       `json:"message"`   // 日志内容
	Phase     ServicePhase `json:"phase"`     // 产生日志时所处阶段
	PhaseStr  string       `json:"phaseStr"`  // 阶段可读字符串
	CreatedAt time.Time    `json:"createdAt"` // 日志产生时间
}

// ============ ServicePhase 服务生命周期阶段 =================
type ServicePhase int

const (
	PhaseProbing    ServicePhase = iota // 探针阶段：验证配置可行性，最多 maxProbes 次连续失败
	PhaseRunning                        // 运行阶段：穿透成功且稳定过，短线后无限重启
	PhaseRestarting                     // 等待重启阶段:
	PhaseFailed                         // 运行失败：探针耗尽，需人为修改配置文件然后重启启动
	PhaseStopped                        // 主动停止
)

// 根据阶段返回 String
func (p ServicePhase) String() string {
	switch p {
	case PhaseProbing:
		return "PROBING" // 探测  运行5次探测
	case PhaseRunning:
		return "RUNNING" // 运行中 配置文件可用
	case PhaseRestarting:
		return "RESTARTING" // 无限重启
	case PhaseFailed:
		return "FAILED" // 探测失败
	default:
		return "STOPPED" // 手动停止
	}
}

// ============== serviceEntry 单个服务的运行时状态 ============

// serviceEntry 服务实例
type serviceEntry struct {
	cancel context.CancelFunc
	done   chan struct{}

	deviceName  string // 设备名
	serviceName string // 服务名

	// 以下字段供面板读取，goroutine 写，面板读，用 RWMutex 保护
	mu           sync.RWMutex
	phase        ServicePhase // 当前阶段
	externalPort uint16       // STUN 映射后的外部端口
	restartCount int          // 重启次数
	lastError    string
	logMessages  []ServiceLog // 滚动日志
	updatedAt    time.Time
}

// newServiceEntry 创建服务实例
func newServiceEntry(cancel context.CancelFunc, deviceName, serviceName string) *serviceEntry {
	return &serviceEntry{
		cancel:      cancel,
		done:        make(chan struct{}),
		deviceName:  deviceName,
		serviceName: serviceName,
		phase:       PhaseProbing,
		updatedAt:   time.Now(),
	}
}

// currentPhase 并发安全地读取当前阶段
func (e *serviceEntry) currentPhase() ServicePhase {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.phase
}

// applyState 唯一的状态变更入口，返回阶段是否真的改变了
// 若新阶段为 PhaseRestarting，自动累加重启计数
func (e *serviceEntry) applyState(phase ServicePhase, port uint16, msg string) (changed bool) {
	e.mu.Lock()
	defer e.mu.Unlock()

	now := time.Now()
	changed = e.phase != phase
	if phase == PhaseRestarting && changed {
		e.restartCount++
	}
	e.phase = phase
	e.externalPort = port

	switch phase {
	case PhaseFailed, PhaseRestarting:
		e.lastError = msg
	default:
		e.lastError = "" // 成功路径自动清错
	}

	e.updatedAt = now
	e.appendLogLocked(phase, msg, now)
	return
}

// onlyLog 仅追加日志，不改阶段
func (e *serviceEntry) onlyLog(message string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.appendLogLocked(e.phase, message, time.Now())
}

// appendLogLocked 空消息跳过；与上一条相同去重；超出上限丢弃最旧的
func (e *serviceEntry) appendLogLocked(phase ServicePhase, message string, now time.Time) {
	if message == "" {
		return
	}
	// 去重：与上一条相同则跳过
	if len(e.logMessages) > 0 && e.logMessages[len(e.logMessages)-1].Message == message {
		return
	}
	e.logMessages = append(e.logMessages, ServiceLog{
		Message:   message,
		Phase:     phase,
		PhaseStr:  phase.String(),
		CreatedAt: now,
	})

	// 滚动窗口：只保留最新的 maxServiceLogs 条
	if len(e.logMessages) > maxServiceLogs {
		e.logMessages = e.logMessages[len(e.logMessages)-maxServiceLogs:]
	}
}

// 生成单个服务的快照
func (e *serviceEntry) snapshot(key string, kind EventKind) StateEvent {
	e.mu.RLock()
	defer e.mu.RUnlock()
	logs := append([]ServiceLog(nil), e.logMessages...)
	return StateEvent{
		Kind:         kind,
		Key:          key,
		DeviceName:   e.deviceName,
		ServiceName:  e.serviceName,
		Phase:        e.phase,
		PhaseStr:     e.phase.String(),
		ExternalPort: e.externalPort,
		RestartCount: e.restartCount,
		LastError:    e.lastError,
		Logs:         logs,
		UpdatedAt:    e.updatedAt,
	}
}

// =====================Scheduler  调度器主体 =============

type Scheduler struct {
	ctx    context.Context // scheduler 级别的生命周期，用于 failedTTL goroutine
	cancel context.CancelFunc

	mu      sync.RWMutex
	service map[string]*serviceEntry // key:"deviceID-serviceID" 所有服务的管理
	runner  Runner                   // STUN 隧道执行器

	subMu        sync.RWMutex
	subscribers  map[chan StateEvent]struct{} // 所有活跃的订阅通道
	subBufferCap int                          // 每个订阅通道的缓冲大小，默认 16
}

// 创建调度器
func NewScheduler(runner Runner) *Scheduler {
	if runner == nil {
		runner = NewSTUNRunner()
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &Scheduler{
		ctx:          ctx,
		cancel:       cancel,
		service:      make(map[string]*serviceEntry),
		runner:       runner,
		subscribers:  make(map[chan StateEvent]struct{}),
		subBufferCap: 16,
	}
}

// Close 关闭调度器，停止所有服务并取消 failedTTL goroutine
func (s *Scheduler) Close() {
	s.cancel()
	s.StopAll()
}

// detach 从mac 里去掉 entry 并发送取消信号，返回被去掉的服务
func (s *Scheduler) detach(key string) *serviceEntry {
	s.mu.Lock()
	entry, ok := s.service[key]
	if ok {
		delete(s.service, key)
	}
	s.mu.Unlock()

	if ok && entry != nil {
		entry.cancel()
	}
	return entry
}

// waitEntry 等待 entry 的 goroutine 退出，最多等 12s
func waitEntry(entry *serviceEntry) {
	if entry == nil {
		return
	}
	select {
	case <-entry.done:
	case <-time.After(12 * time.Second):
	}
}

// releaseEntry goroutine 退出时清理 map，FAILED 状态保留供外部查询失败原因
func (s *Scheduler) releaseEntry(Key string, entry *serviceEntry) {
	if entry.currentPhase() == PhaseFailed {
		return
	}
	s.mu.Lock()
	if currentEntry, ok := s.service[Key]; ok && currentEntry == entry {
		delete(s.service, Key)
	}
	s.mu.Unlock()
}

// transition 状态变更唯一写入路径：改 entry → 决定 kind → emit
func (s *Scheduler) transition(entry *serviceEntry, key string, phase ServicePhase, port uint16, msg string) {
	changed := entry.applyState(phase, port, msg)
	kind := EventLogAppended
	if changed {
		kind = EventPhaseChanged
	}
	s.emit(entry.snapshot(key, kind))
}

// log 仅追加日志，不改阶段
func (s *Scheduler) log(entry *serviceEntry, key, msg string) {
	if msg == "" {
		return
	}
	entry.onlyLog(msg)
	s.emit(entry.snapshot(key, EventLogAppended))
}

// Snapshot  查所有服务的Event
func (s *Scheduler) Snapshot() []StateEvent {
	s.mu.RLock()
	// 先在读锁里，把所有 entry 的引用收集起来
	type kv struct {
		k string
		e *serviceEntry
	}
	items := make([]kv, 0, len(s.service))
	for k, e := range s.service {
		items = append(items, kv{k, e})
	}
	s.mu.RUnlock()
	result := make([]StateEvent, 0, len(items))
	for _, it := range items {
		result = append(result, it.e.snapshot(it.k, EventPhaseChanged))
	}
	return result

}

// ============== 订阅事件 ============
// Subscribe 注册订阅者
func (s *Scheduler) Subscribe() (<-chan StateEvent, func()) {
	ch := make(chan StateEvent, s.subBufferCap) // 创建一个带缓冲的通道
	s.subMu.Lock()
	s.subscribers[ch] = struct{}{} // 把这个通道登记到订阅者表里
	s.subMu.Unlock()

	return ch, func() { // 返回两个东西：通道本身、以及一个"取消函数"
		s.subMu.Lock()
		delete(s.subscribers, ch) // 调用取消函数时，把这个通道从订阅者表里删掉
		s.subMu.Unlock()
	}
}

func (s *Scheduler) Get(deviceID, serviceID uint) (StateEvent, bool) {
	key := serviceKey(deviceID, serviceID)
	s.mu.RLock() // 读锁：允许多个 goroutine 同时读，但不能同时写
	entry, ok := s.service[key]
	s.mu.RUnlock()
	if !ok {
		return StateEvent{}, false // 服务不存在
	}
	return entry.snapshot(key, EventPhaseChanged), true
}

// emit  把事件推给所有订阅者
func (s *Scheduler) emit(event StateEvent) {
	s.subMu.RLock()
	// 先在读锁里面把订阅者列表复制一份出来
	subs := make([]chan StateEvent, 0, len(s.subscribers))
	for ch := range s.subscribers {
		subs = append(subs, ch)
	}
	s.subMu.RUnlock()

	// 逐个发送
	for _, ch := range subs {
		select {
		case ch <- event:
		default:
		}
	}
}

// ============== 生命周期管理 =============

// StartAll 批量启动 devices 列表中所有 Enabled=true 的服务
func (s *Scheduler) StartAll(devices []model.Device) {
	for i := range devices {
		device := &devices[i]
		for j := range device.Services {
			service := &device.Services[j]
			if service.Enabled {
				s.StartService(device, service)
			}
		}
	}
}

// 启动服务
func (s *Scheduler) StartService(device *model.Device, service *model.Service) {
	key := serviceKey(device.DeviceID, service.ID)

	// 先停止旧的服务,并且等待服务取消
	old := s.detach(key)
	waitEntry(old)

	// 重置运行时字段，清空上次遗留状态
	service.PunchSuccess = false
	service.ExternalPort = 0
	service.LastError = ""

	if !service.Enabled {
		return
	}

	// 重建服务
	ctx, cancel := context.WithCancel(context.Background())
	entry := newServiceEntry(cancel, device.Name, service.Name)

	// 二次检查：waitEntry 期间另一方可能已注册了相同 key
	s.mu.Lock()
	if _, exists := s.service[key]; exists {
		s.mu.Unlock()
		cancel()
		return
	}
	s.service[key] = entry
	s.mu.Unlock()

	req := buildSTUNRequest(device, service)

	go s.runService(ctx, key, entry, req)
}

func (s *Scheduler) StopService(deviceID, serviceID uint) {
	waitEntry(s.detach(serviceKey(deviceID, serviceID)))
}

// 停止所有服务
func (s *Scheduler) StopAll() {
	s.mu.Lock()
	entries := make([]*serviceEntry, 0, len(s.service))
	for k, e := range s.service {
		delete(s.service, k)
		e.cancel()
		entries = append(entries, e)
	}
	s.mu.Unlock()

	for _, e := range entries {
		waitEntry(e)
	}
}

// ====================runService 主循环 ==========
// runService 每个服务对应一个 goroutine
// 退出条件：
//
//	a. ctx 取消（外部 StopService）→ PhaseStopped
//	b. runner.Run 返回 nil（正常结束）→ PhaseStopped
//	c. 探测连续失败 ≥ maxProbes → PhaseFailed，service.Enabled=false
func (s *Scheduler) runService(ctx context.Context, key string, entry *serviceEntry, req STUNRequest) {
	defer close(entry.done)
	defer s.releaseEntry(key, entry)

	const maxProbes = 5

	probeFailures := 0
	everAlive := false
	probingBackoff := NewProbingBackoff()
	RestartingBackoff := NewRestartingBackoff()

	s.transition(entry, key, PhaseProbing, 0, "")

	for {

		//手动停止
		if ctx.Err() != nil {
			s.transition(entry, key, PhaseStopped, 0, "")
		}

		err := s.runner.Run(ctx, req, func(state STUNState) {
			switch state.State {
			case STUNMapped:
				s.transition(entry, key, PhaseProbing, state.ExternalPort, state.Log)
			case STUNAlive:
				everAlive = true
				s.transition(entry, key, PhaseRunning, state.ExternalPort, state.Log)
			case STUNFailed:
				s.transition(entry, key, PhaseRestarting, 0, state.Log)
			case STUNLog:
				s.log(entry, key, state.Log) // 仍然保留：这是「不切换阶段，只加日志」
			}
		})

		// runner跑完一轮，如果手动取消
		if ctx.Err() != nil {
			s.transition(entry, key, PhaseStopped, 0, "")
			return
		}
		if err == nil {
			s.transition(entry, key, PhaseStopped, 0, "")
			return
		}

		if everAlive {
			// 曾经活过：无限退避重启
			s.transition(entry, key, PhaseRestarting, 0, err.Error())
			if !sleepCtx(ctx, RestartingBackoff.Next()) {
				s.transition(entry, key, PhaseStopped, 0, "")
				return
			}
			s.transition(entry, key, PhaseProbing, 0, "")
			continue
		}

		// 从未活过：消耗探针
		probeFailures++
		if probeFailures >= maxProbes {
			s.transition(entry, key, PhaseFailed, 0, err.Error())
			return
		}
		s.transition(entry, key, PhaseRestarting, 0, err.Error())
		if !sleepCtx(ctx, probingBackoff.Next()) {
			s.transition(entry, key, PhaseStopped, 0, "")
			return
		}
		s.transition(entry, key, PhaseProbing, 0, "")

	}

}

// ====================== Backoff退避策略 ================

// Backoff 按预步长依次返回等待时长
type Backoff struct {
	steps []time.Duration // 退避步长列表
	index int             // 当前步长下标
}

// NewRestartingBackoff 创建重启退避器：1s → 2s → 4s → 10s → 1min → 1min...
func NewRestartingBackoff() *Backoff {
	return &Backoff{
		steps: []time.Duration{
			1 * time.Second,
			3 * time.Second,
			5 * time.Second,
			10 * time.Second,
			1 * time.Minute,
		},
	}
}

func NewProbingBackoff() *Backoff {
	return &Backoff{
		steps: []time.Duration{
			1 * time.Second,
			1 * time.Second,
			1 * time.Second,
			1 * time.Second,
		},
	}
}

// Next 返回下一个退避时长。
func (b *Backoff) Next() time.Duration {
	if b.index < len(b.steps) {
		step := b.steps[b.index]
		b.index++
		return step
	}
	return b.steps[len(b.steps)-1]
}

// Reset 重置退避器
func (b *Backoff) Reset() {
	b.index = 0
}

// ============ 辅助函数 =========

// serviceKey 将设备 ID 和服务 ID 拼成唯一字符串键
func serviceKey(deviceID, serviceID uint) string {
	return fmt.Sprintf("%d-%d", deviceID, serviceID)
}

func buildSTUNRequest(device *model.Device, service *model.Service) STUNRequest {
	return STUNRequest{
		ServiceName:  service.Name,
		TargetIP:     device.IP,
		InternalPort: service.InternalPort,
		Protocol:     service.Protocol,
		UseUPnP:      service.UseUPnP,
	}
}

func sleepCtx(ctx context.Context, d time.Duration) bool {
	select {
	case <-time.After(d):
		return true
	case <-ctx.Done():
		return false
	}
}
