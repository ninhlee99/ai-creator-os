package web

import (
	"encoding/json"
	"github.com/ninhlee99/ai-creator-os/internal/automation"
	"github.com/ninhlee99/ai-creator-os/internal/network"
	"github.com/ninhlee99/ai-creator-os/internal/studio"
	"github.com/ninhlee99/ai-creator-os/internal/viralradar"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
)

// viralJobView là dữ liệu hiển thị của một tác vụ dựng video viral 9:16.
type viralJobView struct {
	ID         string
	Title      string
	Topic      string
	Genre      string
	GenreVN    string
	Status     string
	StatusVN   string
	Progress   int
	MediaURL   string
	SRTURL     string
	LogTail    string
	CreatedAt  string
	FinishedAt string
}

// viralView là mô hình dữ liệu chính cho trang /viral.
type viralView struct {
	StudioOK              bool
	Jobs                  []viralJobView
	AccountNames          []string
	AccountOptions        []*network.Account
	SelectedAccountID     int64
	CharacterEditorReady  bool
	CharacterEditorStatus string
	AutoOn                bool
	ActiveCount           int
	DoneCount             int
	TotalCount            int
	Error                 string
	Notice                string
	AvailableModels       []modelPhotoCardView
	SelectedModelID       int64
	SelectedModelPhoto    string
	RadarSources          []viralradar.Source
	RadarQuery            string
	RadarLastRun          string
	RadarKeyReady         bool
}

// handleViral renders the video creation form and actual job history.
func viralGenreVN(genre string) string {
	switch genre {
	case "dance":
		return "Vũ đạo"
	case "lifehack":
		return "Mẹo đời sống"
	case "mystery":
		return "Bí ẩn"
	case "tech_fact":
		return "Công nghệ"
	case "trendjack":
		return "Chủ đề phổ thông"
	default:
		return genre
	}
}

func viralStatusVN(st string) string {
	switch st {
	case studio.StatusQueued:
		return "Đang chờ"
	case studio.StatusRunning:
		return "Đang dựng"
	case studio.StatusDone:
		return "Hoàn tất"
	case studio.StatusFailed:
		return "Cần xử lý"
	default:
		return st
	}
}

func (s *Server) handleViral(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	v := viralView{
		StudioOK:        s.Studio != nil,
		Error:           q.Get("err"),
		Notice:          q.Get("notice"),
		AvailableModels: s.listModelPhotoCards(),
		RadarQuery:      s.defaultViralRadarQuery(),
	}
	if s.Studio != nil {
		v.CharacterEditorReady, v.CharacterEditorStatus = s.Studio.ViralCharacterEditorStatus(r.Context())
	}
	if key, ok := s.effectiveEnv("YOUTUBE_API_KEY"); ok && strings.TrimSpace(key) != "" {
		v.RadarKeyReady = true
	}
	settings := s.settings()
	if raw, ok := settings.Get(automation.KeyViralRadarSources); ok {
		_ = json.Unmarshal([]byte(raw), &v.RadarSources)
		if len(v.RadarSources) > 10 {
			v.RadarSources = v.RadarSources[:10]
		}
	}
	if lastRun, ok := settings.Get("viral.radar.last_run"); ok {
		v.RadarLastRun = lastRun
	}
	if query, ok := settings.Get(automation.KeyViralRadarQuery); ok && strings.TrimSpace(query) != "" {
		v.RadarQuery = query
	}

	if msg := q.Get("msg"); msg == "created" {
		v.Notice = "Tác vụ sản xuất video viral AI đã được đưa vào hàng đợi thành công!"
	}

	// Channel selection scopes every identity photo shown and accepted below.
	if s.Mgr != nil {
		if accounts, err := s.Mgr.ListWithAutopilot(); err == nil {
			v.AccountOptions = accounts
			for _, a := range accounts {
				if a != nil && a.Username != "" {
					v.AccountNames = append(v.AccountNames, a.Username)
				}
			}
		}
	}
	if raw := q.Get("account_id"); raw != "" {
		v.SelectedAccountID, _ = strconv.ParseInt(raw, 10, 64)
	}
	if v.SelectedAccountID == 0 && strings.TrimSpace(q.Get("account")) != "" {
		for _, account := range v.AccountOptions {
			if account != nil && account.Username == strings.TrimSpace(q.Get("account")) {
				v.SelectedAccountID = account.ID
				break
			}
		}
	}
	// A deep link to a saved character photo also selects its owning channel.
	if modelIDStr := q.Get("model_id"); modelIDStr != "" {
		if id, err := strconv.ParseInt(modelIDStr, 10, 64); err == nil && id > 0 {
			for _, m := range v.AvailableModels {
				if m.ID == id {
					if v.SelectedAccountID == 0 {
						v.SelectedAccountID = m.AccountID
					}
					if m.AccountID == v.SelectedAccountID {
						v.SelectedModelID = id
						v.SelectedModelPhoto = m.URL
					}
					break
				}
			}
		}
	}
	if v.SelectedAccountID == 0 && len(v.AccountOptions) > 0 && v.AccountOptions[0] != nil {
		v.SelectedAccountID = v.AccountOptions[0].ID
	}
	if v.SelectedModelID == 0 {
		for _, m := range v.AvailableModels {
			if m.AccountID == v.SelectedAccountID {
				v.SelectedModelID = m.ID
				v.SelectedModelPhoto = m.URL
				break
			}
		}
	}

	// Video tham chiếu chỉ được chọn rõ ràng từ file đã nhập.

	// Đọc công tắc tự động hóa viral
	st := s.settings()
	v.AutoOn = true
	if val, ok := st.Get(automation.KeyViralAutoProduce); ok {
		v.AutoOn = val == "1" || val == "true"
	}

	if s.Studio != nil {
		jobs := s.Studio.ListViralJobs(40)
		v.TotalCount = len(jobs)
		for _, j := range jobs {
			var p studio.ViralParams
			_ = studioUnmarshalJSON(j.Params, &p)

			var mediaURL string
			var srtURL string
			if j.Status == studio.StatusDone && j.Output != "" {
				mediaURL = "/media/" + filepath.Base(j.Output)
				srtFile := strings.TrimSuffix(j.Output, filepath.Ext(j.Output)) + ".srt"
				if fileExists(srtFile) {
					srtURL = "/media/" + filepath.Base(srtFile)
				}
			}

			topic := p.Topic
			if topic == "" {
				topic = j.Title
			}

			lines := strings.Split(strings.TrimSpace(j.Log), "\n")
			tail := ""
			if len(lines) > 0 {
				tail = lines[len(lines)-1]
			}

			jv := viralJobView{
				ID:         j.ID,
				Title:      j.Title,
				Topic:      topic,
				Genre:      p.Genre,
				GenreVN:    viralGenreVN(p.Genre),
				Status:     j.Status,
				StatusVN:   viralStatusVN(j.Status),
				Progress:   j.Progress,
				MediaURL:   mediaURL,
				SRTURL:     srtURL,
				LogTail:    tail,
				CreatedAt:  j.CreatedAt,
				FinishedAt: j.FinishedAt,
			}
			v.Jobs = append(v.Jobs, jv)

			if j.Status == studio.StatusRunning || j.Status == studio.StatusQueued {
				v.ActiveCount++
			} else if j.Status == studio.StatusDone {
				v.DoneCount++
			}
		}
	}

	s.render(w, "viral", s.ctx("Viral", v))
}
