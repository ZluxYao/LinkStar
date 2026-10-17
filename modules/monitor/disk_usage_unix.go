//go:build !windows

package monitor

import "golang.org/x/sys/unix"

func readDiskUsagePlatform(path string) (diskUsage, error) {
	var stat unix.Statfs_t
	if err := unix.Statfs(path, &stat); err != nil {
		return diskUsage{}, err
	}
	blockSize := uint64(stat.Bsize)
	return diskUsage{
		totalBytes: uint64(stat.Blocks) * blockSize,
		freeBytes:  uint64(stat.Bavail) * blockSize,
	}, nil
}
