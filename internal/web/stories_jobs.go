package web

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/ninhlee99/ai-creator-os/internal/studio"
	"log"
	"net/http"
	"path/filepath"
	"strings"
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
