package web

import (
	"fmt"
	"github.com/ninhlee99/ai-creator-os/internal/automation"
	"github.com/ninhlee99/ai-creator-os/internal/network"
	"github.com/ninhlee99/ai-creator-os/internal/studio"
	"net/http"
	"strconv"
	"strings"
)

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
	flowReady, flowStatus := s.Studio.WebAIVideoReady(studio.AffiliateVideoEngineGoogleFlow)

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
	} else if notice == "" {
		notice = r.URL.Query().Get("msg")
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
		"Warning", r.URL.Query().Get("warn"),
		"Sounds", sounds,
		"TopSound", topSound(sounds),
		"TrendsSource", studio.TrendsSource,
		"TrendsErr", errText(trendsErr),
		"MediaGenOK", mgOK,
		"FlowReady", flowReady,
		"FlowStatus", flowStatus,
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
