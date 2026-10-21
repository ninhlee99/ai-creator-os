package web

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/ninhlee99/ai-creator-os/internal/automation"
	"github.com/ninhlee99/ai-creator-os/internal/network"
	"github.com/ninhlee99/ai-creator-os/internal/studio"
)

// ---------------------------------------------------------------------------
// Studio page: AI video creation (affiliate) + VN trending sounds
// ---------------------------------------------------------------------------

// studioJobView is the template/JSON projection of a studio job.
// Label/Class are rendered server-side so clients never redefine status
// labels (R2-W3: một nguồn nhãn duy nhất).
type studioJobView struct {
	ID         string `json:"id"`
	Kind       string `json:"kind"`
	Title      string `json:"title"`
	Status     string `json:"status"`
	Progress   int    `json:"progress"`
	Output     string `json:"output"`
	OutputKind string `json:"output_kind"`
	Caption    string `json:"caption"`
	CreatedAt  string `json:"created_at"`
	FinishedAt string `json:"finished_at"`
	KindLabel  string `json:"kind_label"`
	Label      string `json:"label"`
	Class      string `json:"class"`
	QCVerified bool   `json:"qc_verified"`
}

func kindLabel(kind string) string {
	switch kind {
	case studio.KindAffiliate:
		return "Affiliate"
	case "ai_content":
		return "Muse · Flow"
	default:
		return kind
	}
}

func (s *Server) studioJobViews(jobs []studio.Job) []studioJobView {
	out := make([]studioJobView, 0, len(jobs))
	for _, j := range jobs {
		out = append(out, studioJobView{
			ID: j.ID, Kind: j.Kind, Title: j.Title, Status: j.Status,
			Progress: j.Progress, Output: mediaName(j.Output), OutputKind: studioOutputKind(j.Output),
			Caption:   j.Caption,
			CreatedAt: j.CreatedAt, FinishedAt: j.FinishedAt,
			KindLabel:  kindLabel(j.Kind),
			Label:      statusLabel(j.Status),
			Class:      statusClass(j.Status),
			QCVerified: studioOutputQCVerified(j),
		})
	}
	return out
}

func studioOutputKind(output string) string {
	switch strings.ToLower(filepath.Ext(output)) {
	case ".mp4", ".mov", ".webm":
		return "video"
	case ".png", ".jpg", ".jpeg", ".webp":
		return "image"
	case ".md", ".txt":
		return "text"
	default:
		return "file"
	}
}

// Old affiliate/viral jobs can be technically complete without current quality evidence.
// Keep render status separate from production QC so legacy output is not shown as approved.
func studioOutputQCVerified(j studio.Job) bool {
	if j.Kind == "ai_content" {
		return false
	}
	if j.Kind != studio.KindAffiliate && j.Kind != studio.KindViral {
		return true
	}
	if j.Status != studio.StatusDone || strings.TrimSpace(j.Output) == "" {
		return false
	}
	qc, qcAt := pipelineMarkerState(j.Log, "VISUAL_QC_START:", "VISUAL_QC_PASS:", "VISUAL_QC_FAIL:")
	if qc != "pass" {
		return false
	}
	if j.Kind == studio.KindAffiliate {
		analysis, analysisAt := pipelineMarkerState(j.Log, "PRODUCT_ANALYSIS_START:", "PRODUCT_ANALYSIS_PASS:", "PRODUCT_ANALYSIS_FAIL:")
		director, directorAt := pipelineMarkerState(j.Log, "DIRECTOR_START:", "DIRECTOR_PASS:", "DIRECTOR_FAIL:")
		production, productionAt := pipelineMarkerState(j.Log, "PRODUCTION_START:", "PRODUCTION_PASS:", "PRODUCTION_FAIL:")
		return analysis == "pass" && director == "pass" && production == "pass" && analysisAt < directorAt && directorAt < productionAt && productionAt < qcAt
	} else {
		replacement, replacementAt := pipelineMarkerState(j.Log, "CHARACTER_REPLACEMENT_START:", "CHARACTER_REPLACEMENT_PASS:", "CHARACTER_REPLACEMENT_FAIL:")
		validation, validationAt := pipelineMarkerState(j.Log, "OUTPUT_VALIDATION_START:", "OUTPUT_VALIDATION_PASS:", "OUTPUT_VALIDATION_FAIL:")
		return replacement == "pass" && validation == "pass" && replacementAt < validationAt && validationAt < qcAt
	}
}

func pipelineMarkerState(log, startMarker, passMarker, failMarker string) (string, int) {
	startAt := strings.LastIndex(log, startMarker)
	passAt := strings.LastIndex(log, passMarker)
	failAt := strings.LastIndex(log, failMarker)
	if passAt >= 0 && passAt > failAt && passAt > startAt {
		return "pass", passAt
	}
	if failAt >= 0 && failAt > passAt && failAt > startAt {
		return "fail", failAt
	}
	if startAt >= 0 && startAt > passAt && startAt > failAt {
		return "running", startAt
	}
	return "unknown", -1
}

func (s *Server) writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(v)
}

