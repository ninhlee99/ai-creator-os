package web

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"encoding/xml"
	"github.com/ninhlee99/ai-creator-os/internal/studio"
	"io"
	"log"
	"net/http"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
)

func (s *Server) handleStories(w http.ResponseWriter, r *http.Request) {
	data := s.ctx()
	v := s.storiesPageData()
	if !v.StudioOK {
		v.Error = "Studio chưa được khởi tạo — kiểm tra log khởi động app."
	}
	data["Stories"] = v
	data["ok"] = r.URL.Query().Get("ok")
	data["err"] = r.URL.Query().Get("err")
	s.render(w, "stories", data)
}

// storiesRedirect chuyển về /stories kèm toast (app.js tự đọc ?ok=/?err=).
func storiesRedirect(w http.ResponseWriter, r *http.Request, okMsg, errMsg string) {
	loc := "/stories"
	if errMsg != "" {
		loc += "?err=" + url.QueryEscape(errMsg)
	} else if okMsg != "" {
		loc += "?ok=" + url.QueryEscape(okMsg)
	}
	seeOther(w, r, loc)
}

// handleStoryCreate tạo job kể chuyện mới (form POST).
func (s *Server) handleStoryCreate(w http.ResponseWriter, r *http.Request) {
	if s.Studio == nil {
		storiesRedirect(w, r, "", "Studio chưa sẵn sàng.")
		return
	}
	p := storyParamsFromForm(r)
	if p.Topic == "" && p.CustomText != "" {
		for _, line := range strings.Split(p.CustomText, "\n") {
			if line = strings.TrimSpace(line); line != "" {
				p.Topic = strings.TrimPrefix(strings.TrimPrefix(line, "#"), "Chương 1:")
				p.Topic = strings.TrimSpace(p.Topic)
				break
			}
		}
	}
	if strings.TrimSpace(p.Topic) == "" {
		storiesRedirect(w, r, "", "Nhập chủ đề truyện.")
		return
	}
	// Nhạc nền: cùng file autopilot (không bắt Ninh upload riêng) hoặc nhạc mặc định.
	if p.MusicOn {
		if mp := filepath.Join(s.DataDir, "autopilot-music.m4a"); fileExists(mp) {
			p.MusicPath = mp
		} else if defM := studio.FindDefaultMusic(); defM != "" {
			p.MusicPath = defM
		} else {
			p.MusicOn = false
		}
	}
	id, err := s.Studio.CreateStoryJob(p)
	if err != nil {
		storiesRedirect(w, r, "", err.Error())
		return
	}
	log.Printf("web: story job %s đã tạo (chủ đề %q)", id, p.Topic)
	storiesRedirect(w, r, "Đã xếp truyện vào hàng đợi — theo dõi tiến trình bên dưới.", "")
}

// storyParamsFromForm đọc form tạo truyện + clamp giá trị.
func storyParamsFromForm(r *http.Request) studio.StoryParams {
	durMin, _ := strconv.Atoi(r.FormValue("duration_minutes"))
	if durMin <= 0 {
		durMin = 30
	}
	words, _ := strconv.Atoi(r.FormValue("words"))
	scenes, _ := strconv.Atoi(r.FormValue("scenes"))
	genre := strings.TrimSpace(r.FormValue("genre"))
	autoClip := r.FormValue("auto_clip_shorts") != "0"
	return studio.StoryParams{
		Topic:           strings.TrimSpace(r.FormValue("topic")),
		CustomText:      strings.TrimSpace(r.FormValue("custom_story_text")),
		Genre:           genre,
		DurationMinutes: durMin,
		Words:           words,
		Scenes:          scenes,
		MusicOn:         r.FormValue("music_on") == "1",
		Voice:           strings.TrimSpace(r.FormValue("voice")),
		AutoClipShorts:  autoClip,
	}
}

// handleStoryUploadText extracts readable text from .txt or .docx uploads.
func (s *Server) handleStoryUploadText(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 5<<20)
	if err := r.ParseMultipartForm(5 << 20); err != nil {
		http.Error(w, "Tệp vượt giới hạn 5 MB hoặc không hợp lệ", http.StatusBadRequest)
		return
	}
	f, header, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "Thiếu tệp truyện", http.StatusBadRequest)
		return
	}
	defer f.Close()
	var text string
	switch strings.ToLower(filepath.Ext(header.Filename)) {
	case ".txt":
		b, err := io.ReadAll(io.LimitReader(f, 5<<20))
		if err != nil {
			http.Error(w, "Không đọc được tệp", http.StatusBadRequest)
			return
		}
		text = string(b)
	case ".docx":
		b, err := io.ReadAll(io.LimitReader(f, 5<<20))
		if err != nil {
			http.Error(w, "Không đọc được tệp", http.StatusBadRequest)
			return
		}
		zr, err := zip.NewReader(bytes.NewReader(b), int64(len(b)))
		if err != nil {
			http.Error(w, "Tệp DOCX không hợp lệ", http.StatusBadRequest)
			return
		}
		for _, item := range zr.File {
			if item.Name != "word/document.xml" {
				continue
			}
			if item.UncompressedSize64 > 8<<20 {
				http.Error(w, "Văn bản DOCX vượt giới hạn 8 MB", http.StatusRequestEntityTooLarge)
				return
			}
			rc, err := item.Open()
			if err != nil {
				break
			}
			dec := xml.NewDecoder(rc)
			var out strings.Builder
			inText := false
			for {
				tok, err := dec.Token()
				if err == io.EOF {
					break
				}
				if err != nil {
					rc.Close()
					http.Error(w, "Không đọc được nội dung DOCX", http.StatusBadRequest)
					return
				}
				switch tok := tok.(type) {
				case xml.StartElement:
					if tok.Name.Local == "t" {
						inText = true
					}
					if tok.Name.Local == "p" && out.Len() > 0 {
						out.WriteByte('\n')
					}
				case xml.EndElement:
					if tok.Name.Local == "t" {
						inText = false
					}
				case xml.CharData:
					if inText {
						out.Write([]byte(tok))
						if out.Len() > 4<<20 {
							rc.Close()
							http.Error(w, "Văn bản DOCX vượt giới hạn 4 MB", http.StatusRequestEntityTooLarge)
							return
						}
					}
				}
			}
			rc.Close()
			text = out.String()
			break
		}
	default:
		http.Error(w, "Chỉ hỗ trợ tệp .txt hoặc .docx", http.StatusUnsupportedMediaType)
		return
	}
	text = strings.TrimSpace(text)
	if text == "" {
		http.Error(w, "Tệp không có văn bản đọc được", http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(map[string]string{"text": text})
}

// handleStoryPublish đăng 1 truyện đã dựng lên YouTube (private-first).
