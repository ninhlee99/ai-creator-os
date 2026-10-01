package web

import (
	"context"
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
	"time"

	"github.com/ninhlee99/ai-creator-os/internal/products"
)

// ---------------------------------------------------------------------------
// Affiliate autopilot: product discovery, per-account theme policy, model
// photo library, one-click hands-off runs.
// ---------------------------------------------------------------------------

// themeOption is one entry of the theme dropdown.
type themeOption struct {
	Slug  string
	Label string
}

// themeOptions lists the curated themes in deterministic order.
func themeOptions() []themeOption {
	out := make([]themeOption, 0, len(products.ThemeOrder))
	for _, slug := range products.ThemeOrder {
		label := slug
		if t, ok := products.DescribeTheme(slug); ok {
			label = t.Label
		}
		out = append(out, themeOption{Slug: slug, Label: label})
	}
	return out
}

// resolveMinCommission parses the "min_commission" percent field; an empty
// value falls back to the theme's suggested floor.
func resolveMinCommission(theme, raw string) float64 {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		if t, ok := products.DescribeTheme(theme); ok {
			return t.DefaultMinCommission * 100
		}
		return 0
	}
	p, _ := strconv.ParseFloat(raw, 64)
	if p < 0 {
		p = 0
	}
	if p > 100 {
		p = 100
	}
	return p
}

// ---------------------------------------------------------------- products

// handleProducts renders the product discovery page: theme + commission
// floor form, plus the best saved products for the selected theme.
func (s *Server) handleProducts(w http.ResponseWriter, r *http.Request) {
	if s.Products == nil {
		s.render(w, "products", s.ctx(
			"Themes", themeOptions(),
			"Error", "Kho sản phẩm chưa được khởi tạo. Hãy kiểm tra cấu hình app rồi thử lại.",
		))
		return
	}
	theme := strings.TrimSpace(r.URL.Query().Get("theme"))
	minPct := resolveMinCommission(theme, r.URL.Query().Get("min"))
	var results []products.Product
	if theme != "" {
		var err error
		results, err = s.Products.TopByTheme(theme, minPct/100, 0, 30)
		if err != nil {
			s.render(w, "products", s.ctx(
				"Themes", themeOptions(),
				"Theme", theme,
				"MinPct", minPct,
				"Error", "Không đọc được kho sản phẩm: "+err.Error(),
			))
			return
		}
	}
	enabled, hours, lastRun, music := s.scheduleView()
	s.render(w, "products", s.ctx(
		"Themes", themeOptions(),
		"Theme", theme,
		"MinPct", minPct,
		"Results", results,
		"Notice", r.URL.Query().Get("ok"),
		"SchedEnabled", enabled,
		"SchedHours", hours,
		"SchedLastRun", lastRun,
		"MusicName", music,
	))
}

// handleProductsSearch runs the configured providers for a theme, saves
// every hit into the product store, then displays TopByTheme. Provider
// problems are shown honestly instead of hidden.
func (s *Server) handleProductsSearch(w http.ResponseWriter, r *http.Request) {
	if s.Products == nil {
		s.render(w, "products", s.ctx(
			"Themes", themeOptions(),
			"Error", "Kho sản phẩm chưa được khởi tạo. Hãy kiểm tra cấu hình app rồi thử lại.",
		))
		return
	}
	if err := r.ParseForm(); err != nil {
		s.fail(w, err, "parse products search form")
		return
	}
	theme := strings.TrimSpace(r.PostFormValue("theme"))
	minPct := resolveMinCommission(theme, r.PostFormValue("min_commission"))
	minComm := minPct / 100
	if theme == "" {
		seeOther(w, r, "/products")
		return
	}
	if len(s.ProductProviders) == 0 {
		s.render(w, "products", s.ctx(
			"Themes", themeOptions(),
			"Theme", theme,
			"MinPct", minPct,
			"Error", "Chưa cấu hình nguồn sản phẩm. Hãy thêm provider (ví dụ: TikTok Shop) rồi thử lại.",
		))
		return
	}
	var warnings []string
	for _, p := range s.ProductProviders {
		if c, ok := p.(interface{ Configured() bool }); ok && !c.Configured() {
			warnings = append(warnings,
				fmt.Sprintf("Nguồn %q chưa cấu hình (thiếu API key) — đã bỏ qua.", p.Name()))
		}
	}
	q := products.QueryForTheme(theme, minComm, 50)
	ctx, cancel := context.WithTimeout(r.Context(), 90*time.Second)
	defer cancel()
	found, errs := products.Aggregate(ctx, s.ProductProviders, q)
	for _, e := range errs {
		warnings = append(warnings, e.Error())
	}
	saved := 0
	for _, p := range found {
		p.Theme = theme
		if _, err := s.Products.Save(p); err != nil {
			log.Printf("web: save product: %v", err)
			continue
		}
		saved++
	}
	results, rerr := s.Products.TopByTheme(theme, minComm, 0, 30)
	data := s.ctx(
		"Themes", themeOptions(),
		"Theme", theme,
		"MinPct", minPct,
		"Results", results,
		"Searched", true,
		"Found", len(found),
		"Saved", saved,
		"Warnings", warnings,
	)
	if rerr != nil {
		data["Error"] = "Không đọc được kho sản phẩm: " + rerr.Error()
	}
	s.render(w, "products", data)
}

// ------------------------------------------------- account autopilot policy