// handleStudio renders the Studio page: creation form, job links, trends.
// Đợt AK: tab "Video chữ động" (kinetic) đã gỡ — trái format affiliate đã
// chốt (ảnh + nhạc, không chữ) và không tự động hóa nào dùng.
func (s *Server) handleStudio(w http.ResponseWriter, r *http.Request) {
	if s.Studio == nil {
		s.render(w, "studio", s.ctx(
			"Error", "Studio chưa được khởi tạo."))
		return
	}
	jobs := s.studioJobViews(s.Studio.ListJobs(30))
	// Zero-touch (Đợt 3 / A6): a stale chart refreshes itself in the
	// background — nobody has to remember the refresh button.
	if studio.TrendsStale() {
		studio.RefreshVNTrendingAsync()
	}
	sounds, trendsErr := studio.FetchVNTrending(r.Context())
	activeJobs, jobCount := 0, len(jobs)
	doneCount := 0
	for _, j := range jobs {
		if j.Status == "running" || j.Status == "queued" {
			activeJobs++
		} else if j.Status == "done" {
			doneCount++
		}
	}
	mgOK := false
	if mg := s.Studio.MediaGen(); mg != nil {
		mgOK = mg.Healthy(r.Context())
	}

	var accountNames []string
	var accountOptions []*network.Account
	if s.Mgr != nil {
		if accounts, err := s.Mgr.ListWithAutopilot(); err == nil {
			accountOptions = accounts
			for _, a := range accounts {
				if a != nil && a.Username != "" {
					accountNames = append(accountNames, a.Username)
				}
			}
		}
	}

	mode := r.URL.Query().Get("mode")
	if mode == "" {
		mode = "affiliate"
	}
	notice := r.URL.Query().Get("notice")
	if notice == "" && r.URL.Query().Get("msg") == "created" {
		notice = "Tác vụ sản xuất video đã được đưa vào hàng đợi AI thành công!"
	}

	var selectedModelID int64
	if midStr := r.URL.Query().Get("model_id"); midStr != "" {
		selectedModelID, _ = strconv.ParseInt(midStr, 10, 64)
	}
	var selectedAccountID int64
	if raw := r.URL.Query().Get("account_id"); raw != "" {
		selectedAccountID, _ = strconv.ParseInt(raw, 10, 64)
	}
	if selectedAccountID == 0 && strings.TrimSpace(r.URL.Query().Get("account")) != "" {
		for _, account := range accountOptions {
			if account != nil && account.Username == strings.TrimSpace(r.URL.Query().Get("account")) {
				selectedAccountID = account.ID
				break
			}
		}
	}
	availableModels := s.listModelPhotoCards()
	if selectedAccountID == 0 && selectedModelID > 0 {
		for _, model := range availableModels {
			if model.ID == selectedModelID {
				selectedAccountID = model.AccountID
				break
			}
		}
	}
	if selectedAccountID == 0 && len(accountOptions) > 0 && accountOptions[0] != nil {
		selectedAccountID = accountOptions[0].ID
	}
	if selectedModelID == 0 {
		for _, model := range availableModels {
			if model.AccountID == selectedAccountID {
				selectedModelID = model.ID
				break
			}
		}
	}
	prefill := map[string]string{
		"ProductName":  r.URL.Query().Get("product_name"),
		"ProductPrice": r.URL.Query().Get("product_price"),
		"Niche":        r.URL.Query().Get("niche"),
		"AffLink":      r.URL.Query().Get("aff_link"),
		"PhotoURL":     r.URL.Query().Get("photo_url"),
	}
	if pid, err := strconv.ParseInt(r.URL.Query().Get("product_id"), 10, 64); err == nil && pid > 0 && s.Products != nil {
		if p, err := s.Products.GetByID(pid); err == nil {
			if prefill["ProductName"] == "" {
				prefill["ProductName"] = p.Title
			}
			if prefill["ProductPrice"] == "" && p.Price > 0 {
				prefill["ProductPrice"] = fmt.Sprintf("%.0f ₫", p.Price)
			}
			if prefill["Niche"] == "" {
				prefill["Niche"] = p.Category
			}
			if prefill["AffLink"] == "" {
				prefill["AffLink"] = p.ProductURL
			}
			if prefill["PhotoURL"] == "" && len(p.ImageURLs) > 0 {
				prefill["PhotoURL"] = p.ImageURLs[0]
			}
		}
	}

	s.render(w, "studio", s.ctx(
		"Jobs", jobs, // giữ field cho tương thích: danh sách job ở /studio/jobs
		"Shelf", s.shelfViews(),
		"JobCount", jobCount,
		"ActiveJobs", activeJobs,
		"RunningCount", activeJobs,
		"DoneCount", doneCount,
		"CurrentTab", "create",
		"InitialMode", mode,
		"Notice", notice,
		"Sounds", sounds,
		"TopSound", topSound(sounds),
		"TrendsSource", studio.TrendsSource,
		"TrendsErr", errText(trendsErr),
		"MediaGenOK", mgOK,
		"AccountNames", accountNames,
		"AccountOptions", accountOptions,
		"SelectedAccountID", selectedAccountID,
		"AvailableModels", availableModels,
		"Prefill", prefill,
		"SelectedModelID", selectedModelID,
		"AudioStoryPlots", studio.GetCuratedAudioStories(""),
		"DefStoryGenre", s.atSettingStr(automation.KeyStoryGenre, "tâm lý"),
		"DefStoryWords", s.atSettingInt(automation.KeyStoryWords, 600),
		"DefStoryScenes", s.atSettingInt(automation.KeyStoryScenes, 6),
		"Error", r.URL.Query().Get("err"),
	))
}

// topSound returns "Artist – Title" of the top-ranked trending sound.
func topSound(sounds []studio.TrendingSound) string {
	if len(sounds) == 0 {
		return ""
	}
	return sounds[0].Artist + " – " + sounds[0].Title
}

func errText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

// saveUpload stores one multipart file into dir, returning its path.
func saveUpload(r *http.Request, field, dir string) (string, error) {
	f, hdr, err := r.FormFile(field)
	if err != nil {
		return "", nil // field absent is fine
	}
	defer f.Close()
	if hdr.Size == 0 {
		return "", nil
	}
	// Đợt AC: báo rõ khi file quá lớn thay vì cắt im lặng 50MB.
	if hdr.Size > 50<<20 {
		return "", fmt.Errorf("file %s quá lớn (%.1fMB, tối đa 50MB)", field, float64(hdr.Size)/(1<<20))
	}
	ext := strings.ToLower(filepath.Ext(hdr.Filename))
	switch ext {
	case ".jpg", ".jpeg", ".png", ".webp", ".mp3", ".m4a", ".wav", ".ogg":
	default:
		return "", fmt.Errorf("file %s: định dạng %s không hỗ trợ", field, ext)
	}
	name := fmt.Sprintf("%d-%s%s", time.Now().UnixNano(), field, ext)
	dst := filepath.Join(dir, name)
	out, err := os.Create(dst)
	if err != nil {
		return "", err
	}
	if _, err := io.Copy(out, io.LimitReader(f, 50<<20)); err != nil {
		out.Close()
		return "", err
	}
	out.Close()
	return dst, nil
}

// saveUploads lưu tối đa max ảnh từ 1 ô chọn nhiều file (chỉ nhận ảnh).
func saveUploads(r *http.Request, field, dir string, max int) ([]string, error) {
	if r.MultipartForm == nil {
		return nil, nil
	}
	var out []string
	for i, hdr := range r.MultipartForm.File[field] {
		if i >= max {
			break
		}
		if hdr.Size == 0 {
			continue
		}
		if hdr.Size > 50<<20 {
			return nil, fmt.Errorf("ảnh %s quá lớn (tối đa 50MB)", hdr.Filename)
		}
		ext := strings.ToLower(filepath.Ext(hdr.Filename))
		switch ext {
		case ".jpg", ".jpeg", ".png", ".webp":
		default:
			return nil, fmt.Errorf("ảnh %s: định dạng %s không hỗ trợ", hdr.Filename, ext)
		}
		f, err := hdr.Open()
		if err != nil {
			return nil, err
		}
		dst := filepath.Join(dir, fmt.Sprintf("%d-%s-%d%s", time.Now().UnixNano(), field, i, ext))
		out2, err := os.Create(dst)
		if err != nil {
			f.Close()
			return nil, err
		}
		_, err = io.Copy(out2, f)
		out2.Close()
		f.Close()
		if err != nil {
			return nil, err
		}
		out = append(out, dst)
	}
	return out, nil
}

// saveMotionClips stores generated motion footage supplied by the operator.
// Affiliate video APIs stay disabled under the project's free-only policy.
func saveMotionClips(r *http.Request, field, dir string, max int) ([]string, error) {
	if r.MultipartForm == nil {
		return nil, nil
	}
	headers := r.MultipartForm.File[field]
	if len(headers) > max {
		return nil, fmt.Errorf("tối đa %d clip chuyển động mỗi video", max)
	}
	const maxClipBytes = 100 << 20
	const maxTotalBytes = 600 << 20
	var total int64
	paths := make([]string, 0, len(headers))
	for i, hdr := range headers {
		if hdr.Size == 0 {
			continue
		}
		if hdr.Size > maxClipBytes {
			return nil, fmt.Errorf("clip %s quá lớn (tối đa 100MB)", hdr.Filename)
		}
		total += hdr.Size
		if total > maxTotalBytes {
			return nil, fmt.Errorf("tổng dung lượng clip vượt 600MB")
		}
		ext := strings.ToLower(filepath.Ext(hdr.Filename))
		switch ext {
		case ".mp4", ".mov", ".webm", ".mkv":
		default:
			return nil, fmt.Errorf("clip %s: định dạng %s không hỗ trợ", hdr.Filename, ext)
		}
		in, err := hdr.Open()
		if err != nil {
			return nil, err
		}
		path := filepath.Join(dir, fmt.Sprintf("%d-motion-%02d%s", time.Now().UnixNano(), i, ext))
		out, err := os.Create(path)
		if err != nil {
			in.Close()
			return nil, err
		}
		written, copyErr := io.Copy(out, io.LimitReader(in, maxClipBytes+1))
		closeInErr := in.Close()
		closeOutErr := out.Close()
		if copyErr != nil || closeInErr != nil || closeOutErr != nil {
			_ = os.Remove(path)
			if copyErr != nil {
				return nil, copyErr
			}
			if closeInErr != nil {
				return nil, closeInErr
			}
			return nil, closeOutErr
		}
		if written > maxClipBytes || written != hdr.Size {
			_ = os.Remove(path)
			return nil, fmt.Errorf("clip %s bị cắt hoặc vượt giới hạn tải lên", hdr.Filename)
		}
		paths = append(paths, path)
	}
	return paths, nil
}

