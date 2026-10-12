// Package lockfile giữ khóa độc quyền cho một thư mục dữ liệu: chỉ một
// process aicos được chạy trên cùng data dir tại một thời điểm (Đợt S).
//
// Khác với file khóa PID: flock của hệ điều hành tự giải phóng khi process
// chết (kể cả crash/kill -9), nên không bao giờ kẹt khóa "ma". Instance thứ
// hai thoát êm với thông báo thân thiện thay vì giành SQLite với instance
// đang chạy.
package lockfile

import "os"

// Lock giữ một file đã khóa. Đóng (Close) để giải phóng.
type Lock struct {
	f    *os.File
	path string
}

// Acquire mở (tạo nếu chưa có) path và giữ khóa độc quyền không chặn.
// Trả về (nil, nil) khi một process khác đang giữ khóa — người gọi nên
// thoát êm, không coi là lỗi.
func Acquire(path string) (*Lock, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	if !tryLock(f) {
		f.Close()
		return nil, nil // đã có instance khác đang chạy
	}
	return &Lock{f: f, path: path}, nil
}

// Close giải phóng khóa và đóng file.
func (l *Lock) Close() error {
	if l == nil || l.f == nil {
		return nil
	}
	unlock(l.f)
	return l.f.Close()
}
