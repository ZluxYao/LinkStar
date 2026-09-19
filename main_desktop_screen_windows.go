//go:build desktop

package main

import "golang.org/x/sys/windows"

// SM_CXFULLSCREEN / SM_CYFULLSCREEN：主显示器去掉任务栏之后能给窗口的尺寸
const (
	smCXFullscreen = 16
	smCYFullscreen = 17
)

var (
	user32               = windows.NewLazySystemDLL("user32.dll")
	procGetSystemMetrics = user32.NewProc("GetSystemMetrics")
	procGetDpiForSystem  = user32.NewProc("GetDpiForSystem")
)

// primaryWorkArea 返回主显示器可用区域的逻辑像素尺寸，取不到返回 0, 0。
func primaryWorkArea() (int, int) {
	rw, _, _ := procGetSystemMetrics.Call(smCXFullscreen)
	rh, _, _ := procGetSystemMetrics.Call(smCYFullscreen)
	w, h := int(int32(rw)), int(int32(rh))
	if w <= 0 || h <= 0 {
		return 0, 0
	}
	// 进程是 per-monitor DPI 感知的，上面拿到的是物理像素；
	// wails 的 Width/Height 是逻辑像素，按系统 DPI 折回去。
	// GetDpiForSystem 是 Win10 1607 才有的，没有就当 100% 处理。
	if procGetDpiForSystem.Find() == nil {
		if dpi, _, _ := procGetDpiForSystem.Call(); dpi >= 96 {
			w = w * 96 / int(dpi)
			h = h * 96 / int(dpi)
		}
	}
	return w, h
}