type affiliatePlanShotResponse struct {
	studio.ProShot
	ManualPrompt string `json:"manual_prompt"`
}

// handleStudioAffiliatePlan performs product analysis and director work before
// the operator creates or uploads any motion clips.
func (s *Server) handleStudioAffiliatePlan(w http.ResponseWriter, r *http.Request) {
	if s.Studio == nil {
		s.writeJSON(w, map[string]any{"ok": false, "error": "Studio chưa sẵn sàng"})
		return
	}
	if err := r.ParseMultipartForm(60 << 20); err != nil {
		s.writeJSON(w, map[string]any{"ok": false, "error": "Không đọc được form"})
		return
	}
	p, err := s.affiliatePlanParamsFromRequest(r)
	if err != nil {
		s.writeJSON(w, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	if s.Mgr != nil {
		account, err := s.Mgr.Get(p.AccountID)
		if err != nil {
			s.writeJSON(w, map[string]any{"ok": false, "error": "Không đọc được hồ sơ kênh: " + err.Error()})
			return
		}
		p.ChannelProfile.Model = strings.TrimSpace(account.Persona)
		p.ChannelMemoryKey = strings.TrimSpace(account.Username)
		if p.ChannelProfile.Model == "" {
			p.ChannelProfile.Model = strings.TrimSpace(account.Niche)
		}
	}
	if s.Growth != nil {
		brand, err := s.Growth.GetBrandDNA(p.AccountID)
		if err != nil {
			s.writeJSON(w, map[string]any{"ok": false, "error": "Không đọc được Brand DNA của kênh: " + err.Error()})
			return
		}
		if brand != nil {
			p.ChannelProfile.Positioning = brand.Positioning
			p.ChannelProfile.Audience = brand.AudiencePersona
			p.ChannelProfile.Tone = brand.ToneOfVoice
			p.ChannelProfile.VisualStyle = brand.VisualStyle
			p.ChannelProfile.RedLines = append([]string(nil), brand.RedLines...)
		}
	}
	ctx, cancel := context.WithTimeout(r.Context(), 4*time.Minute)
	defer cancel()
	prep, err := s.Studio.PrepareAffiliatePlan(ctx, p)
	if err != nil {
		s.writeJSON(w, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	shots := make([]affiliatePlanShotResponse, 0, len(prep.Shots))
	for _, shot := range prep.Shots {
		shots = append(shots, affiliatePlanShotResponse{
			ProShot: shot, ManualPrompt: studio.AffiliateManualVideoPrompt(prep.Params.ProductName, shot),
		})
	}
	s.writeJSON(w, map[string]any{
		"ok": true, "plan_id": prep.ID, "seconds": prep.Params.Seconds,
		"product_brief": prep.ProductBrief, "shots": shots,
	})
}

func (s *Server) affiliatePlanParamsFromRequest(r *http.Request) (studio.AffiliateParams, error) {
	var p studio.AffiliateParams
	upDir := s.Studio.UploadDir()
	if err := os.MkdirAll(upDir, 0o755); err != nil {
		return p, fmt.Errorf("không tạo được thư mục upload: %w", err)
	}
	accountID, _ := strconv.ParseInt(strings.TrimSpace(r.FormValue("account_id")), 10, 64)
	if accountID <= 0 {
		return p, fmt.Errorf("chọn kênh nhận diện nhân vật")
	}

	var modelPhotos []string
	for _, key := range []string{"model_photos", "character_photos", "model_photo", "character_photo"} {
		photos, err := saveUploads(r, key, upDir, 10)
		if err != nil {
			return p, fmt.Errorf("ảnh nhân vật lỗi: %w", err)
		}
		modelPhotos = append(modelPhotos, photos...)
	}
	if len(modelPhotos) > 0 {
		library, err := s.Studio.ModelLibrary()
		if err != nil {
			return p, fmt.Errorf("không đọc được thư viện nhân vật: %w", err)
		}
		var saved []string
		for _, path := range modelPhotos {
			photo, err := library.Add(accountID, path)
			if err != nil {
				return p, fmt.Errorf("không lưu được ảnh nhân vật vào kênh: %w", err)
			}
			saved = append(saved, photo.Path)
		}
		modelPhotos = saved
	}
	if len(modelPhotos) == 0 {
		var ids []int64
		for _, raw := range r.MultipartForm.Value["model_photo_ids"] {
			if id, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64); err == nil && id > 0 {
				ids = append(ids, id)
			}
		}
		if len(ids) > 0 {
			modelPhotos = s.findModelPhotosByIDsForAccount(accountID, ids)
			if len(modelPhotos) != len(ids) {
				return p, fmt.Errorf("ảnh đã chọn không thuộc thư viện của kênh đích")
			}
		}
	}
	if len(modelPhotos) == 0 {
		if id, err := strconv.ParseInt(strings.TrimSpace(r.FormValue("model_id")), 10, 64); err == nil && id > 0 {
			modelPhotos = s.findModelPhotosByIDsForAccount(accountID, []int64{id})
		}
	}
	if len(modelPhotos) == 0 || !fileExists(modelPhotos[0]) {
		return p, fmt.Errorf("chọn hoặc tải ảnh nhân vật từ thư viện của kênh đã chọn")
	}

	productName := strings.TrimSpace(r.FormValue("product_name"))
	niche := strings.TrimSpace(r.FormValue("niche"))
	price := strings.TrimSpace(r.FormValue("product_price"))
	affLink := strings.TrimSpace(r.FormValue("aff_link"))
	var remotePhotoURLs []string
	if raw := strings.TrimSpace(r.FormValue("product_id")); raw != "" && s.Products != nil {
		if id, err := strconv.ParseInt(raw, 10, 64); err == nil && id > 0 {
			if product, err := s.Products.GetByID(id); err == nil {
				if productName == "" {
					productName = product.Title
				}
				if niche == "" {
					niche = product.Category
				}
				if price == "" && product.Price > 0 {
					price = fmt.Sprintf("%.0f ₫", product.Price)
				}
				if affLink == "" {
					affLink = product.ProductURL
				}
				remotePhotoURLs = append(remotePhotoURLs, product.ImageURLs...)
			}
		}
	}
	for _, raw := range strings.Split(strings.TrimSpace(r.FormValue("photo_urls")), "\n") {
		if raw = strings.TrimSpace(raw); strings.HasPrefix(raw, "http://") || strings.HasPrefix(raw, "https://") {
			remotePhotoURLs = append(remotePhotoURLs, raw)
		}
	}
	productPhoto, err := saveUpload(r, "product_photo", upDir)
	if err != nil {
		return p, fmt.Errorf("ảnh sản phẩm lỗi: %w", err)
	}
	extraPhotos, err := saveUploads(r, "extra_photos", upDir, 6)
	if err != nil {
		return p, fmt.Errorf("ảnh góc khác lỗi: %w", err)
	}
	for i, raw := range remotePhotoURLs {
		if productPhoto != "" && len(extraPhotos) >= 5 {
			break
		}
		dst := filepath.Join(upDir, fmt.Sprintf("%d-plan-product-%d.jpg", time.Now().UnixNano(), i))
		ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
		err := studio.DownloadImage(ctx, raw, dst)
		cancel()
		if err == nil && fileExists(dst) {
			if productPhoto == "" {
				productPhoto = dst
			} else {
				extraPhotos = append(extraPhotos, dst)
			}
		}
	}
	if productPhoto == "" {
		return p, fmt.Errorf("thiếu ảnh listing sản phẩm thật")
	}
	seconds, _ := strconv.Atoi(strings.TrimSpace(r.FormValue("seconds")))
	if seconds <= 0 {
		seconds = 60
	}
	p = studio.AffiliateParams{
		Mode: studio.AffiliateModeShots, VideoEngine: "manual",
		AccountID: accountID, ProductName: productName, ProductPrice: price,
		Niche: niche, ProductCategory: strings.TrimSpace(r.FormValue("product_category")),
		ProductUsage: strings.TrimSpace(r.FormValue("product_usage")),
		ProductFocus: strings.TrimSpace(r.FormValue("product_focus")),
		AffLink:      affLink, ModelPhoto: modelPhotos[0], ModelPhotos: modelPhotos,
		ProductPhoto: productPhoto, ExtraPhotos: extraPhotos, Seconds: seconds,
	}
	if p.ProductName == "" {
		return p, fmt.Errorf("nhập tên sản phẩm theo listing")
	}
	return p, nil
}

func (s *Server) handleStudioAffiliateCreatePrepared(w http.ResponseWriter, r *http.Request, planID string) {
	prep, err := s.Studio.GetAffiliatePreparation(planID)
	if err != nil {
		seeOther(w, r, "/studio?err="+url.QueryEscape(err.Error()))
		return
	}
	p := prep.Params
	if raw := strings.TrimSpace(r.FormValue("account_id")); raw != "" {
		id, _ := strconv.ParseInt(raw, 10, 64)
		if id != p.AccountID {
			seeOther(w, r, "/studio?err="+url.QueryEscape("Kênh đã đổi sau khi lập shot plan; hãy lập lại plan."))
			return
		}
	}
	if name := strings.TrimSpace(r.FormValue("product_name")); name != "" && name != p.ProductName {
		seeOther(w, r, "/studio?err="+url.QueryEscape("Tên sản phẩm đã đổi sau khi lập shot plan; hãy lập lại plan."))
		return
	}
	if raw := strings.TrimSpace(r.FormValue("seconds")); raw != "" {
		seconds, _ := strconv.Atoi(raw)
		if seconds > 0 && seconds != p.Seconds {
			seeOther(w, r, "/studio?err="+url.QueryEscape("Thời lượng đã đổi sau khi lập shot plan; hãy lập lại plan."))
			return
		}
	}
	clips, err := saveMotionClips(r, "motion_clips", s.Studio.UploadDir(), 30)
	if err != nil {
		seeOther(w, r, "/studio?err="+url.QueryEscape("Clip chuyển động lỗi: "+err.Error()))
		return
	}
	if len(clips) != len(prep.Shots) {
		seeOther(w, r, "/studio?err="+url.QueryEscape(fmt.Sprintf("Shot plan cần đúng %d clip theo đúng thứ tự; đã nhận %d.", len(prep.Shots), len(clips))))
		return
	}
	p.Clips = clips
	p.PreparedPlanID = prep.ID
	p.BPM, _ = strconv.Atoi(r.FormValue("bpm"))
	p.MusicStart, _ = strconv.ParseFloat(r.FormValue("music_start"), 64)
	p.MusicTitle = strings.TrimSpace(r.FormValue("music_title"))
	p.MusicArtist = strings.TrimSpace(r.FormValue("music_artist"))
	p.FixedCaption = strings.TrimSpace(r.FormValue("fixed_caption"))
	p.MusicPath, err = saveUpload(r, "music_file", s.Studio.UploadDir())
	if err != nil {
		seeOther(w, r, "/studio?err="+url.QueryEscape("File nhạc lỗi: "+err.Error()))
		return
	}
	if p.MusicPath == "" {
		if u := strings.TrimSpace(r.FormValue("music_url")); u != "" {
			dst := filepath.Join(s.Studio.UploadDir(), fmt.Sprintf("%d-affiliate-music.m4a", time.Now().UnixNano()))
			ctx, cancel := context.WithTimeout(r.Context(), 5*time.Minute)
			err = studio.DownloadAudio(ctx, u, dst)
			cancel()
			if err != nil {
				seeOther(w, r, "/studio?err="+url.QueryEscape("Không tải được nhạc: "+err.Error()))
				return
			}
			p.MusicPath = dst
		}
	}
	if p.MusicPath == "" && strings.TrimSpace(r.FormValue("music_category")) != "" {
		p.MusicPath = studio.FindTrendingMusic(strings.TrimSpace(r.FormValue("music_category")))
		if p.MusicPath == "" {
			seeOther(w, r, "/studio?err="+url.QueryEscape("Không tìm thấy nhạc cục bộ cho lựa chọn này."))
			return
		}
	}
	if _, err := s.Studio.CreateAffiliateJob(p); err != nil {
		seeOther(w, r, "/studio?err="+url.QueryEscape("Không tạo được job: "+err.Error()))
		return
	}
	seeOther(w, r, "/studio/jobs?tab=running&msg=created")
}

// handleStudioAffiliateCreate queues an affiliate video job.
func (s *Server) handleStudioAffiliateCreate(w http.ResponseWriter, r *http.Request) {
	studioErr := func(msg string) {
		seeOther(w, r, "/studio?err="+url.QueryEscape(msg))
	}
	if s.Studio == nil {
		studioErr("Studio chưa sẵn sàng — thử tải lại trang.")
		return
	}
	if err := r.ParseMultipartForm(60 << 20); err != nil {
		studioErr("Không đọc được form — thử lại.")
		return
	}
	planID := strings.TrimSpace(r.FormValue("prepared_plan_id"))
	if planID == "" {
		studioErr("Lập shot plan trước khi tải clip chuyển động.")
		return
	}
	s.handleStudioAffiliateCreatePrepared(w, r, planID)
}

// (R2-W3), JSON for polling when Accept: application/json.
func (s *Server) handleStudioJobs(w http.ResponseWriter, r *http.Request) {
	if strings.Contains(r.Header.Get("Accept"), "application/json") {
		var jobs []studioJobView
		if ids := r.URL.Query()["id"]; len(ids) > 0 {
			jobs = make([]studioJobView, 0, len(ids))
			seen := make(map[string]struct{}, len(ids))
			for _, id := range ids {
				if _, ok := seen[id]; ok {
					continue
				}
				seen[id] = struct{}{}
				if s.Studio == nil {
					continue
				}
				j, ok := s.Studio.GetJob(id)
				if ok {
					jobs = append(jobs, s.studioJobViews([]studio.Job{j})[0])
				}
			}
		} else if s.Studio != nil {
			jobs = s.studioJobViews(activeStudioJobs(s.Studio.ListJobs(50)))
		}
		s.writeJSON(w, map[string]any{"jobs": jobs})
		return
	}

	var allJobs []studioJobView
	if s.Studio != nil {
		allJobs = s.studioJobViews(s.Studio.ListJobs(50))
	}

	var runningJobs, doneJobs, failedJobs []studioJobView
	for _, j := range allJobs {
		switch j.Status {
		case "running", "queued":
			runningJobs = append(runningJobs, j)
		case "done":
			doneJobs = append(doneJobs, j)
		case "failed":
			failedJobs = append(failedJobs, j)
		}
	}

	tab := r.URL.Query().Get("tab")
	if tab == "" {
		if len(runningJobs) > 0 {
			tab = "running"
		} else {
			tab = "done"
		}
	}

	var displayedJobs []studioJobView
	switch tab {
	case "running":
		displayedJobs = runningJobs
	case "done":
		displayedJobs = doneJobs
	case "failed":
		displayedJobs = failedJobs
	case "schedule":
		displayedJobs = runningJobs
	default:
		displayedJobs = allJobs
	}

	schedEnabled, schedHours, schedLastRun, schedMusic, schedAutoPub, schedYouTube := s.scheduleView()
	schedMeta := false
	if s.Ledger != nil {
		schedMeta = automation.AutopilotMetaEnabled(s.settings())
	}

	nextRunStr := "Đang chờ lịch kích hoạt"
	if schedEnabled {
		if schedLastRun != "" {
			if t, err := time.Parse(time.RFC3339, schedLastRun); err == nil {
				next := t.Add(time.Duration(schedHours) * time.Hour)
				if next.After(time.Now()) {
					nextRunStr = next.Format("15:04 ngày 02/01/2006")
				} else {
					nextRunStr = "Sắp chạy ngay trong vài phút tới"
				}
			}
		} else {
			nextRunStr = "Sắp kích hoạt trong chu kỳ tiếp theo"
		}
	} else {
		nextRunStr = "Tạm dừng (Lịch tự động đang tắt)"
	}

	s.render(w, "studio_jobs", s.ctx(
		"Title", "Quản lý Jobs & Lịch Studio",
		"Path", "/studio/jobs",
		"Jobs", displayedJobs,
		"AllJobs", allJobs,
		"RunningJobs", runningJobs,
		"DoneJobs", doneJobs,
		"FailedJobs", failedJobs,
		"RunningCount", len(runningJobs),
		"DoneCount", len(doneJobs),
		"TotalCount", len(allJobs),
		"Tab", tab,
		"CurrentTab", tab,
		"SchedEnabled", schedEnabled,
		"SchedHours", schedHours,
		"SchedLastRun", schedLastRun,
		"SchedNextRun", nextRunStr,
		"SchedMusic", schedMusic,
		"SchedAutoPub", schedAutoPub,
		"SchedYouTube", schedYouTube,
		"SchedMeta", schedMeta,
		"Msg", r.URL.Query().Get("msg"),
		"Err", r.URL.Query().Get("err"),
	))
}

// activeStudioJobs keeps status-only polling focused on work still in progress.
func activeStudioJobs(jobs []studio.Job) []studio.Job {
	active := make([]studio.Job, 0, len(jobs))
	for _, job := range jobs {
		if job.Status == studio.StatusQueued || job.Status == studio.StatusRunning {
			active = append(active, job)
		}
	}
	return active
}

// handleStudioScheduleSave saves the autopilot schedule settings directly from Studio.
func (s *Server) handleStudioScheduleSave(w http.ResponseWriter, r *http.Request) {
	if s.Ledger == nil {
		seeOther(w, r, "/studio/jobs?tab=schedule&err="+url.QueryEscape("Kho cài đặt chưa khởi tạo"))
		return
	}
	if err := r.ParseForm(); err != nil {
		seeOther(w, r, "/studio/jobs?tab=schedule&err="+url.QueryEscape("Không đọc được dữ liệu form: "+err.Error()))
		return
	}
	enabled := "0"
	if r.PostFormValue("enabled") == "on" {
		enabled = "1"
	}
	autoPub := "0"
	if r.PostFormValue("auto_publish") == "on" {
		autoPub = "1"
	}
	youTube := "0"
	if r.PostFormValue("youtube_enabled") == "on" {
		youTube = "1"
	}
	meta := "0"
	if r.PostFormValue("meta_enabled") == "on" {
		meta = "1"
	}
	hours := 6
	if h, err := strconv.Atoi(strings.TrimSpace(r.PostFormValue("interval_hours"))); err == nil && h >= 1 && h <= 168 {
		hours = h
	}

	st := s.settings()
	_ = st.Set(SettingAutopilotEnabled, enabled)
	_ = st.Set(SettingAutopilotInterval, strconv.Itoa(hours))
	_ = st.Set(SettingAutopilotAutoPublish, autoPub)
	_ = st.Set(SettingAutopilotYouTube, youTube)
	_ = st.Set(SettingAutopilotMeta, meta)

	msg := "Đã cập nhật lịch tự động: TẮT"
	if enabled == "1" {
		msg = fmt.Sprintf("Đã bật lịch tự động tạo video mỗi %d giờ.", hours)
	}
	seeOther(w, r, "/studio/jobs?tab=schedule&msg="+url.QueryEscape(msg))
}

// handleStudioScheduleRunNow triggers one immediate autopilot production cycle.
func (s *Server) handleStudioScheduleRunNow(w http.ResponseWriter, r *http.Request) {
	if s.Autopilot == nil {
		seeOther(w, r, "/studio/jobs?tab=schedule&err="+url.QueryEscape("Hệ thống Autopilot chưa được cấu hình runner."))
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
		defer cancel()
		_ = s.Autopilot.RunAll(ctx)
	}()
	seeOther(w, r, "/studio/jobs?tab=running&msg="+url.QueryEscape("Đã phát lệnh kích hoạt dây chuyền AI sản xuất video ngay lập tức!"))
}

// handleStudioJobCancel cancels a running/queued job.
func (s *Server) handleStudioJobCancel(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if s.Studio != nil {
		s.Studio.CancelRunning(id)
	}
	seeOther(w, r, "/studio/jobs")
}

// handleStudioJobDelete removes a job and all its files.
func (s *Server) handleStudioJobDelete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	returnTo := "/studio/jobs"
	if r.FormValue("return_to") == "/" {
		returnTo = "/"
	}
	if s.Studio == nil {
		seeOther(w, r, returnTo)
		return
	}
	if err := s.Studio.DeleteJob(id); err != nil {
		seeOther(w, r, returnTo+"?err="+url.QueryEscape("Không xoá được job: "+err.Error()))
		return
	}
	if returnTo == "/" {
		seeOther(w, r, "/?ok="+url.QueryEscape("Đã xóa tác vụ thất bại."))
		return
	}
	seeOther(w, r, "/studio/jobs?msg="+url.QueryEscape("Đã xóa job."))
}

// handleStudioJobRetry creates a new affiliate job from an old job's params.
func (s *Server) handleStudioJobRetry(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if s.Studio == nil {
		seeOther(w, r, "/studio/jobs")
		return
	}
	j, ok := s.Studio.GetJob(id)
	if !ok {
		seeOther(w, r, "/studio?err="+url.QueryEscape("Không tìm thấy job "+id))
		return
	}
	if j.Kind != studio.KindAffiliate {
		seeOther(w, r, "/studio?err="+url.QueryEscape("Chỉ thử lại được job affiliate"))
		return
	}
	var p studio.AffiliateParams
	if err := json.Unmarshal([]byte(j.Params), &p); err != nil {
		seeOther(w, r, "/studio?err="+url.QueryEscape("Không đọc được tham số job cũ"))
		return
	}
	planID, err := s.Studio.ReissueAffiliatePreparation(p)
	if err != nil {
		seeOther(w, r, "/studio?err="+url.QueryEscape("Không thể khôi phục shot plan: "+err.Error()))
		return
	}
	p.PreparedPlanID = planID
	if _, err := s.Studio.CreateAffiliateJob(p); err != nil {
		seeOther(w, r, "/studio?err="+url.QueryEscape("Không tạo được job mới: "+err.Error()))
		return
	}
	seeOther(w, r, "/studio/jobs")
}

// studioAssetView projects a storyboard asset for JSON.
// Label/Class are server-rendered so clients never redefine them (R2-W3).
type studioAssetView struct {
	ID       int64  `json:"id"`
	Idx      int    `json:"idx"`
	Kind     string `json:"kind"`
	Status   string `json:"status"`
	Prompt   string `json:"prompt"`
	Preview  string `json:"preview"`
	ImageURL string `json:"image_url,omitempty"`
	VideoURL string `json:"video_url,omitempty"`
	Label    string `json:"label"`
	Class    string `json:"class"`
}

// handleStudioJobDetail returns one job + its storyboard assets as JSON.
func (s *Server) handleStudioJobDetail(w http.ResponseWriter, r *http.Request) {
	if s.Studio == nil {
		http.NotFound(w, r)
		return
	}
	id := r.PathValue("id")
	j, ok := s.Studio.GetJob(id)
	if !ok {
		http.NotFound(w, r)
		return
	}
	assets := s.Studio.ListAssets(id)
	views := make([]studioAssetView, 0, len(assets))
	jobDir := filepath.Join(s.Studio.WorkDir(), id)

	for _, a := range assets {
		v := studioAssetView{
			ID:     a.ID,
			Idx:    a.Idx,
			Kind:   a.Kind,
			Status: a.Status,
			Prompt: a.Prompt,
			Label:  statusLabel(a.Status),
			Class:  statusClass(a.Status),
		}

		if a.Status == "done" {
			// 1. Check for Image Asset
			imgExts := []string{".png", ".jpg", ".jpeg", ".webp"}
			isImg := false
			for _, ext := range imgExts {
				if strings.HasSuffix(strings.ToLower(a.Path), ext) {
					isImg = true
					break
				}
			}

			if isImg && a.Path != "" {
				v.ImageURL = "/studio/assets/" + id + "/" + filepath.Base(a.Path)
			} else {
				// Search disk in jobDir for keyframe image
				imgCandidates := []string{
					fmt.Sprintf("scene_%02d.png", a.Idx),
					fmt.Sprintf("scene_%02d.jpg", a.Idx),
					fmt.Sprintf("scene-%02d.png", a.Idx),
					fmt.Sprintf("scene-%02d.jpg", a.Idx),
					fmt.Sprintf("photo-%02d.png", a.Idx),
					fmt.Sprintf("photo-%02d.png", a.Idx-1),
					fmt.Sprintf("photo-%02d.jpg", a.Idx),
					fmt.Sprintf("photo-%02d.jpg", a.Idx-1),
					fmt.Sprintf("own%d_0.png", a.Idx),
					fmt.Sprintf("own%d_0.jpg", a.Idx),
				}
				for _, c := range imgCandidates {
					if fileExists(filepath.Join(jobDir, c)) {
						v.ImageURL = "/studio/assets/" + id + "/" + c
						break
					}
				}
			}

			// 2. Check for Video Asset
			vidExts := []string{".mp4", ".mov", ".webm"}
			isVid := false
			for _, ext := range vidExts {
				if strings.HasSuffix(strings.ToLower(a.Path), ext) {
					isVid = true
					break
				}
			}

			if isVid && a.Path != "" {
				v.VideoURL = "/studio/assets/" + id + "/" + filepath.Base(a.Path)
			} else {
				// Search disk in jobDir for motion clip
				vidCandidates := []string{
					fmt.Sprintf("motion_%02d.mp4", a.Idx),
					fmt.Sprintf("motion_%02d.mp4", a.Idx+1),
					fmt.Sprintf("clip_%02d.mp4", a.Idx),
					fmt.Sprintf("clip_%02d.mp4", a.Idx+1),
				}
				for _, c := range vidCandidates {
					if fileExists(filepath.Join(jobDir, c)) {
						v.VideoURL = "/studio/assets/" + id + "/" + c
						break
					}
				}
			}

			// 3. Set Preview
			if v.ImageURL != "" {
				v.Preview = v.ImageURL
			} else if v.VideoURL != "" {
				v.Preview = v.VideoURL
			} else if a.Path != "" {
				v.Preview = "/studio/assets/" + id + "/" + filepath.Base(a.Path)
			}
		}

		views = append(views, v)
	}

	resp := map[string]any{
		"job":    j,
		"assets": views,
	}
	// Checklist lên xu hướng cho video affiliate đã xong (đo từ file thật).
	if j.Kind == studio.KindAffiliate && j.Status == studio.StatusDone && j.Output != "" {
		var p studio.AffiliateParams
		_ = json.Unmarshal([]byte(j.Params), &p)
		outPath := j.Output
		if s.Studio != nil {
			outPath = s.Studio.OutputPath(j.Output)
		}
		resp["trend"] = studio.TrendReadiness(r.Context(), outPath, p, j.Caption)
	}
	s.writeJSON(w, resp)
}

var studioAssetRe = regexp.MustCompile(`^[A-Za-z0-9_.-]+\.(png|jpg|jpeg|webp|mp4|mov|webm)$`)

func (s *Server) handleStudioAsset(w http.ResponseWriter, r *http.Request) {
	if s.Studio == nil {
		http.NotFound(w, r)
		return
	}
	job := r.PathValue("job")
	file := r.PathValue("file")
	if !studioAssetRe.MatchString(file) || strings.Contains(job, "..") || strings.Contains(job, "/") {
		http.NotFound(w, r)
		return
	}
	// The job id is a unix-nano timestamp; be lenient but safe.
	for _, c := range job {
		if c < '0' || c > '9' {
			http.NotFound(w, r)
			return
		}
	}
	path := filepath.Join(s.Studio.WorkDir(), job, file)
	if !fileExists(path) {
		// Fallback 1: Upload directory
		uploadCandidate := filepath.Join(s.Studio.UploadDir(), file)
		if fileExists(uploadCandidate) {
			path = uploadCandidate
		} else {
			// Fallback 2: OutDir root
			outCandidate := filepath.Join(s.OutDir, file)
			if fileExists(outCandidate) {
				path = outCandidate
			}
		}
	}
	if !strings.HasPrefix(filepath.Clean(path), filepath.Clean(s.Studio.WorkDir())) &&
		!strings.HasPrefix(filepath.Clean(path), filepath.Clean(s.OutDir)) {
		http.NotFound(w, r)
		return
	}
	serveMediaFile(w, r, path)
}

// handleStudioTrends serves the VN trending chart: HTML page (R2-W3),
// JSON when Accept: application/json.
func (s *Server) handleStudioTrends(w http.ResponseWriter, r *http.Request) {
	sounds, err := studio.FetchVNTrending(r.Context())
	if strings.Contains(r.Header.Get("Accept"), "application/json") {
		s.writeJSON(w, map[string]any{
			"source": studio.TrendsSource,
			"sounds": sounds,
			"error":  errText(err),
		})
		return
	}
	s.render(w, "studio_trends", s.ctx(
		"Title", "Nhạc thịnh hành", "Path", "/studio/trends",
		"Sounds", sounds, "TrendsSource", studio.TrendsSource, "TrendsErr", errText(err),
	))
}

// handleStudioTrendsRefresh forces a chart refresh.
func (s *Server) handleStudioTrendsRefresh(w http.ResponseWriter, r *http.Request) {
	_, _ = studio.RefreshVNTrending(r.Context())
	http.Redirect(w, r, "/studio/trends", http.StatusSeeOther)
}

// handleStudioMediaGenHealth reports media-gen provider health as JSON.
func (s *Server) handleStudioMediaGenHealth(w http.ResponseWriter, r *http.Request) {
	healthy, keys, name := false, 0, ""
	if s.Studio != nil {
		if mg := s.Studio.MediaGen(); mg != nil {
			name = mg.Name()
			healthy = mg.Healthy(r.Context())
			if g, ok := mg.(interface{ KeyCount() int }); ok {
				keys = g.KeyCount()
			}
		}
	}
	s.writeJSON(w, map[string]any{
		"provider": name, "healthy": healthy, "keys": keys,
	})
}

var studioUploadFileRe = regexp.MustCompile(`^[A-Za-z0-9_.-]+\.(png|jpg|jpeg|webp|mp4|mov|m4a|mp3)$`)

// handleStudioUpload serves uploaded and AI-generated assets from Studio's upload directory.
func (s *Server) handleStudioUpload(w http.ResponseWriter, r *http.Request) {
	if s.Studio == nil {
		http.NotFound(w, r)
		return
	}
	file := r.PathValue("file")
	if !studioUploadFileRe.MatchString(file) || strings.Contains(file, "..") {
		http.NotFound(w, r)
		return
	}
	path := filepath.Join(s.Studio.UploadDir(), file)
	if !strings.HasPrefix(filepath.Clean(path), filepath.Clean(s.Studio.UploadDir())) {
		http.NotFound(w, r)
		return
	}
	serveMediaFile(w, r, path)
}

// handleStudioGenerateImage generates a high-quality product image via Gemini Imagen or visual fallback.
func (s *Server) handleStudioGenerateImage(w http.ResponseWriter, r *http.Request) {
	if s.Studio == nil {
		writeJSONErr(w, "Studio chưa sẵn sàng", http.StatusServiceUnavailable)
		return
	}
	productName := strings.TrimSpace(r.FormValue("product_name"))
	niche := strings.TrimSpace(r.FormValue("niche"))
	prompt := strings.TrimSpace(r.FormValue("prompt"))
	aspect := strings.TrimSpace(r.FormValue("aspect_ratio"))
	if productName == "" {
		productName = "Sản phẩm AI Creator"
	}
	if niche == "" {
		niche = "Thương mại điện tử"
	}

	wPx, hPx := 1080, 1920
	if aspect == "16:9" {
		wPx, hPx = 1920, 1080
	} else if aspect == "1:1" {
		wPx, hPx = 1080, 1080
	}

	filename := fmt.Sprintf("%d-ai-prod.png", time.Now().UnixNano())
	outPath := filepath.Join(s.Studio.UploadDir(), filename)
	source := "studio_visual"

	// Try Gemini Imagen if keys are healthy
	if s.Studio.MediaGen() != nil && s.Studio.MediaGen().Healthy(r.Context()) {
		geminiPrompt := prompt
		if geminiPrompt == "" {
			geminiPrompt = fmt.Sprintf("Ultra-detailed professional commercial advertising photograph of %s, %s, 4K quality, tack-sharp focus, photorealistic, cinematic studio lighting, commercial photography, no text, no watermark", productName, niche)
		}
		if gerr := s.Studio.MediaGen().GenerateImage(r.Context(), geminiPrompt, nil, outPath); gerr == nil {
			source = "gemini"
		}
	}

	// If Gemini was not used or failed, generate high-quality visual card
	if source != "gemini" {
		sub := prompt
		if sub == "" {
			sub = "CHÍNH HÃNG 100% · ƯU ĐÃI ĐẶC BIỆT"
		}
		if err := studio.GenerateProductVisual(productName, niche, sub, wPx, hPx, outPath); err != nil {
			writeJSONErr(w, "Không tạo được ảnh: "+err.Error(), http.StatusInternalServerError)
			return
		}
	}

	s.writeJSON(w, map[string]any{
		"ok":       true,
		"filename": filename,
		"path":     outPath,
		"url":      "/studio/uploads/" + filename,
		"source":   source,
	})
}

var mediaNameRe = regexp.MustCompile(`^[A-Za-z0-9_-]+\.(mp4|srt|vtt|mp3|wav|jpg|jpeg|png|webp|md|txt)$`)

func (s *Server) handleMedia(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	// Strict allowlist: a single path segment, valid media extensions only. Anything else
	// (including "..") is a 404.
	if !mediaNameRe.MatchString(name) {
		http.NotFound(w, r)
		return
	}
	serveMediaFile(w, r, filepath.Join(s.OutDir, name))
}

// handleStudioModelPhoto serves uploaded user model photos from data/output/studio/models/{account}/{file}.
func (s *Server) handleStudioModelPhoto(w http.ResponseWriter, r *http.Request) {
	account := r.PathValue("account")
	file := r.PathValue("file")
	if !mediaNameRe.MatchString(file) {
		http.NotFound(w, r)
		return
	}
	serveMediaFile(w, r, filepath.Join(s.OutDir, "studio", "models", account, file))
}

// --------------------------------------------------------------- publishers

// capcutExportRoot là nơi xuất gói chỉnh sửa (~/Movies/AI Creator OS).
// Biến để test trỏ về thư mục tạm.
var capcutExportRoot = func() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "Movies", "AI Creator OS")
}

