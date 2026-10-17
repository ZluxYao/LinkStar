package model

import "time"

// SystemObservation 是运行监控页面使用的系统资源快照。
type SystemObservation struct {
	SampledAt time.Time          `json:"sampledAt"`
	OS        string             `json:"os"`
	Arch      string             `json:"arch"`
	CPUCount  int                `json:"cpuCount"`
	Memory    MemoryObservation  `json:"memory"`
	Process   ProcessObservation `json:"process"`
	Disk      DiskObservation    `json:"disk"`
}

// MemoryObservation 是主机内存使用情况。
type MemoryObservation struct {
	TotalBytes     uint64 `json:"totalBytes"`
	AvailableBytes uint64 `json:"availableBytes"`
	UsedBytes      uint64 `json:"usedBytes"`
}

// ProcessObservation 是 LinkStar 进程自身的运行情况。
type ProcessObservation struct {
	CPUPercent    float64 `json:"cpuPercent"`
	MemoryBytes   uint64  `json:"memoryBytes"`
	HeapBytes     uint64  `json:"heapBytes"`
	Goroutines    int     `json:"goroutines"`
	UptimeSeconds int64   `json:"uptimeSeconds"`
}

// DiskObservation 是工作目录所在磁盘的容量情况。
type DiskObservation struct {
	Path       string `json:"path"`
	TotalBytes uint64 `json:"totalBytes"`
	UsedBytes  uint64 `json:"usedBytes"`
	FreeBytes  uint64 `json:"freeBytes"`
	Error      string `json:"error,omitempty"`
}
