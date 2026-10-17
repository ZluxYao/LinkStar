package monitor

import "fmt"

type diskUsage struct {
	totalBytes uint64
	freeBytes  uint64
}

func readDiskUsage(path string) (diskUsage, error) {
	usage, err := readDiskUsagePlatform(path)
	if err != nil {
		return diskUsage{}, fmt.Errorf("读取磁盘容量失败: %w", err)
	}
	if usage.freeBytes > usage.totalBytes {
		usage.freeBytes = usage.totalBytes
	}
	return usage, nil
}
