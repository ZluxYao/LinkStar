package stun

import (
	"context"
	"fmt"
	"linkstar/modules/stun/model"
	"sync"
	"sync/atomic"
	"time"
)

type Backoff struct {
	steps []time.Duration
	idx   int
}

func NewBackoff() *Backoff {
	return &Backoff{
		steps: []time.Duration{
			1 * time.Second,
			2 * time.Second,
			4 * time.Second,
			5 * time.Minute,
		},
	}
}

func (b *Backoff) Next() time.Duration {
	if b.idx < len(b.steps) {
		d := b.steps[b.idx]
		b.idx++
		return d
	}
	return 5 * time.Minute
}

func (b *Backoff) Reset() {
	b.idx = 0
}

type ServicePhase int

const (
	PhaseProbing ServicePhase = iota
	PhaseRunning
	PhaseRestarting
	PhaseFailed
	PhaseStopped
)

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

type StateEvent struct {
	Key          string       `json:"key"`
	DeviceName   string       `json:"deviceName"`
	ServiceName  string       `json:"serviceName"`
	Phase        ServicePhase `json:"phase"`
	PhaseStr     string       `json:"phaseStr"`
	ExternalPort uint16       `json:"externalPort"`
	RestartCount int          `json:"restartCount"`
	LastError    string       `json:"lastError"`
	Logs         []ServiceLog `json:"logs"`
	UpdatedAt    time.Time    `json:"updatedAt"`
}

type ServiceLog struct {
	Message   string       `json:"message"`
	Phase     ServicePhase `json:"phase"`
	PhaseStr  string       `json:"phaseStr"`
	CreatedAt time.Time    `json:"createdAt"`
}

const maxServiceLogs = 15

type serviceEntry struct {
	cancel      context.CancelFunc
	done        chan struct{}
	deviceName  string
	serviceName string

	mu           sync.RWMutex
	phase        ServicePhase
	externalPort uint16
	restartCount int
	lastError    string
	logMessages  []ServiceLog
	updatedAt    time.Time
}

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

func (e *serviceEntry) setState(phase ServicePhase, port uint16, errMsg string) {
	e.mu.Lock()
	now := time.Now()
	if phase == PhaseRestarting {
		e.restartCount++
	}
	e.phase = phase
	e.externalPort = port
	e.lastError = errMsg
	e.updatedAt = now
	e.appendLogLocked(phase, errMsg, now)
	e.mu.Unlock()
}

func (e *serviceEntry) addLog(phase ServicePhase, message string) {
	e.mu.Lock()
	e.appendLogLocked(phase, message, time.Now())
	e.mu.Unlock()
}

func (e *serviceEntry) appendLogLocked(phase ServicePhase, message string, now time.Time) {
	if message == "" {
		return
	}

	if len(e.logMessages) > 0 && e.logMessages[len(e.logMessages)-1].Message == message {
		return
	}

	e.logMessages = append(e.logMessages, ServiceLog{
		Message:   message,
		Phase:     phase,
		PhaseStr:  phase.String(),
		CreatedAt: now,
	})
	if len(e.logMessages) > maxServiceLogs {
		e.logMessages = e.logMessages[len(e.logMessages)-maxServiceLogs:]
	}
}

func (e *serviceEntry) phaseValue() ServicePhase {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.phase
}

