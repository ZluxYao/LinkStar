package stun

import (
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"linkstar/modules/stun/model"
)

// TrafficServiceInfo 是流量计数器的展示信息。
type TrafficServiceInfo struct {
	Key         string
	DeviceName  string
	ServiceName string
	Protocol    string
}

// TrafficMeter 记录一个穿透服务的转发流量。
// 原子计数器让转发热路径不需要抢全局锁。
type TrafficMeter struct {
	infoMu sync.RWMutex
	info   TrafficServiceInfo

	inBytes  atomic.Uint64
	outBytes atomic.Uint64
	active   atomic.Int64
}

func (m *TrafficMeter) updateInfo(info TrafficServiceInfo) {
	m.infoMu.Lock()
	m.info = info
	m.infoMu.Unlock()
}

func (m *TrafficMeter) getInfo() TrafficServiceInfo {
	m.infoMu.RLock()
	defer m.infoMu.RUnlock()
	return m.info
}

func (m *TrafficMeter) addInbound(n int64) {
	if n > 0 {
		m.inBytes.Add(uint64(n))
	}
}

func (m *TrafficMeter) addOutbound(n int64) {
	if n > 0 {
		m.outBytes.Add(uint64(n))
	}
}

func (m *TrafficMeter) beginConnection() {
	m.active.Add(1)
}

func (m *TrafficMeter) endConnection() {
	if m.active.Add(-1) < 0 {
		// 理论上不会发生。保底恢复为 0，避免异常路径显示负数。
		m.active.Store(0)
	}
}

type trafficSample struct {
	inBytes  uint64
	outBytes uint64
}

type trafficRegistry struct {
	mu     sync.RWMutex
	meters map[string]*TrafficMeter

	sampleMu    sync.Mutex
	lastSample  time.Time
	lastService map[string]trafficSample
}

func newTrafficRegistry() *trafficRegistry {
	return &trafficRegistry{
		meters:      make(map[string]*TrafficMeter),
		lastService: make(map[string]trafficSample),
	}
}

var trafficRuntime = newTrafficRegistry()

func (r *trafficRegistry) meter(info TrafficServiceInfo) *TrafficMeter {
	if info.Key == "" {
		return nil
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if meter, ok := r.meters[info.Key]; ok {
		meter.updateInfo(info)
		return meter
	}
	meter := &TrafficMeter{info: info}
	r.meters[info.Key] = meter
	return meter
}

func bytesPerSecond(delta uint64, elapsed time.Duration) uint64 {
	if delta == 0 || elapsed <= 0 {
		return 0
	}
	rate := float64(delta) / elapsed.Seconds()
	if rate >= float64(^uint64(0)) {
		return ^uint64(0)
	}
	return uint64(rate + 0.5)
}

// snapshot 返回指定服务的流量快照，并用上一次快照计算当前速率。
func (r *trafficRegistry) snapshot(infos []TrafficServiceInfo) model.TrafficObservation {
	meters := make([]*TrafficMeter, 0, len(infos))
	r.mu.Lock()
	for _, info := range infos {
		meters = append(meters, r.meterLocked(info))
	}
	r.mu.Unlock()

	sort.Slice(meters, func(i, j int) bool {
		return meters[i].getInfo().Key < meters[j].getInfo().Key
	})

	now := time.Now()
	r.sampleMu.Lock()
	defer r.sampleMu.Unlock()

	elapsed := now.Sub(r.lastSample)
	if r.lastSample.IsZero() {
		elapsed = 0
	}

	result := model.TrafficObservation{
		Services: make([]model.ServiceTraffic, 0, len(meters)),
	}
	currentKeys := make(map[string]struct{}, len(meters))
	for _, meter := range meters {
		info := meter.getInfo()
		inBytes := meter.inBytes.Load()
		outBytes := meter.outBytes.Load()
		active := meter.active.Load()
		if active < 0 {
			active = 0
		}

		previous := r.lastService[info.Key]
		service := model.ServiceTraffic{
			Key:               info.Key,
			DeviceName:        info.DeviceName,
			ServiceName:       info.ServiceName,
			Protocol:          info.Protocol,
			InBytes:           inBytes,
			OutBytes:          outBytes,
			InBytesPerSecond:  bytesPerSecond(inBytes-previous.inBytes, elapsed),
			OutBytesPerSecond: bytesPerSecond(outBytes-previous.outBytes, elapsed),
			ActiveConnections: int(active),
		}
		result.InBytes += inBytes
		result.OutBytes += outBytes
		result.ActiveConnections += int(active)
		result.Services = append(result.Services, service)
		currentKeys[info.Key] = struct{}{}
		r.lastService[info.Key] = trafficSample{inBytes: inBytes, outBytes: outBytes}
	}

	for key := range r.lastService {
		if _, ok := currentKeys[key]; !ok {
			delete(r.lastService, key)
		}
	}
	// 总速率直接累加单服务速率，避免把不同服务的采样时间混在一起。
	for _, service := range result.Services {
		result.InBytesPerSecond += service.InBytesPerSecond
		result.OutBytesPerSecond += service.OutBytesPerSecond
	}

	r.lastSample = now
	return result
}

func (r *trafficRegistry) meterLocked(info TrafficServiceInfo) *TrafficMeter {
	if meter, ok := r.meters[info.Key]; ok {
		meter.updateInfo(info)
		return meter
	}
	meter := &TrafficMeter{info: info}
	r.meters[info.Key] = meter
	return meter
}

func trafficMeter(info TrafficServiceInfo) *TrafficMeter {
	return trafficRuntime.meter(info)
}
