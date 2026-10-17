package monitor

import (
	"os"
	"runtime"
	"runtime/metrics"
	"sync"
	"time"

	"github.com/pbnjay/memory"
	"linkstar/modules/monitor/model"
)

var (
	processStateMu sync.Mutex
	startedAt      = time.Now()
	lastCPUAt      time.Time
	lastCPUSecs    float64
)

var processCPUMetricNames = []string{
	"/cpu/classes/user:cpu-seconds",
	"/cpu/classes/gc/total:cpu-seconds",
	"/cpu/classes/scavenge/total:cpu-seconds",
}

// InitMonitor 初始化运行监控模块。
func InitMonitor() error {
	processStateMu.Lock()
	defer processStateMu.Unlock()
	startedAt = time.Now()
	lastCPUAt = time.Time{}
	lastCPUSecs = 0
	return nil
}

// GetSystemSnapshot 返回当前主机和 LinkStar 进程的资源快照。
func GetSystemSnapshot() model.SystemObservation {
	now := time.Now()
	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)

	totalMemory := memory.TotalMemory()
	availableMemory := memory.FreeMemory()
	if availableMemory > totalMemory {
		availableMemory = totalMemory
	}

	workingDir, err := os.Getwd()
	disk := model.DiskObservation{Path: "工作目录"}
	if err != nil {
		disk.Error = "获取工作目录失败: " + err.Error()
	} else if usage, diskErr := readDiskUsage(workingDir); diskErr != nil {
		disk.Error = diskErr.Error()
	} else {
		disk.TotalBytes = usage.totalBytes
		disk.FreeBytes = usage.freeBytes
		if usage.totalBytes >= usage.freeBytes {
			disk.UsedBytes = usage.totalBytes - usage.freeBytes
		}
	}

	processStateMu.Lock()
	uptime := int64(now.Sub(startedAt).Seconds())
	processStateMu.Unlock()
	if uptime < 0 {
		uptime = 0
	}

	return model.SystemObservation{
		SampledAt: now,
		OS:        runtime.GOOS,
		Arch:      runtime.GOARCH,
		CPUCount:  runtime.NumCPU(),
		Memory: model.MemoryObservation{
			TotalBytes:     totalMemory,
			AvailableBytes: availableMemory,
			UsedBytes:      totalMemory - availableMemory,
		},
		Process: model.ProcessObservation{
			CPUPercent:    sampleProcessCPU(now),
			MemoryBytes:   mem.Sys,
			HeapBytes:     mem.HeapAlloc,
			Goroutines:    runtime.NumGoroutine(),
			UptimeSeconds: uptime,
		},
		Disk: disk,
	}
}

func sampleProcessCPU(now time.Time) float64 {
	samples := make([]metrics.Sample, len(processCPUMetricNames))
	for i, name := range processCPUMetricNames {
		samples[i].Name = name
	}
	metrics.Read(samples)

	var cpuSeconds float64
	for _, sample := range samples {
		if sample.Value.Kind() == metrics.KindFloat64 {
			cpuSeconds += sample.Value.Float64()
		}
	}

	processStateMu.Lock()
	defer processStateMu.Unlock()
	if lastCPUAt.IsZero() || !now.After(lastCPUAt) {
		lastCPUAt = now
		lastCPUSecs = cpuSeconds
		return 0
	}

	elapsed := now.Sub(lastCPUAt).Seconds()
	cpuDelta := cpuSeconds - lastCPUSecs
	lastCPUAt = now
	lastCPUSecs = cpuSeconds
	if elapsed <= 0 || cpuDelta < 0 || runtime.NumCPU() <= 0 {
		return 0
	}

	percent := cpuDelta / elapsed / float64(runtime.NumCPU()) * 100
	if percent < 0 {
		return 0
	}
	if percent > 100 {
		return 100
	}
	return percent
}
