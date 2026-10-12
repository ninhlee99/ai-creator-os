//go:build !windows

package automation

import (
	"fmt"
	"syscall"
)

// DiskUsagePct trả về % đĩa đã dùng tại path. Pure Go (syscall.Statfs),
// chạy được trên linux/darwin, không cgo.
func DiskUsagePct(path string) (int, error) {
	var st syscall.Statfs_t
	if err := syscall.Statfs(path, &st); err != nil {
		return 0, err
	}
	total := st.Blocks * uint64(st.Bsize)
	if total == 0 {
		return 0, fmt.Errorf("không đo được dung lượng đĩa")
	}
	free := st.Bavail * uint64(st.Bsize)
	used := total - free
	return int(used * 100 / total), nil
}
