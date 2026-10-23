package web

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/ninhlee99/ai-creator-os/internal/automation"
	"github.com/ninhlee99/ai-creator-os/internal/studio"
	"log"
	"net/http"
	"path/filepath"
	"strings"
	"time"
)

func (s *Server) handleStoryPublish(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	account := strings.TrimSpace(r.FormValue("account"))
	if account == "" {
		storiesRedirect(w, r, "", "Chọn kênh YouTube để đăng.")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*60*1000000000)
	defer cancel()
	note, ok := s.automation().PublishStoryNow(ctx, id, account)
	if !ok {
		storiesRedirect(w, r, "", note)
		return
	}
	storiesRedirect(w, r, note, "")
}

func (s *Server) handleStoryPublishReview(w http.ResponseWriter, r *http.Request) {
	if s.Ledger == nil || s.Studio == nil {
		storiesRedirect(w, r, "", "Studio hoặc Ledger chưa sẵn sàng.")
		return
	}
	id := r.PathValue("id")
	job, ok := s.Studio.GetJob(id)
	if !ok || job.Kind != studio.KindStory || job.Status != studio.StatusDone {
		storiesRedirect(w, r, "", "Chỉ đối chiếu được job Story đã dựng xong.")
		return
	}
	_, published, err := s.Ledger.GetSetting(automation.KeyStoryPublishedPrefix + id)
	if err != nil {
		storiesRedirect(w, r, "", "Không đọc được trạng thái đăng: "+err.Error())
		return
	}
	if published {
		_ = s.Ledger.DeleteSetting(automation.KeyStoryNeedsReviewPrefix + id)
		_ = s.Ledger.DeleteSetting(automation.KeyStoryPublishingPrefix + id)
		storiesRedirect(w, r, "Truyện đã được ghi nhận trên YouTube.", "")
		return
	}
	_, needsReview, err := s.Ledger.GetSetting(automation.KeyStoryNeedsReviewPrefix + id)
	if err != nil {
		storiesRedirect(w, r, "", "Không đọc được trạng thái đối chiếu: "+err.Error())
		return
	}
	if !needsReview {
		storiesRedirect(w, r, "", "Job này không còn cần đối chiếu.")
		return
	}
	switch r.PostFormValue("resolution") {
	case "published":
		remoteID := strings.TrimSpace(r.PostFormValue("remote_id"))
		if remoteID == "" {
			storiesRedirect(w, r, "", "Nhập YouTube video ID đã kiểm tra.")
			return
		}
		if err := s.Ledger.SetSetting(automation.KeyStoryPublishedPrefix+id, remoteID); err != nil {
			storiesRedirect(w, r, "", "Không lưu được xác nhận: "+err.Error())
			return
		}
		if err := s.Ledger.DeleteSetting(automation.KeyStoryNeedsReviewPrefix + id); err != nil {
			storiesRedirect(w, r, "", "Video đã ghi nhận nhưng chưa xóa được cờ đối chiếu: "+err.Error())
			return
		}
		if err := s.Ledger.DeleteSetting(automation.KeyStoryPublishingPrefix + id); err != nil {
			storiesRedirect(w, r, "", "Video đã ghi nhận nhưng chưa xóa được marker upload: "+err.Error())
			return
		}
		_ = s.Ledger.Decide("human", "story_publish_reconcile", &id,
			"đã xác nhận video Story tồn tại trên YouTube.", map[string]any{
				"job_id": id, "video": remoteID, "resolution": "published",
			})
		storiesRedirect(w, r, "Đã xác nhận video tồn tại trên YouTube.", "")
	case "retry":
		account := strings.TrimSpace(r.PostFormValue("account"))
		if account == "" || s.Mgr == nil {
			storiesRedirect(w, r, "", "Chọn kênh YouTube trước khi thử lại.")
			return
		}
		accounts, err := s.Mgr.List()
		if err != nil {
			storiesRedirect(w, r, "", "Không đọc được danh sách kênh: "+err.Error())
			return
		}
		accountExists := false
		for _, candidate := range accounts {
			if candidate.Username == account {
				accountExists = true
				break
			}
		}
		if !accountExists {
			storiesRedirect(w, r, "", "Kênh YouTube không còn trong danh sách.")
			return
		}
		if err := s.Ledger.DeleteSetting(automation.KeyStoryPublishingPrefix + id); err != nil {
			storiesRedirect(w, r, "", "Không xóa được marker upload cũ: "+err.Error())
			return
		}
		if err := s.Ledger.DeleteSetting(automation.KeyStoryNeedsReviewPrefix + id); err != nil {
			storiesRedirect(w, r, "", "Không xóa được cờ đối chiếu: "+err.Error())
			return
		}
		_ = s.Ledger.Decide("human", "story_publish_reconcile", &id,
			"đã xác nhận chưa có video trên YouTube; cho phép thử lại.", map[string]any{
				"job_id": id, "resolution": "retry", "account": account,
			})
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Minute)
		defer cancel()
		note, ok := s.automation().PublishStoryNow(ctx, id, account)
		if !ok {
			_ = s.Ledger.SetSetting(automation.KeyStoryNeedsReviewPrefix+id, note)
			storiesRedirect(w, r, "", note)
			return
		}
		storiesRedirect(w, r, note, "")
	default:
		storiesRedirect(w, r, "", "Chọn kết quả đối chiếu hợp lệ.")
	}
}

