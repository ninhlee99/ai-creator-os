package studio

// Xuất gói chỉnh sửa cho CapCut (bán tự động).
//
// CapCut không có API, và file nháp dự án (draft_content.json) bị mã hóa
// từ CapCut 6 — tự viết dự án CapCut không ổn định. Cách chắc chắn: gom
// video đã dựng + ảnh/clip từng cảnh vào 1 thư mục, mở Finder tới
// đó và bật CapCut để người dùng kéo thả vào timeline chỉnh tiếp.

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var unsafeName = regexp.MustCompile(`[^\p{L}\p{N}_-]+`)

// ExportForEditing copy file của job vào destRoot/<tên>-<id>/ và trả về
// thư mục đó. Thứ tự file giữ theo cảnh (01-, 02-…) để kéo vào timeline
// đúng thứ tự.
func (s *Studio) ExportForEditing(id, destRoot string) (string, error) {
	j, ok := s.GetJob(id)
	if !ok {
		return "", fmt.Errorf("không tìm thấy job")
	}
	if j.Output == "" {
		return "", fmt.Errorf("job chưa có video để xuất")
	}
	name := strings.Trim(unsafeName.ReplaceAllString(j.Title, "-"), "-")
	if name == "" {
		name = "video"
	}
	dir := filepath.Join(destRoot, name+"-"+id)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	if err := copyStudioFile(filepath.Join(s.outDir, j.Output), filepath.Join(dir, "00-video-hoan-chinh"+filepath.Ext(j.Output))); err != nil {
		return "", fmt.Errorf("copy video: %w", err)
	}
	n := 1
	for _, a := range s.ListAssets(id) {
		if a.Status != StatusDone || a.Path == "" {
			continue
		}
		dst := filepath.Join(dir, fmt.Sprintf("%02d-%s%s", n, a.Kind, filepath.Ext(a.Path)))
		if copyStudioFile(a.Path, dst) == nil {
			n++
		}
	}
	if j.Caption != "" {
		_ = os.WriteFile(filepath.Join(dir, "caption.txt"), []byte(j.Caption), 0o644)
	}
	return dir, nil
}