// openOnMac chạy lệnh `open` của macOS (seam cho test).
var openOnMac = func(args ...string) error { return exec.Command("open", args...).Run() }

// handleStudioJobCapCut xuất gói chỉnh sửa, mở Finder tới thư mục và bật
// CapCut nếu máy có (CapCut không có API — người dùng kéo thả file vào).
func (s *Server) handleStudioJobCapCut(w http.ResponseWriter, r *http.Request) {
	if s.Studio == nil {
		http.NotFound(w, r)
		return
	}
	dir, err := s.Studio.ExportForEditing(r.PathValue("id"), capcutExportRoot())
	if err != nil {
		seeOther(w, r, "/studio/jobs?err="+url.QueryEscape("Xuất cho CapCut lỗi: "+err.Error()))
		return
	}
	_ = openOnMac("-R", dir)
	msg := "Đã xuất vào " + dir + " — kéo các file vào timeline CapCut."
	if openOnMac("-a", "CapCut") != nil {
		msg += " (Chưa thấy app CapCut — cài từ capcut.com rồi mở thư mục này.)"
	}
	seeOther(w, r, "/studio/jobs?msg="+url.QueryEscape(msg))
}

// mediaName trả tên file để phát qua /media/{name} (route chỉ nhận 1
// đoạn tên). j.Output là đường dẫn đầy đủ — dùng thẳng sẽ ra URL hỏng.
func mediaName(output string) string {
	if output == "" {
		return ""
	}
	return filepath.Base(output)
}

