// Package logx ghi log ra file có tự xoay theo dung lượng (Đợt R).
//
// Vì sao cần: launchd (macOS) mở StandardOutPath một lần lúc start job và
// giữ file descriptor — công cụ xoay log ngoài như newsyslog rename file
// nhưng launchd vẫn ghi vào inode cũ, file "đã xoay" tiếp tục phình vô
// hạn. App tự quản lý file log của mình thì xoay mới thật sự có tác dụng.
//
// Pure Go, không dependency mới: giữ đúng luật "1 binary duy nhất".
package logx

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
)

// Mặc định: mỗi file 20MB, giữ 7 bản cũ (tương đương chính sách newsyslog
// cũ trong deploy/README.md — giờ do app tự thực thi).
const (
	DefaultMaxSize   = 20 << 20 // 20MB
	DefaultMaxBackup = 7
)

// Rotator là io.Writer thread-safe: ghi vào file hiện tại, tự xoay khi
// vượt maxSize. aicos.log -> aicos.log.1 -> ... -> aicos.log.N.
type Rotator struct {
	mu        sync.Mutex
	path      string
	maxSize   int64
	maxBackup int
	file      *os.File
	size      int64
}

// New mở (tạo) file log tại path. Tạo thư mục cha nếu chưa có.
func New(path string, maxSize int64, maxBackup int) (*Rotator, error) {
	if maxSize <= 0 {
		maxSize = DefaultMaxSize
	}
	if maxBackup < 0 {
		maxBackup = 0
	}
	if dir := filepath.Dir(path); dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("logx: tạo thư mục log: %w", err)
		}
	}
	r := &Rotator{path: path, maxSize: maxSize, maxBackup: maxBackup}
	if err := r.open(); err != nil {
		return nil, err
	}
	return r, nil
}

func (r *Rotator) open() error {
	f, err := os.OpenFile(r.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return fmt.Errorf("logx: mở %s: %w", r.path, err)
	}
	fi, err := f.Stat()
	if err != nil {
		f.Close()
		return fmt.Errorf("logx: stat %s: %w", r.path, err)
	}
	r.file = f
	r.size = fi.Size()
	// File đã phình từ trước (vd: do bản cũ/newsyslog) → xoay ngay.
	if r.size >= r.maxSize {
		if err := r.rotate(); err != nil {
			return err
		}
	}
	return nil
}

// Write ghi p vào log, xoay khi cần. Không bao giờ trả lỗi ra ngoài để
// không làm chết app chỉ vì ghi log hỏng — log là phụ, app là chính.
func (r *Rotator) Write(p []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.file == nil {
		return len(p), nil
	}
	if r.size+int64(len(p)) > r.maxSize {
		if err := r.rotate(); err != nil {
			// Xoay hỏng: ghi tiếp vào file cũ còn hơn mất log.
			return r.file.Write(p)
		}
	}
	n, err := r.file.Write(p)
	r.size += int64(n)
	return n, err
}

// rotate đóng file hiện tại, đẩy .N -> .N+1, mở file mới.
func (r *Rotator) rotate() error {
	if r.file != nil {
		r.file.Close()
		r.file = nil
	}
	if r.maxBackup > 0 {
		// Xóa bản cũ nhất, đẩy các bản còn lại lên một nấc.
		_ = os.Remove(fmt.Sprintf("%s.%d", r.path, r.maxBackup))
		for i := r.maxBackup - 1; i >= 1; i-- {
			_ = os.Rename(fmt.Sprintf("%s.%d", r.path, i),
				fmt.Sprintf("%s.%d", r.path, i+1))
		}
		_ = os.Rename(r.path, r.path+".1")
	} else {
		_ = os.Remove(r.path)
	}
	f, err := os.OpenFile(r.path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	r.file = f
	r.size = 0
	return nil
}

// Close đóng file log.
func (r *Rotator) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.file == nil {
		return nil
	}
	err := r.file.Close()
	r.file = nil
	return err
}

// Path trả về đường dẫn file log đang ghi.
func (r *Rotator) Path() string { return r.path }

var _ io.Writer = (*Rotator)(nil)