func (e *serviceEntry) snapshot(key string) StateEvent {
	e.mu.RLock()
	defer e.mu.RUnlock()
	logs := append([]ServiceLog(nil), e.logMessages...)
	return StateEvent{
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

type Scheduler struct {
	mu       sync.Mutex
	services map[string]*serviceEntry
	runner   TunnelRunner

	subMu        sync.RWMutex
	subscribers  map[chan StateEvent]struct{}
	subBufferCap int
}

func NewScheduler(runner TunnelRunner) *Scheduler {
	if runner == nil {
		runner = NewSTUNTunnelRunner()
	}

	return &Scheduler{
		services:     make(map[string]*serviceEntry),
		runner:       runner,
		subscribers:  make(map[chan StateEvent]struct{}),
		subBufferCap: 16,
	}
}

func (s *Scheduler) Subscribe() (<-chan StateEvent, func()) {
	ch := make(chan StateEvent, s.subBufferCap)

	s.subMu.Lock()
	s.subscribers[ch] = struct{}{}
	s.subMu.Unlock()

	cancel := func() {
		s.subMu.Lock()
		if _, ok := s.subscribers[ch]; ok {
			delete(s.subscribers, ch)
		}
		s.subMu.Unlock()
	}

	return ch, cancel
}

func (s *Scheduler) Snapshot() []StateEvent {
	s.mu.Lock()
	entries := make([]struct {
		key   string
		entry *serviceEntry
	}, 0, len(s.services))
	for key, entry := range s.services {
		entries = append(entries, struct {
			key   string
			entry *serviceEntry
		}{key: key, entry: entry})
	}
	s.mu.Unlock()

	result := make([]StateEvent, 0, len(entries))
	for _, item := range entries {
		result = append(result, item.entry.snapshot(item.key))
	}

	return result
}

func serviceKey(deviceID, serviceID uint) string {
	return fmt.Sprintf("%d-%d", deviceID, serviceID)
}

func (s *Scheduler) emit(entry *serviceEntry, key string) {
	event := entry.snapshot(key)

	s.subMu.RLock()
	subscribers := make([]chan StateEvent, 0, len(s.subscribers))
	for ch := range s.subscribers {
		subscribers = append(subscribers, ch)
	}
	s.subMu.RUnlock()

	for _, ch := range subscribers {
		select {
		case ch <- event:
		default:
		}
	}
}

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

func (s *Scheduler) StartService(device *model.Device, service *model.Service) {
	key := serviceKey(device.DeviceID, service.ID)

	old := s.detachService(key)
	waitServiceEntry(old)

	resetServiceRuntime(service)
	service.LastError = ""

	if !service.Enabled {
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	entry := newServiceEntry(cancel, device.Name, service.Name)

	s.mu.Lock()
	if _, exists := s.services[key]; exists {
		s.mu.Unlock()
		cancel()
		return
	}
	s.services[key] = entry
	s.mu.Unlock()

	go s.runService(ctx, device, service, key, entry)
}

func (s *Scheduler) StopService(deviceID, serviceID uint) {
	key := serviceKey(deviceID, serviceID)
	waitServiceEntry(s.detachService(key))
}

func (s *Scheduler) StopAll() {
	s.mu.Lock()
	entries := make([]*serviceEntry, 0, len(s.services))
	for key, entry := range s.services {
		delete(s.services, key)
		entry.cancel()
		entries = append(entries, entry)
	}
	s.mu.Unlock()

	for _, entry := range entries {
		waitServiceEntry(entry)
	}
}

func (s *Scheduler) runService(
	ctx context.Context,
	device *model.Device,
	service *model.Service,
	key string,
	entry *serviceEntry,
) {
	defer close(entry.done)
	defer s.releaseService(key, entry)

	const maxProbeFailures = 5

	probeFailures := 0
	backoff := NewBackoff()

	s.setRuntimeState(service, entry, key, PhaseProbing, 0, "")

	for {
		if ctx.Err() != nil {
			s.setRuntimeState(service, entry, key, PhaseStopped, 0, "")
			return
		}

		var ready atomic.Bool
		err := s.runner.Run(ctx, s.buildRequest(device, service), func(state STUNState) {
			s.handleSTUNState(service, entry, key, &ready, state)
		})

		if ctx.Err() != nil {
			s.setRuntimeState(service, entry, key, PhaseStopped, 0, "")
			return
		}

		if err == nil {
			s.setRuntimeState(service, entry, key, PhaseStopped, 0, "")
			return
		}

		errMsg := err.Error()
		if ready.Load() {
			probeFailures = 0
			backoff.Reset()
			wait := backoff.Next()
			s.setRuntimeState(service, entry, key, PhaseRestarting, 0, errMsg)
			if !sleepWithCtx(ctx, wait) {
				s.setRuntimeState(service, entry, key, PhaseStopped, 0, "")
				return
			}
			continue
		}

		probeFailures++
		if probeFailures >= maxProbeFailures {
			service.Enabled = false
			s.setRuntimeState(service, entry, key, PhaseFailed, 0, errMsg)
			return
		}

		s.setRuntimeState(service, entry, key, PhaseRestarting, 0, errMsg)
		if !sleepWithCtx(ctx, 1*time.Second) {
			s.setRuntimeState(service, entry, key, PhaseStopped, 0, "")
			return
		}

		s.setRuntimeState(service, entry, key, PhaseProbing, 0, errMsg)
	}
}

func (s *Scheduler) handleSTUNState(
	service *model.Service,
	entry *serviceEntry,
	key string,
	ready *atomic.Bool,
	state STUNState,
) {
	switch state.State {
	case STUNMapped:
		s.setRuntimeState(service, entry, key, PhaseProbing, state.ExternalPort, "")
		entry.addLog(PhaseProbing, state.Log)
		s.emit(entry, key)
	case STUNAlive:
		ready.Store(true)
		s.setRuntimeState(service, entry, key, PhaseRunning, state.ExternalPort, "")
		entry.addLog(PhaseRunning, state.Log)
		s.emit(entry, key)
	case STUNFailed:
		s.setRuntimeState(service, entry, key, PhaseRestarting, 0, state.Log)
	case STUNLog:
		entry.addLog(entry.phaseValue(), state.Log)
		s.emit(entry, key)
	}
}

func (s *Scheduler) setRuntimeState(
	service *model.Service,
	entry *serviceEntry,
	key string,
	phase ServicePhase,
	port uint16,
	errMsg string,
) {
	entry.setState(phase, port, errMsg)
	s.applyServiceRuntime(service, phase, port, errMsg)
	s.emit(entry, key)
}

func (s *Scheduler) applyServiceRuntime(
	service *model.Service,
	phase ServicePhase,
	port uint16,
	errMsg string,
) {
	switch phase {
	case PhaseRunning:
		service.PunchSuccess = true
		service.ExternalPort = port
		service.LastError = ""
	case PhaseFailed:
		service.PunchSuccess = false
		service.ExternalPort = 0
		service.LastError = errMsg
	case PhaseRestarting:
		service.PunchSuccess = false
		service.ExternalPort = 0
		service.LastError = errMsg
	case PhaseProbing:
		service.PunchSuccess = false
		service.ExternalPort = port
		service.LastError = errMsg
	default:
		service.PunchSuccess = false
		service.ExternalPort = 0
		service.LastError = ""
	}
}

func (s *Scheduler) buildRequest(device *model.Device, service *model.Service) STUNRequest {
	return STUNRequest{
		ServiceName:  service.Name,
		TargetIP:     device.IP,
		InternalPort: service.InternalPort,
		Protocol:     service.Protocol,
		UseUPnP:      service.UseUPnP,
	}
}

func (s *Scheduler) detachService(key string) *serviceEntry {
	s.mu.Lock()
	entry := s.services[key]
	if entry != nil {
		delete(s.services, key)
	}
	s.mu.Unlock()

	if entry != nil {
		entry.cancel()
	}

	return entry
}

func (s *Scheduler) releaseService(key string, entry *serviceEntry) {
	if entry.phaseValue() == PhaseFailed {
		return
	}

	s.mu.Lock()
	if current, ok := s.services[key]; ok && current == entry {
		delete(s.services, key)
	}
	s.mu.Unlock()
}

func waitServiceEntry(entry *serviceEntry) {
	if entry == nil {
		return
	}

	select {
	case <-entry.done:
	case <-time.After(15 * time.Second):
	}
}

func resetServiceRuntime(service *model.Service) {
	service.PunchSuccess = false
	service.ExternalPort = 0
}
