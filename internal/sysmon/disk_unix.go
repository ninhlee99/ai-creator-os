//go:build !windows

package sysmon

import "syscall"

// ReadDiskUsage đo dung lượng đĩa tại đường dẫn path (mặc định ".").
func ReadDiskUsage(path string) (totalGB, usedGB, freeGB, pct float64, status string) {
	if path == "" {
		path = "."
	}
	var st syscall.Statfs_t
	if err := syscall.Statfs(path, &st); err != nil {
		return 0, 0, 0, 0, "ok"
	}
	total := st.Blocks * uint64(st.Bsize)
	if total == 0 {
		return 0, 0, 0, 0, "ok"
	}
	free := st.Bavail * uint64(st.Bsize)
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