// handleStudioJobRemix dựng lại 1 video affiliate với tham số đã sửa
// (thời lượng, nhịp, đoạn nhạc, file nhạc, thứ tự ảnh, chú thích). Bản cũ
// giữ nguyên — đây là "chỉnh sửa" trong app vì CapCut không có API.
func (s *Server) handleStudioJobRemix(w http.ResponseWriter, r *http.Request) {
	back := func(q string) { seeOther(w, r, "/studio/jobs?"+q) }
	if s.Studio == nil {
		back("err=" + url.QueryEscape("Studio chưa sẵn sàng."))
		return
	}
	j, ok := s.Studio.GetJob(r.PathValue("id"))
	if !ok || j.Kind != studio.KindAffiliate {
		back("err=" + url.QueryEscape("Chỉ chỉnh sửa được video affiliate đã có."))
		return
	}
	var p studio.AffiliateParams
	if err := json.Unmarshal([]byte(j.Params), &p); err != nil {
		back("err=" + url.QueryEscape("Không đọc được tham số video cũ."))
		return
	}
	if err := r.ParseMultipartForm(60 << 20); err != nil {
		back("err=" + url.QueryEscape("Không đọc được form — thử lại."))
		return
	}
	if v, err := strconv.Atoi(strings.TrimSpace(r.FormValue("seconds"))); err == nil {
		p.Seconds = clampInt(v, 10, 120)
	}
	if v, err := strconv.Atoi(strings.TrimSpace(r.FormValue("bpm"))); err == nil {
		p.BPM = clampInt(v, 60, 200)
	}
	if v, err := strconv.ParseFloat(strings.TrimSpace(r.FormValue("music_start")), 64); err == nil && v >= 0 {
		p.MusicStart = v
	}
	music, err := saveUpload(r, "music_file", s.Studio.UploadDir())
	if err != nil {
		back("err=" + url.QueryEscape("File nhạc lỗi: "+err.Error()))
		return
	}
	if music != "" {
		p.MusicPath = music
	}
	p = reorderPhotos(p, r.FormValue("order"))
	p.FixedCaption = strings.TrimSpace(r.FormValue("caption"))
	planID, err := s.Studio.ReissueAffiliatePreparation(p)
	if err != nil {
		back("err=" + url.QueryEscape("Không thể khôi phục shot plan: "+err.Error()))
		return
	}
	p.PreparedPlanID = planID
	if _, err := s.Studio.CreateAffiliateJob(p); err != nil {
		back("err=" + url.QueryEscape("Không dựng lại được: "+err.Error()))
		return
	}
	back("msg=" + url.QueryEscape("Đang dựng bản chỉnh sửa — bản cũ vẫn giữ để so sánh."))
}