// accountRedirect goes back to the account page carrying an optional
// success notice (?ok=) or error (?err=).
func (s *Server) accountRedirect(w http.ResponseWriter, r *http.Request, id int64, notice, errMsg string) {
	loc := "/accounts/" + strconv.FormatInt(id, 10)
	q := url.Values{}
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

var modelUploadExts = map[string]bool{
	".jpg": true, ".jpeg": true, ".png": true, ".webp": true,
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

var modelFileRe = regexp.MustCompile(`^[A-Za-z0-9_.-]+\.(jpg|jpeg|png|webp)$`)

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

// ------------------------------------------------- autopilot schedule (UI-managed)

// The autopilot background schedule is managed in the web UI (not via env
// or CLI): these settings live in the products store. Exported so cmd/aicos
// (the scheduler loop) reads the same keys the UI writes.
const (
	SettingAutopilotEnabled  = "autopilot_enabled"
	SettingAutopilotInterval = "autopilot_interval_hours"
	SettingAutopilotLastRun  = "autopilot_last_run"
	SettingAutopilotMusic    = "autopilot_music_name"
)

const (
	setSchedEnabled  = SettingAutopilotEnabled
	setSchedInterval = SettingAutopilotInterval
	setSchedLastRun  = SettingAutopilotLastRun
	setMusicName     = SettingAutopilotMusic
)

// autopilotMusicPath is where the UI-uploaded music bed lives.
func autopilotMusicPath(databasePath string) string {
	dir := "."
	if databasePath != "" {
		dir = filepath.Dir(databasePath)
	}
	return filepath.Join(dir, "autopilot-music.m4a")
}

// scheduleView reads the current schedule state for the products page.
func (s *Server) scheduleView() (enabled bool, hours int, lastRun, music string) {
	hours = 6
	if s.Products == nil {
		return
	}
	if v, ok := s.Products.GetSetting(setSchedEnabled); ok && v == "1" {
		enabled = true
	}
	if v, ok := s.Products.GetSetting(setSchedInterval); ok {
		if h, err := strconv.Atoi(v); err == nil && h >= 1 && h <= 168 {
			hours = h
		}
	}
	if v, ok := s.Products.GetSetting(setSchedLastRun); ok && v != "" {
		lastRun = v
	}
	if v, ok := s.Products.GetSetting(setMusicName); ok {
		music = v
	}
	return
}

// handleProductsSchedule saves the autopilot background schedule from the
// web UI: enable toggle + interval in hours.
func (s *Server) handleProductsSchedule(w http.ResponseWriter, r *http.Request) {
	if s.Products == nil {
		s.fail(w, fmt.Errorf("kho sản phẩm chưa khởi tạo"), "save schedule")
		return
	}
	if err := r.ParseForm(); err != nil {
		s.fail(w, err, "parse schedule form")
		return
	}
	enabled := "0"
	if r.PostFormValue("enabled") == "on" {
		enabled = "1"
	}
	hours := 6
	if h, err := strconv.Atoi(strings.TrimSpace(r.PostFormValue("interval_hours"))); err == nil && h >= 1 && h <= 168 {
		hours = h
	}
	if err := s.Products.SetSetting(setSchedEnabled, enabled); err != nil {
		s.fail(w, err, "save schedule")
		return
	}
	if err := s.Products.SetSetting(setSchedInterval, strconv.Itoa(hours)); err != nil {
		s.fail(w, err, "save schedule")
		return
	}
	msg := "Đã tắt lịch autopilot."
	if enabled == "1" {
		msg = fmt.Sprintf("Đã bật lịch autopilot — chạy mỗi %d giờ cho các account đã sẵn sàng.", hours)
	}
	seeOther(w, r, "/products?ok="+url.QueryEscape(msg))
}

var audioFileRe = regexp.MustCompile(`(?i)\.(mp3|m4a|wav|ogg|aac)$`)

// handleAutopilotMusicUpload replaces the autopilot music bed from the UI.
// Stored next to the database; the scheduler picks it up automatically.
func (s *Server) handleAutopilotMusicUpload(w http.ResponseWriter, r *http.Request) {
	if s.Products == nil {
		s.fail(w, fmt.Errorf("kho sản phẩm chưa khởi tạo"), "upload music")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 25<<20) // 25 MB cap
	if err := r.ParseMultipartForm(25 << 20); err != nil {
		s.render(w, "products", s.ctx(
			"Themes", themeOptions(),
			"Error", "File nhạc quá lớn hoặc lỗi upload (tối đa 25MB).",
		))
		return
	}
	f, hdr, err := r.FormFile("music")
	if err != nil {
		s.render(w, "products", s.ctx(
			"Themes", themeOptions(),
			"Error", "Chưa chọn file nhạc.",
		))
		return
	}
	defer f.Close()
	if !audioFileRe.MatchString(hdr.Filename) {
		s.render(w, "products", s.ctx(
			"Themes", themeOptions(),
			"Error", "Chỉ nhận file nhạc mp3/m4a/wav/ogg/aac.",
		))
		return
	}
	dst := autopilotMusicPath(s.Cfg.DatabasePath)
	if dir := filepath.Dir(dst); dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			s.fail(w, err, "upload music")
			return
		}
	}
	out, err := os.Create(dst)
	if err != nil {
		s.fail(w, err, "upload music")
		return
	}
	if _, err := io.Copy(out, f); err != nil {
		out.Close()
		_ = os.Remove(dst)
		s.fail(w, err, "upload music")
		return
	}
	out.Close()
	_ = s.Products.SetSetting(setMusicName, hdr.Filename)
	if s.Autopilot != nil {
		s.Autopilot.MusicPath = dst
	}
	seeOther(w, r, "/products?ok="+url.QueryEscape("Đã cập nhật nhạc nền autopilot: "+hdr.Filename))
}
