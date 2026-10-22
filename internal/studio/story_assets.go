package studio

import (
	"fmt"
	"os"
	"path/filepath"
)

// findDonePhoto tìm ảnh đã vẽ xong của job cũ có cùng prompt — để job
// retry tái dùng, khỏi đốt image-gen vẽ lại (đợt Y). "" = không có.
func (s *Studio) findDonePhoto(jobID, prompt string) string {
	var path string
	err := s.db.QueryRow(`SELECT path FROM studio_assets
		WHERE job_id=? AND kind='photo' AND status=? AND prompt=? LIMIT 1`,
		jobID, StatusDone, prompt).Scan(&path)
	if err != nil || path == "" {
		return ""
	}
	if fi, statErr := os.Stat(path); statErr != nil || fi.IsDir() {
		return ""
	}
	return path
}

// findDoneAudio tìm file wav đã tổng hợp xong của job cũ — để job retry tái
// dùng, khỏi đốt quota TTS tổng hợp lại (đợt AC). Scene retry dùng cùng params
// nên scene thứ i có text giống nhau; chỉ cần file tồn tại và >0 byte.
func (s *Studio) findDoneAudio(jobID string, sceneIdx int) string {
	if jobID == "" {
		return ""
	}
	p := filepath.Join(s.workRoot, jobID, fmt.Sprintf("scene-%02d.wav", sceneIdx))
	fi, err := os.Stat(p)
	if err != nil || fi.IsDir() || fi.Size() == 0 {
		return ""
	}
	return p
}
func copyStoryFile(src, dst string) error {
	b, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, b, 0o644)
}