// reorderPhotos đổi thứ tự ảnh sản phẩm + ảnh thêm (ảnh người mẫu giữ
// nguyên vì là ảnh khóa khuôn mặt). order: "reverse" | "rotate" | "".
func reorderPhotos(p studio.AffiliateParams, order string) studio.AffiliateParams {
	var all []string
	if p.ProductPhoto != "" {
		all = append(all, p.ProductPhoto)
	}
	all = append(all, p.ExtraPhotos...)
	if len(all) < 2 {
		return p
	}
	switch order {
	case "reverse":
		for i, k := 0, len(all)-1; i < k; i, k = i+1, k-1 {
			all[i], all[k] = all[k], all[i]
		}
	case "rotate":
		all = append(all[1:], all[0])
	default:
		return p
	}
	p.ProductPhoto, p.ExtraPhotos = all[0], append([]string(nil), all[1:]...)
	return p
}

// handleStudioStoryCreate nhận yêu cầu tạo video dài / kể chuyện YouTube & cắt Shorts từ Studio.
func (s *Server) handleStudioStoryCreate(w http.ResponseWriter, r *http.Request) {
	if s.Studio == nil {
		seeOther(w, r, "/studio?err="+url.QueryEscape("Studio chưa sẵn sàng — thử tải lại trang."))
		return
	}
	_ = r.ParseMultipartForm(10 << 20)
	_ = r.ParseForm()
	genre := strings.TrimSpace(r.PostFormValue("genre"))
	if genre == "" {
		genre = "tâm lý"
	}
	topic := strings.TrimSpace(r.PostFormValue("topic"))
	isAutoGenerated := false
	if topic == "" {
		topic = studio.DefaultAudioStoryTopic(genre)
		isAutoGenerated = true
	}

	durationMin, _ := strconv.Atoi(r.PostFormValue("duration_minutes"))
	if durationMin <= 0 {
		durationMin = 30
	}
	words, _ := strconv.Atoi(r.PostFormValue("words"))
	scenes, _ := strconv.Atoi(r.PostFormValue("scenes"))
	voice := strings.TrimSpace(r.PostFormValue("voice"))
	autoClip := r.PostFormValue("auto_clip_shorts") == "1" || r.PostFormValue("auto_clip_shorts") == "true"
	clipSec, _ := strconv.Atoi(r.PostFormValue("clip_duration_sec"))
	if clipSec <= 0 {
		clipSec = 180
	}

	musicOn := r.PostFormValue("music_on") == "1"
	musicPath := ""
	if musicOn {
		if mp := filepath.Join(s.DataDir, "autopilot-music.m4a"); fileExists(mp) {
			musicPath = mp
		} else {
			musicOn = false
		}
	}

	p := studio.StoryParams{
		Topic:           topic,
		Genre:           genre,
		DurationMinutes: durationMin,
		Words:           words,
		Scenes:          scenes,
		MusicOn:         musicOn,
		MusicPath:       musicPath,
		Voice:           voice,
		AutoClipShorts:  autoClip,
		ClipDurationSec: clipSec,
	}

	id, err := s.Studio.CreateStoryJob(p)
	if err != nil {
		seeOther(w, r, "/studio?mode=story&err="+url.QueryEscape("Lỗi tạo video truyện: "+err.Error()))
		return
	}

	if s.Ledger != nil {
		_ = s.Ledger.Decide("ai_orchestrator", "story_produce", nil, fmt.Sprintf("Studio điều phối video truyện dài job=%s duration=%dp genre=%s auto_clip=%v auto_plot=%v: %s", id, durationMin, genre, autoClip, isAutoGenerated, topic), nil)
	}

	clipNote := ""
	if autoClip {
		clipNote = " và tự động cắt clip 3–5 phút cho TikTok/Reels"
	}
	noticePrefix := fmt.Sprintf("Đã đưa video truyện dài %d phút", durationMin)
	if isAutoGenerated {
		noticePrefix = fmt.Sprintf("AI đã tự động nghĩ cốt truyện và đưa video truyện dài %d phút", durationMin)
	}
	seeOther(w, r, "/studio/jobs?tab=running&notice="+url.QueryEscape(fmt.Sprintf("%s '%s' vào hàng đợi AI%s (Job: %s)!", noticePrefix, topic, clipNote, id)))
}

