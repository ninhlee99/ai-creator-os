package web

// Per-account affiliate autopilot configuration: theme policy, hands-off
// toggle and run, plus the model photo library used by the director.

import (
	"fmt"
	"io"
	"log"
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
	q.Set("tab", "autopilot")
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
	// Zero-touch (Đợt 3): a fresh theme means a fresh 30-day plan.
	if a, err := s.Mgr.Get(id); err == nil {
		s.autoGeneratePlan(a, "Tự sinh khi đổi chủ đề")
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
			fmt.Sprintf("Đã bật autopilot (hoa hồng tối thiểu %.1f%%).", pct), "")
		return
	}
	s.accountRedirect(w, r, id, "Đã tắt autopilot.", "")
}

// handleAccountAutopilotRun runs one hands-off cycle immediately and lands
// on the studio job page. A backend failure is shown on the account page.
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

// handleAccountModelUpload stores one uploaded model photo into the
// account's library (identity-lock ground truth). Images only, max 10MB.
func (s *Server) handleAccountModelUpload(w http.ResponseWriter, r *http.Request) {
	id, ok := s.accountID(w, r)
	if !ok {
		return
	}
	if s.Studio == nil {
		s.accountRedirect(w, r, id, "", "Studio chưa được khởi tạo nên chưa upload được ảnh mẫu.")
		return
	}
	if err := r.ParseMultipartForm(12 << 20); err != nil {
		s.accountRedirect(w, r, id, "", "Không đọc được file upload: "+err.Error())
		return
	}
	f, hdr, err := r.FormFile("photo")
	if err != nil {
		s.accountRedirect(w, r, id, "", "Chưa chọn file ảnh nào.")
		return
	}
	defer f.Close()
	ext := strings.ToLower(filepath.Ext(hdr.Filename))
	if !modelUploadExts[ext] {
		s.accountRedirect(w, r, id, "", "Chỉ nhận file ảnh jpg/png/webp.")
		return
	}
	tmp, err := os.CreateTemp("", "model-upload-*"+ext)
	if err != nil {
		s.accountRedirect(w, r, id, "", "Không lưu được file tạm: "+err.Error())
		return
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName) // the store copies the file into the library
	n, err := io.Copy(tmp, io.LimitReader(f, 10<<20+1))
	tmp.Close()
	if err != nil {
		s.accountRedirect(w, r, id, "", "Không đọc được file ảnh: "+err.Error())
		return
	}
	if n > 10<<20 {
		s.accountRedirect(w, r, id, "", "File quá lớn — tối đa 10MB.")
		return
	}
	ms, err := s.Studio.ModelLibrary()
	if err != nil {
		s.accountRedirect(w, r, id, "", "Không mở được thư viện ảnh: "+err.Error())
		return
	}
	if _, err := ms.Add(id, tmpName); err != nil {
		s.accountRedirect(w, r, id, "", "Không thêm được ảnh mẫu: "+err.Error())
		return
	}
	s.accountRedirect(w, r, id, "Đã thêm ảnh mẫu vào thư viện.", "")
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
	http.ServeFile(w, r, path)
}

var modelFileRe = regexp.MustCompile(`^[A-Za-z0-9_.-]+\.(jpg|jpeg|png|webp)$`)

// handleModelPhoto serves a model library photo. The account segment must
// be numeric and the file name must match the image allowlist; the
// resolved path is verified to stay under the models dir (no traversal).

var modelUploadExts = map[string]bool{
	".jpg": true, ".jpeg": true, ".png": true, ".webp": true,
}

// handleAccountModelUpload stores one uploaded model photo into the
// account's library (identity-lock ground truth). Images only, max 10MB.