// handleStoryRetry tạo job mới từ job lỗi (thử lại tay, đợt Y): cùng
// params, Attempt=0, tái dùng ảnh đã vẽ xong ở job cũ.
func (s *Server) handleStoryRetry(w http.ResponseWriter, r *http.Request) {
	if s.Studio == nil {
		storiesRedirect(w, r, "", "Studio chưa sẵn sàng.")
		return
	}
	id := r.PathValue("id")
	j, ok := s.Studio.GetJob(id)
	if !ok || j.Kind != studio.KindStory {
		storiesRedirect(w, r, "", "Không tìm thấy truyện để thử lại.")
		return
	}
	var p studio.StoryParams
	if err := json.Unmarshal([]byte(j.Params), &p); err != nil {
		storiesRedirect(w, r, "", "Tham số truyện hỏng — tạo truyện mới tay.")
		return
	}
	p.Attempt = 0
	p.ReuseFromJobID = id
	// Nhạc nền: cùng file chung như lúc tạo hoặc nhạc mặc định.
	if p.MusicOn {
		if mp := filepath.Join(s.DataDir, "autopilot-music.m4a"); fileExists(mp) {
			p.MusicPath = mp
		} else if defM := studio.FindDefaultMusic(); defM != "" {
			p.MusicPath = defM
		} else {
			p.MusicOn = false
		}
	}
	newID, err := s.Studio.CreateStoryJob(p)
	if err != nil {
		storiesRedirect(w, r, "", err.Error())
		return
	}
	log.Printf("web: story retry %s → job mới %s", id, newID)
	storiesRedirect(w, r, "Đã tạo lại truyện (tái dùng ảnh đã vẽ xong ở lần trước).", "")
}

// handleStoryLog trả toàn bộ log của job (dạng text, mở tab mới, đợt Y).
func (s *Server) handleStoryLog(w http.ResponseWriter, r *http.Request) {
	if s.Studio == nil {
		http.Error(w, "Studio chưa sẵn sàng.", http.StatusServiceUnavailable)
		return
	}
	id := r.PathValue("id")
	j, ok := s.Studio.GetJob(id)
	if !ok || j.Kind != studio.KindStory {
		http.Error(w, "Không tìm thấy truyện.", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	fmt.Fprint(w, s.Studio.StoryLogFull(id))
}

// handleStoryDelete xoá 1 job kể chuyện (form POST, có confirm ở UI).
func (s *Server) handleStoryDelete(w http.ResponseWriter, r *http.Request) {
	if s.Studio == nil {
		storiesRedirect(w, r, "", "Studio chưa sẵn sàng.")
		return
	}
	if err := s.Studio.DeleteStoryJob(r.PathValue("id")); err != nil {
		storiesRedirect(w, r, "", err.Error())
		return
	}
	storiesRedirect(w, r, "Đã xoá truyện.", "")
}

// handleStorySettings lưu mặc định kể chuyện (form POST).