// handleAPIStoryPlots trả về danh sách cốt truyện audio YouTube (AI tự nghĩ sẵn, độc bản).
func (s *Server) handleAPIStoryPlots(w http.ResponseWriter, r *http.Request) {
	genre := r.URL.Query().Get("genre")
	topic := strings.TrimSpace(r.URL.Query().Get("topic"))
	dur, _ := strconv.Atoi(r.URL.Query().Get("duration"))
	if dur <= 0 {
		dur = 30
	}

	plots := studio.GetCuratedAudioStories(genre)
	if topic != "" || r.URL.Query().Get("refresh") == "1" {
		dynamicPlot := studio.GenerateUniqueAudioStory(topic, genre, dur)
		plots = append([]studio.AudioStoryPlot{dynamicPlot}, plots...)
	}

	s.writeJSON(w, map[string]any{
		"ok":    true,
		"plots": plots,
	})
}

// handleAPITikTokTrends trả về dữ liệu JSON của Top 10 Video Đu Trend TikTok Việt & Drama Showbiz (Việt/Trung).
func (s *Server) handleAPITikTokTrends(w http.ResponseWriter, r *http.Request) {
	if r.URL.Query().Get("refresh") == "1" {
		studio.InvalidateTikTokRadarCache()
	}
	ttTrends, err := studio.FetchTikTokTrendingVideos(r.Context())
	dramasVN, dramasCN, _ := studio.FetchSplitShowbizDrama(r.Context())
	combinedDramas := append([]studio.ShowbizDrama(nil), dramasVN...)
	combinedDramas = append(combinedDramas, dramasCN...)

	s.writeJSON(w, map[string]any{
		"ok":             err == nil,
		"tiktok_trends":  ttTrends,
		"dramas_vn":      dramasVN,
		"dramas_cn":      dramasCN,
		"showbiz_dramas": combinedDramas,
	})
}

// handleAPITikTokAnalyze phân tích một URL video TikTok cụ thể do người dùng cung cấp.
func (s *Server) handleAPITikTokAnalyze(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		s.writeJSON(w, map[string]any{"ok": false, "error": "Dữ liệu không hợp lệ"})
		return
	}
	urlStr := r.FormValue("url")
	if urlStr == "" {
		s.writeJSON(w, map[string]any{"ok": false, "error": "Vui lòng nhập link video TikTok"})
		return
	}
	video, err := studio.AnalyzeTikTokVideoURL(r.Context(), urlStr)
	if err != nil {
		s.writeJSON(w, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	s.writeJSON(w, map[string]any{"ok": true, "video": video})
}

// handleAPIFaceSwapStatus trả về thông tin trạng thái hoạt động của Face Swap engine cục bộ.
func (s *Server) handleAPIFaceSwapStatus(w http.ResponseWriter, r *http.Request) {
	st := studio.CheckFaceSwapEngine(r.Context())
	s.writeJSON(w, map[string]any{
		"ok":     true,
		"status": st,
	})
}
