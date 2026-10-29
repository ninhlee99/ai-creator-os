//go:build windows

package sysmon

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

// ReadDiskUsage đo dung lượng đĩa trên Windows.
func ReadDiskUsage(path string) (totalGB, usedGB, freeGB, pct float64, status string) {
	if path == "" {
		path = "."
	}
	p16, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return 0, 0, 0, 0, "ok"
	}
	var free, total, avail uint64
	r1, _, e1 := windows.NewLazySystemDLL("kernel32.dll").
		NewProc("GetDiskFreeSpaceExW").Call(
		uintptr(unsafe.Pointer(p16)),
		uintptr(unsafe.Pointer(&avail)),
		uintptr(unsafe.Pointer(&total)),
		uintptr(unsafe.Pointer(&free)),
	)
	if r1 == 0 || total == 0 {
		_ = e1
		return 0, 0, 0, 0, "ok"
	}
	used := total - free
	totalGB = float64(total) / (1024 * 1024 * 1024)
	usedGB = float64(used) / (1024 * 1024 * 1024)
	freeGB = float64(free) / (1024 * 1024 * 1024)
	pct = float64(used) * 100.0 / float64(total)
	status = "ok"
	if pct >= 90.0 {
		status = "danger"
	} else if pct >= 75.0 {
		status = "warn"
	}
	return
}
