//go:build windows

package monitor

import (
	"golang.org/x/sys/windows"
)

func readDiskUsagePlatform(path string) (diskUsage, error) {
	directoryName, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return diskUsage{}, err
	}
	var freeBytes, totalBytes, totalFreeBytes uint64
	if err := windows.GetDiskFreeSpaceEx(directoryName, &freeBytes, &totalBytes, &totalFreeBytes); err != nil {
		return diskUsage{}, err
	}
	return diskUsage{
		totalBytes: totalBytes,
		freeBytes:  freeBytes,
	}, nil
}
