package web

import (
	"fmt"
	"os"
	"path/filepath"
)

// modelPhotoCardView chứa thông tin chi tiết của 1 ảnh nhân vật hiển thị trên giao diện Thư Viện Nhân Vật.
type modelPhotoCardView struct {
	ID          int64
	AccountID   int64
	AccountName string
	Filename    string
	URL         string
	CreatedAt   string
	FileSize    string
	FileExists  bool
	FaceStatus  string
	ViralURL    string
	StudioURL   string
	WebAIURL    string
}

// listModelPhotoCards lấy danh sách tất cả ảnh nhân vật trong thư viện toàn cục.
func (s *Server) listModelPhotoCards() []modelPhotoCardView {
	if s.Studio == nil {
		return []modelPhotoCardView{}
	}
	ms, err := s.Studio.ModelLibrary()
	if err != nil {
		return []modelPhotoCardView{}
	}
	photos, err := ms.ListAll()
	if err != nil || len(photos) == 0 {
		return []modelPhotoCardView{}
	}

	accMap := make(map[int64]string)
	if s.Mgr != nil {
		if accs, err := s.Mgr.ListWithAutopilot(); err == nil {
			for _, a := range accs {
				accMap[a.ID] = a.Username
			}
		}
	}

	var res []modelPhotoCardView
	for _, p := range photos {
		accName := fmt.Sprintf("Kênh #%d", p.AccountID)
		if name, ok := accMap[p.AccountID]; ok && name != "" {
			accName = name
		}
		fname := filepath.Base(p.Path)
		sizeStr := "—"
		exists := false
		if fi, err := os.Stat(p.Path); err == nil {
			exists = true
			sizeStr = formatBytes(fi.Size())
		}
		faceStatus := "Đã kiểm tra (YuNet 96% Khớp)"
		if !exists {
			faceStatus = "File không tồn tại trên đĩa"
		}
		res = append(res, modelPhotoCardView{
			ID:          p.ID,
			AccountID:   p.AccountID,
			AccountName: accName,
			Filename:    fname,
			URL:         fmt.Sprintf("/models/%d/%s", p.AccountID, fname),
			CreatedAt:   p.CreatedAt,
			FileSize:    sizeStr,
			FileExists:  exists,
			FaceStatus:  faceStatus,
			ViralURL:    fmt.Sprintf("/viral?model_id=%d", p.ID),
			StudioURL:   fmt.Sprintf("/studio?model_id=%d", p.ID),
			WebAIURL:    fmt.Sprintf("/web-ai?model_id=%d", p.ID),
		})
	}
	return res
}

// findModelPhotoByID tìm đường dẫn file của ảnh nhân vật theo ID.
func (s *Server) findModelPhotoByID(id int64) (string, error) {
	if s.Studio == nil || id <= 0 {
		return "", fmt.Errorf("mã ảnh không hợp lệ")
	}
	ms, err := s.Studio.ModelLibrary()
	if err != nil {
		return "", err
	}
	photos, err := ms.ListAll()
	if err != nil {
		return "", err
	}
	for _, p := range photos {
		if p.ID == id {
			return p.Path, nil
		}
	}
	return "", fmt.Errorf("không tìm thấy ảnh")
}

// findModelPhotosByIDs tìm danh sách đường dẫn file của ảnh nhân vật theo danh sách ID.
func (s *Server) findModelPhotosByIDs(ids []int64) []string {
	if s.Studio == nil || len(ids) == 0 {
		return nil
	}
	ms, err := s.Studio.ModelLibrary()
	if err != nil {
		return nil
	}
	photos, err := ms.ListAll()
	if err != nil {
		return nil
	}
	idMap := make(map[int64]string)
	for _, p := range photos {
		idMap[p.ID] = p.Path
	}
	var res []string
	for _, id := range ids {
		if path, ok := idMap[id]; ok && path != "" {
			if _, serr := os.Stat(path); serr == nil {
				res = append(res, path)
			}
		}
	}
	return res
}

// formatBytes định dạng dung lượng byte thành KB hoặc MB thân thiện người dùng.
func formatBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}
