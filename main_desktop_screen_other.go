//go:build desktop && !windows

package main

// primaryWorkArea 只在 Windows 上有实现，其他平台返回 0, 0 表示不知道，默认尺寸原样用。
func primaryWorkArea() (int, int) { return 0, 0 }
