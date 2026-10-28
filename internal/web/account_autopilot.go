package web

// Per-account affiliate autopilot configuration: theme policy, saved
// hands-off settings, plus the model photo library used by the director.

import (
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/ninhlee99/ai-creator-os/internal/products"
)

// accountRedirect goes back to the account page carrying an optional
// success notice (?ok=) or error (?err=).
func (s *Server) accountRedirect(w http.ResponseWriter, r *http.Request, id int64, notice, errMsg string) {
	loc := "/accounts/" + strconv.FormatInt(id, 10)
	q := url.Values{}
	tab := r.FormValue("tab")
	if tab == "" {
		tab = r.URL.Query().Get("tab")
	}
	if tab == "" {
		tab = "model-photos"
	}
	tab = accountTabName(tab)
	q.Set("tab", tab)
	if notice != "" {
		q.Set("ok", notice)
	}
	if errMsg != "" {
		q.Set("err", errMsg)
	}
	if enc := q.Encode(); enc != "" {
		loc += "?" + enc
	}
	seeOther(w, r, loc)
}

// handleAccountTheme saves the account's affiliate theme.
func (s *Server) handleAccountTheme(w http.ResponseWriter, r *http.Request) {
	id, ok := s.accountID(w, r)
	if !ok {
		return
	}
	_ = r.ParseForm()
	theme := strings.TrimSpace(r.PostFormValue("theme"))
	if _, err := s.Mgr.SetTheme(id, theme); err != nil {
		s.accountRedirect(w, r, id, "", "Không lưu được chủ đề: "+err.Error())
		return
	}
	if theme == "" {
		s.accountRedirect(w, r, id, "Đã xóa chủ đề của tài khoản.", "")
		return
	}
	s.accountRedirect(w, r, id, "Đã lưu chủ đề: "+themeLabelOf(theme)+".", "")
}

func themeLabelOf(slug string) string {
	if t, ok := products.DescribeTheme(slug); ok {
		return t.Label
	}
	return slug
}

// handleAccountAutopilot toggles hands-off mode and the commission floor.
// The form posts the floor as a percent; it is stored as 0..1.
func (s *Server) handleAccountAutopilot(w http.ResponseWriter, r *http.Request) {
	id, ok := s.accountID(w, r)
	if !ok {
		return
	}
	_ = r.ParseForm()
	enabled := r.PostFormValue("enabled") == "on"
	pct, _ := strconv.ParseFloat(strings.TrimSpace(r.PostFormValue("min_commission")), 64)
	if pct < 0 {
		pct = 0
	}
	if pct > 100 {
		pct = 100
	}
	if _, err := s.Mgr.SetAutopilot(id, enabled, pct/100); err != nil {
		s.accountRedirect(w, r, id, "", "Không lưu được cấu hình autopilot: "+err.Error())
		return
	}
	if enabled {
		s.accountRedirect(w, r, id,
			fmt.Sprintf("Đã lưu cấu hình Autopilot bật (hoa hồng tối thiểu %.1f%%); producer hiện tạm dừng vì chưa có xác nhận dùng Muse AI credits theo kênh.", pct), "")
		return
	}
	s.accountRedirect(w, r, id, "Đã tắt autopilot.", "")
}

// handleAccountAutopilotRun preserves the old route but reports the production
// gate while automatic Flow credit authorization is unavailable.
func (s *Server) handleAccountAutopilotRun(w http.ResponseWriter, r *http.Request) {
	id, ok := s.accountID(w, r)
	if !ok {
		return
	}
	if s.Autopilot == nil {
		s.accountRedirect(w, r, id, "",
			"Autopilot chưa được khởi tạo — app đang chạy thiếu cấu hình.")
		return
	}
	res, err := s.Autopilot.Run(r.Context(), id)
	if err != nil {
		s.accountRedirect(w, r, id, "", "Autopilot lỗi: "+err.Error())
		return
	}
	if strings.HasPrefix(res.Product.Title, "LỖI: ") {
		s.accountRedirect(w, r, id, "", res.Product.Title)
		return
	}
	seeOther(w, r, "/studio/jobs/"+res.JobID)
}

// ------------------------------------------------------- model photo library

// modelPhotoView is one library photo with its served URL.
type modelPhotoView struct {
	ID  int64
	URL string
}

// modelPhotoViews lists the account's model photos for the template.
// Empty (never nil) when the studio isn't wired.
func (s *Server) modelPhotoViews(accountID int64) []modelPhotoView {
	out := []modelPhotoView{}
	if s.Studio == nil {
		return out
	}
	ms, err := s.Studio.ModelLibrary()
	if err != nil {
		log.Printf("web: model library: %v", err)
		return out
	}
	photos, err := ms.List(accountID)
	if err != nil {
		log.Printf("web: list model photos: %v", err)
		return out
	}
	for _, p := range photos {
		out = append(out, modelPhotoView{
			ID:  p.ID,
			URL: "/models/" + strconv.FormatInt(accountID, 10) + "/" + filepath.Base(p.Path),
		})
	}
	return out
}

