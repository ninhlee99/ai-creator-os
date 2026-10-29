package web

import (
	"github.com/ninhlee99/ai-creator-os/internal/network"
	"github.com/ninhlee99/ai-creator-os/internal/studio"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
)

// videoJobView là dữ liệu hiển thị của một tác vụ dựng video AI.
type videoJobView struct {
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

type viralJobView = videoJobView

// videoCreateView là mô hình dữ liệu chính cho trang tạo video AI (/workflow/create).
type videoCreateView struct {
	StudioOK              bool
	Jobs                  []videoJobView
	AccountNames          []string
	AccountOptions        []*network.Account
	SelectedAccountID     int64
	CharacterEditorReady  bool
	CharacterEditorStatus string
	ActiveCount           int
	DoneCount             int
	TotalCount            int
	Error                 string
	Notice                string
	Warning               string
	AvailableModels       []modelPhotoCardView
	SelectedModelID       int64
	SelectedModelPhoto    string
	SourceVideoURL        string
	SourceMusicTitle      string
	Topic                 string
	Prompt                string
	OutputType            string
	AvailableWorkflows    []WorkflowOption
	SelectedWorkflowID    string
}

type viralView = videoCreateView

// WorkflowOption chứa thông tin tóm tắt của workflow để lựa chọn tại các màn hình tạo video.
type WorkflowOption struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Category    string `json:"category"`
	Description string `json:"description"`
	IsPreset    bool   `json:"is_preset"`
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

func (s *Server) handleVideoCreate(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	srcURL := strings.TrimSpace(q.Get("tiktok_link"))
	if srcURL == "" {
		srcURL = strings.TrimSpace(q.Get("source_video_url"))
	}
	outputType := strings.TrimSpace(q.Get("type"))
	if outputType == "" {
		outputType = strings.TrimSpace(q.Get("output_type"))
	}
	wfID := strings.TrimSpace(q.Get("workflow_id"))
	if outputType == "" {
		if strings.Contains(wfID, "affiliate") {
			outputType = "affiliate"
		} else if strings.Contains(wfID, "viral") || srcURL != "" {
			outputType = "viral"
		} else {
			outputType = "auto"
		}
	}
	if wfID == "" {
		if outputType == "affiliate" {
			wfID = "preset_affiliate_daily"
		} else if outputType == "viral" {
			wfID = "preset_viral_dance"
		} else {
			wfID = "auto"
		}
	}
	v := viralView{
		StudioOK:           s.Studio != nil,
		Error:              q.Get("err"),
		Notice:             q.Get("notice"),
		Warning:            q.Get("warn"),
		AvailableModels:    s.listModelPhotoCards(),
		SourceVideoURL:     srcURL,
		SourceMusicTitle:   strings.TrimSpace(q.Get("source_music_title")),
		Topic:              strings.TrimSpace(q.Get("topic")),
		Prompt:             strings.TrimSpace(q.Get("prompt")),
		OutputType:         outputType,
		SelectedWorkflowID: wfID,
	}
	if s.Studio != nil {
		v.CharacterEditorReady, v.CharacterEditorStatus = s.Studio.ViralCharacterEditorStatus(r.Context())
		if fs, err := s.Studio.GetFlowStore(); err == nil && fs != nil {
			if list, err := fs.ListGraphs(); err == nil {
				for _, sum := range list {
					v.AvailableWorkflows = append(v.AvailableWorkflows, WorkflowOption{
						ID:          sum.ID,
						Title:       sum.Name,
						Category:    sum.Category,
						Description: sum.Description,
						IsPreset:    sum.IsPreset,
					})
				}
			}
		}
	}
	if msg := q.Get("msg"); msg != "" {
		if msg == "created" {
			v.Notice = "Tác vụ sản xuất video viral AI đã được đưa vào hàng đợi thành công!"
		} else {
			v.Notice = msg
		}
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

	// Source video comes from a TikTok Radar link or an operator-uploaded file.

	if s.Studio != nil {
		jobs := s.Studio.ListJobs(30)
		v.TotalCount = len(jobs)
		for _, j := range jobs {
			var p studio.ViralParams
			if j.Params != "" {
				_ = studioUnmarshalJSON(j.Params, &p)
			}

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

			genre := p.Genre
			if genre == "" {
				genre = j.Kind
			}

			jv := viralJobView{
				ID:         j.ID,
				Title:      j.Title,
				Topic:      topic,
				Genre:      genre,
				GenreVN:    viralGenreVN(genre),
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

	s.render(w, "video_create", s.ctx("VideoCreate", v, "Video", v, "Viral", v))
}

// handleViral giữ tương thích cho router cũ.
func (s *Server) handleViral(w http.ResponseWriter, r *http.Request) {
	s.handleVideoCreate(w, r)
}
