//go:build windows

package automation

import (
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
)

// DiskUsagePct trả về % đĩa đã dùng tại path (Windows: GetDiskFreeSpaceEx).
func DiskUsagePct(path string) (int, error) {
	p16, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return 0, err
	}
	var free, total, avail uint64
	// GetDiskFreeSpaceExW không có wrapper sẵn trong x/sys ở mọi bản —
	// gọi trực tiếp qua syscall.
	r1, _, e1 := windows.NewLazySystemDLL("kernel32.dll").
		NewProc("GetDiskFreeSpaceExW").Call(
		uintptr(unsafe.Pointer(p16)),
		uintptr(unsafe.Pointer(&avail)),
		uintptr(unsafe.Pointer(&total)),
		uintptr(unsafe.Pointer(&free)),
	)
	if r1 == 0 {
		if e1 != nil && e1 != windows.ERROR_SUCCESS {
			return 0, e1
		}
		return 0, fmt.Errorf("không đo được dung lượng đĩa")
	}
	if total == 0 {
		return 0, fmt.Errorf("không đo được dung lượng đĩa")
	}
	used := total - free
	return int(used * 100 / total), nil
}