// handleAccountModelUpload stores uploaded model photos into the
// account's library (identity-lock ground truth). Supports uploading multiple
// photos at once. Images only (jpg/png/webp), max 15MB each.
func (s *Server) handleAccountModelUpload(w http.ResponseWriter, r *http.Request) {
	id, ok := s.accountID(w, r)
	if !ok {
		return
	}
	if s.Studio == nil {
		s.accountRedirect(w, r, id, "", "Studio chưa được khởi tạo nên chưa upload được ảnh mẫu.")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, (151 << 20))
	parseErr := r.ParseMultipartForm(50 << 20)
	if r.MultipartForm != nil {
		defer r.MultipartForm.RemoveAll()
	}
	if parseErr != nil {
		s.accountRedirect(w, r, id, "", "Không đọc được file upload: "+parseErr.Error())
		return
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
		s.accountRedirect(w, r, id, "", "Chưa chọn file ảnh nào.")
		return
	}
	if len(fileHeaders) > 10 {
		s.accountRedirect(w, r, id, "", "Tối đa 10 ảnh mỗi lần tải lên.")
		return
	}

	ms, err := s.Studio.ModelLibrary()
	if err != nil {
		s.accountRedirect(w, r, id, "", "Không mở được thư viện ảnh: "+err.Error())
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
		if _, err := ms.Add(id, tmpName); err == nil {
			addedCount++
		}
		_ = os.Remove(tmpName)
	}

	if addedCount == 0 {
		s.accountRedirect(w, r, id, "", "Không có ảnh hợp lệ nào được tải lên (chỉ nhận file jpg/png/webp dưới 15MB).")
		return
	}
	s.accountRedirect(w, r, id, fmt.Sprintf("Đã tải lên & khóa %d ảnh nhân vật vào thư viện.", addedCount), "")
}

// handleAccountModelDelete removes one model photo from the library.
func (s *Server) handleAccountModelDelete(w http.ResponseWriter, r *http.Request) {
	id, ok := s.accountID(w, r)
	if !ok {
		return
	}
	if s.Studio == nil {
		s.accountRedirect(w, r, id, "", "Studio chưa được khởi tạo.")
		return
	}
	photoID, err := strconv.ParseInt(r.PathValue("photoID"), 10, 64)
	if err != nil {
		s.accountRedirect(w, r, id, "", "Ảnh không hợp lệ.")
		return
	}
	ms, err := s.Studio.ModelLibrary()
	if err != nil {
		s.accountRedirect(w, r, id, "", "Không mở được thư viện ảnh: "+err.Error())
		return
	}
	if err := ms.Delete(photoID); err != nil {
		s.accountRedirect(w, r, id, "", "Không xóa được ảnh: "+err.Error())
		return
	}
	s.accountRedirect(w, r, id, "Đã xóa ảnh mẫu.", "")
}

// handleModelPhoto serves a model library photo. The account segment must
// be numeric and the file name must match the image allowlist; the
// resolved path is verified to stay under the models dir (no traversal).
func (s *Server) handleModelPhoto(w http.ResponseWriter, r *http.Request) {
	if s.Studio == nil {
		http.NotFound(w, r)
		return
	}
	account := r.PathValue("account")
	file := r.PathValue("file")
	if account == "" || file == "" {
		http.NotFound(w, r)
		return
	}
	for _, c := range account {
		if c < '0' || c > '9' {
			http.NotFound(w, r)
			return
		}
	}
	if !modelFileRe.MatchString(file) {
		http.NotFound(w, r)
		return
	}
	root := filepath.Join(s.Studio.WorkDir(), "models", account)
	path := filepath.Join(root, file)
	if !strings.HasPrefix(filepath.Clean(path), filepath.Clean(root)+string(os.PathSeparator)) {
		http.NotFound(w, r)
		return
	}
	if !fileExists(path) {
		cand := filepath.Join("data/output/studio/models", account, file)
		if fileExists(cand) {
			path = cand
		} else {
			http.NotFound(w, r)
			return
		}
	}
	serveMediaFile(w, r, path)
}

var modelFileRe = regexp.MustCompile(`^[A-Za-z0-9_.-]+\.(jpg|jpeg|png|webp)$`)

var modelUploadExts = map[string]bool{
	".jpg": true, ".jpeg": true, ".png": true, ".webp": true,
}
