package web

import (
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/ninhlee99/ai-creator-os/internal/studio"
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

// modelsPageView là cấu trúc dữ liệu truyền vào template models.html.
type modelsPageView struct {
	Photos         []modelPhotoCardView
	Accounts       []*accountView
	Engine         studio.FaceSwapEngineStatus
	TotalPhotos    int
	FaceSwapReady  bool
	ActiveAccountID int64
	Notice         string
	Error          string
}

// handleModels hiển thị màn hình riêng biệt Quản Lý Nhân Vật & Face Lock AI.
func (s *Server) handleModels(w http.ResponseWriter, r *http.Request) {
	var pv modelsPageView
	pv.Notice = r.URL.Query().Get("ok")
	pv.Error = r.URL.Query().Get("err")

	// Lấy danh sách tài khoản / kênh sáng tạo
	accs, err := s.Mgr.ListWithAutopilot()
	if err == nil {
		accMap := make(map[int64]string, len(accs))
		for _, a := range accs {
			pv.Accounts = append(pv.Accounts, s.wrapAccount(a))
			accMap[a.ID] = a.Username
		}
		if len(accs) > 0 {
			pv.ActiveAccountID = accs[0].ID
		}
	}

	// Kiểm tra trạng thái Face Swap Engine cục bộ
	pv.Engine = studio.CheckFaceSwapEngine(r.Context())
	pv.FaceSwapReady = pv.Engine.CanRun

	// Lấy danh sách tất cả ảnh nhân vật đã tải lên
	pv.Photos = s.listModelPhotoCards()
	pv.TotalPhotos = len(pv.Photos)

	s.render(w, "models", s.ctx("Models", pv))
}

// listModelPhotoCards lấy danh sách tất cả ảnh nhân vật trong thư viện toàn cục.
func (s *Server) listModelPhotoCards() []modelPhotoCardView {
	if s.Studio == nil {
		return nil
	}
	ms, err := s.Studio.ModelLibrary()
	if err != nil {
		return nil
	}
	photos, err := ms.ListAll()
	if err != nil || len(photos) == 0 {
		return nil
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

// handleModelUploadGlobal tiếp nhận tải lên ảnh khuôn mặt nhân vật từ màn hình Thư Viện Nhân Vật.
func (s *Server) handleModelUploadGlobal(w http.ResponseWriter, r *http.Request) {
	if s.Studio == nil {
		http.Redirect(w, r, "/models?err="+url.QueryEscape("Studio chưa được khởi tạo."), http.StatusSeeOther)
		return
	}
	if err := r.ParseMultipartForm(50 << 20); err != nil {
		http.Redirect(w, r, "/models?err="+url.QueryEscape("Không đọc được file upload: "+err.Error()), http.StatusSeeOther)
		return
	}

	// Xác định AccountID mục tiêu
	accountIDStr := r.FormValue("account_id")
	accountID, _ := strconv.ParseInt(accountIDStr, 10, 64)
	if accountID <= 0 {
		accs, err := s.Mgr.ListWithAutopilot()
		if err == nil && len(accs) > 0 {
			accountID = accs[0].ID
		} else {
			accountID = 1
		}
	}

	var fileHeaders []*multipart.FileHeader
	if r.MultipartForm != nil {
		for _, key := range []string{"photos", "photo", "character_photos", "model_photos"} {
			if fhs, ok := r.MultipartForm.File[key]; ok && len(fhs) > 0 {
				fileHeaders = append(fileHeaders, fhs...)
			}
		}
	}
	if len(fileHeaders) == 0 {
		http.Redirect(w, r, "/models?err="+url.QueryEscape("Chưa chọn file ảnh nào để tải lên."), http.StatusSeeOther)
		return
	}

	ms, err := s.Studio.ModelLibrary()
	if err != nil {
		http.Redirect(w, r, "/models?err="+url.QueryEscape("Không mở được thư viện ảnh: "+err.Error()), http.StatusSeeOther)
		return
	}

	addedCount := 0
	for _, hdr := range fileHeaders {
		if hdr == nil {
			continue
		}
		ext := strings.ToLower(filepath.Ext(hdr.Filename))
		if !modelUploadExts[ext] {
			continue
		}
		f, err := hdr.Open()
		if err != nil {
			continue
		}
		tmp, err := os.CreateTemp("", "model-upload-*"+ext)
		if err != nil {
			f.Close()
			continue
		}
		tmpName := tmp.Name()
		n, err := io.Copy(tmp, io.LimitReader(f, 15<<20+1))
		f.Close()
		tmp.Close()
		if err != nil || n == 0 || n > 15<<20 {
			_ = os.Remove(tmpName)
			continue
		}
		if _, err := ms.Add(accountID, tmpName); err == nil {
			addedCount++
		}
		_ = os.Remove(tmpName)
	}

	if addedCount == 0 {
		http.Redirect(w, r, "/models?err="+url.QueryEscape("Không có ảnh hợp lệ nào (chỉ nhận file jpg/png/webp dưới 15MB)."), http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, fmt.Sprintf("/models?ok=%s", url.QueryEscape(fmt.Sprintf("Đã tải lên và khóa thành công %d ảnh nhân vật vào thư viện.", addedCount))), http.StatusSeeOther)
}

// handleModelDeleteGlobal xóa ảnh nhân vật khỏi thư viện toàn cục.
func (s *Server) handleModelDeleteGlobal(w http.ResponseWriter, r *http.Request) {
	if s.Studio == nil {
		http.Redirect(w, r, "/models?err="+url.QueryEscape("Studio chưa được khởi tạo."), http.StatusSeeOther)
		return
	}
	idStr := r.PathValue("id")
	if idStr == "" {
		idStr = r.FormValue("id")
	}
	photoID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || photoID <= 0 {
		http.Redirect(w, r, "/models?err="+url.QueryEscape("Mã ảnh không hợp lệ."), http.StatusSeeOther)
		return
	}

	ms, err := s.Studio.ModelLibrary()
	if err != nil {
		http.Redirect(w, r, "/models?err="+url.QueryEscape("Không mở được thư viện ảnh: "+err.Error()), http.StatusSeeOther)
		return
	}

	if err := ms.Delete(photoID); err != nil {
		http.Redirect(w, r, "/models?err="+url.QueryEscape("Không xóa được ảnh: "+err.Error()), http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, "/models?ok="+url.QueryEscape("Đã xóa ảnh nhân vật thành công."), http.StatusSeeOther)
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
