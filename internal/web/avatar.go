//go:build parked

package web

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/ninhlee99/ai-creator-os/internal/ledger"
)

// ------------------------------------------------------- avatar: characters

// characterView is the template projection of a ledger character.
type characterView struct {
	ledger.Character
	// ImgURL serves the reference portrait (/avatars/<file>).
	ImgURL string
	// ShortLock is the first 12 chars of the identity lock.
	ShortLock string
}

func (s *Server) characterViews() []characterView {
	chars, err := s.Ledger.ListCharacters()
	if err != nil {
		log.Printf("web: list characters: %v", err)
		return nil
	}
	out := make([]characterView, 0, len(chars))
	for _, c := range chars {
		out = append(out, characterView{
			Character: c,
			ImgURL:    "/avatars/" + filepath.Base(c.ReferenceImage),
			ShortLock: shortLock(c.IdentityLock),
		})
	}
	return out
}

func shortLock(lock string) string {
	if len(lock) > 12 {
		return lock[:12]
	}
	return lock
}

// avatarSidecarView mirrors vieneuView for the avatar sidecar panel.
type avatarSidecarView struct {
	Connected       bool
	State           string
	Detail          string
	ModelConfigured bool
	ModelPresent    bool
}

func (s *Server) avatarSidecarView() avatarSidecarView {
	v := avatarSidecarView{}
	if s.AvatarSidecar == nil {
		return v
	}
	v.Connected = true
	v.State, v.Detail = s.AvatarSidecar.Status()
	v.ModelConfigured = s.AvatarSidecar.ModelConfigured()
	v.ModelPresent = s.AvatarSidecar.ModelPresent()
	return v
}

// kickAvatarEnsure starts the avatar sidecar model ensure in the
// background; progress is polled via /settings/avatar-sidecar/progress.
func (s *Server) kickAvatarEnsure() {
	if s.AvatarSidecar == nil {
		return
	}
	s.avatarDlMu.Lock()
	s.avatarDlDone, s.avatarDlErr = false, ""
	s.avatarDlMu.Unlock()
	go func() {
		err := s.AvatarSidecar.EnsureModel(context.Background(), func(downloaded, total int64) {
			s.avatarDlMu.Lock()
			s.avatarDlDownloaded, s.avatarDlTotal = downloaded, total
			s.avatarDlMu.Unlock()
		})
		s.avatarDlMu.Lock()
		s.avatarDlDone = true
		if err != nil {
			s.avatarDlErr = err.Error()
		}
		s.avatarDlMu.Unlock()
	}()
}

// handleAvatarCharacters serves the character list as JSON (used by the
// settings page refresh after upload/delete).
func (s *Server) handleAvatarCharacters(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"ok":         true,
		"characters": s.characterViews(),
	})
}

// handleAvatarCharacterCreate accepts a multipart form (name, image file)
// and registers a new character. The identity lock is computed here from
// the uploaded bytes + seed so it can never be out of sync at creation.
func (s *Server) handleAvatarCharacterCreate(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(8 << 20); err != nil { // 8MB max portrait
		writeJSONErr(w, "không đọc được form upload: "+err.Error(), http.StatusBadRequest)
		return
	}
	name := strings.TrimSpace(r.PostFormValue("name"))
	if name == "" {
		writeJSONErr(w, "thiếu tên nhân vật", http.StatusBadRequest)
		return
	}
	f, hdr, err := r.FormFile("image")
	if err != nil {
		writeJSONErr(w, "thiếu file ảnh chân dung", http.StatusBadRequest)
		return
	}
	defer f.Close()
	ext := strings.ToLower(filepath.Ext(hdr.Filename))
	if ext != ".png" && ext != ".jpg" && ext != ".jpeg" {
		writeJSONErr(w, "ảnh phải là PNG hoặc JPG", http.StatusBadRequest)
		return
	}
	img, err := io.ReadAll(io.LimitReader(f, 8<<20))
	if err != nil || len(img) == 0 {
		writeJSONErr(w, "không đọc được ảnh", http.StatusBadRequest)
		return
	}
	seed := time.Now().UnixNano()
	if sv := strings.TrimSpace(r.PostFormValue("seed")); sv != "" {
		if parsed, perr := strconv.ParseInt(sv, 10, 64); perr == nil && parsed != 0 {
			seed = parsed
		}
	}
	voice := strings.TrimSpace(r.PostFormValue("voice"))
	notes := strings.TrimSpace(r.PostFormValue("notes"))

	if err := os.MkdirAll(s.AvatarDir, 0o755); err != nil {
		writeJSONErr(w, "không tạo được thư mục avatars", http.StatusInternalServerError)
		return
	}
	filename := fmt.Sprintf("char-%d%s", time.Now().UnixNano(), ext)
	imgPath := filepath.Join(s.AvatarDir, filename)
	if err := os.WriteFile(imgPath, img, 0o644); err != nil {
		writeJSONErr(w, "không lưu được ảnh", http.StatusInternalServerError)
		return
	}
	lock := computeIdentityLock(img, seed)
	id, err := s.Ledger.CreateCharacter(name, imgPath, seed, lock, voice, notes)
	if err != nil {
		_ = os.Remove(imgPath)
		writeJSONErr(w, "không lưu được nhân vật: "+err.Error(), http.StatusInternalServerError)
		return
	}
	_ = s.Ledger.Decide("human", "avatar_character_add", strptr(fmt.Sprintf("character:%d", id)),
		fmt.Sprintf("thêm nhân vật %q (seed=%d, lock=%s)", name, seed, shortLock(lock)),
		map[string]any{"character_id": id, "name": name, "seed": seed, "identity_lock": shortLock(lock)})
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "id": id})
}

// computeIdentityLock mirrors avatar.ComputeIdentityLock (sha256 of image
// bytes + "|" + seed) without importing internal/engines from web.
func computeIdentityLock(img []byte, seed int64) string {
	h := sha256.New()
	h.Write(img)
	h.Write([]byte("|"))
	h.Write([]byte(strconv.FormatInt(seed, 10)))
	return hex.EncodeToString(h.Sum(nil))
}

// handleAvatarCharacterDelete removes a character (DB row; the image file
// is kept on disk for audit).
func (s *Server) handleAvatarCharacterDelete(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		writeJSONErr(w, "id không hợp lệ", http.StatusBadRequest)
		return
	}
	ch, err := s.Ledger.GetCharacter(id)
	if err != nil {
		writeJSONErr(w, "không tìm thấy nhân vật", http.StatusNotFound)
		return
	}
	if err := s.Ledger.DeleteCharacter(id); err != nil {
		writeJSONErr(w, "không xóa được: "+err.Error(), http.StatusInternalServerError)
		return
	}
	_ = s.Ledger.Decide("human", "avatar_character_delete", strptr(fmt.Sprintf("character:%d", id)),
		fmt.Sprintf("xóa nhân vật %q", ch.Name),
		map[string]any{"character_id": id, "name": ch.Name})
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
}

// handleAvatarImage serves a reference portrait. The filename is strictly
// the base name inside AvatarDir (no path traversal).
func (s *Server) handleAvatarImage(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("file")
	if name == "" || name != filepath.Base(name) || strings.Contains(name, "..") {
		http.Error(w, "bad file", http.StatusBadRequest)
		return
	}
	path := filepath.Join(s.AvatarDir, name)
	f, err := os.Open(path)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer f.Close()
	// sniff content type from extension
	switch strings.ToLower(filepath.Ext(name)) {
	case ".png":
		w.Header().Set("Content-Type", "image/png")
	case ".jpg", ".jpeg":
		w.Header().Set("Content-Type", "image/jpeg")
	}
	http.ServeContent(w, r, name, time.Now(), f)
}

// ------------------------------------------------------- avatar: render test

// handleAvatarRenderTest renders a short test clip for a character speaking
// the given text (TTS → emotion cues → avatar). It returns the mp4's
// /media/ URL for inline preview. The whole flow is logged to decisions.
func (s *Server) handleAvatarRenderTest(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if s.Avatar == nil {
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": "avatar chain chưa được kết nối"})
		return
	}
	var req struct {
		CharacterID int64  `json:"character_id"`
		Text        string `json:"text"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil {
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": "JSON không hợp lệ"})
		return
	}
	req.Text = strings.TrimSpace(req.Text)
	if req.CharacterID <= 0 || req.Text == "" {
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": "thiếu character_id hoặc text"})
		return
	}
	ch, err := s.Ledger.GetCharacter(req.CharacterID)
	if err != nil {
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": "không tìm thấy nhân vật"})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 35*time.Minute)
	defer cancel()
	rel, err := s.Avatar.RenderTestClip(ctx, req.CharacterID, req.Text)
	if err != nil {
		_ = s.Ledger.Decide("human", "avatar_render_test", strptr(fmt.Sprintf("character:%d", req.CharacterID)),
			fmt.Sprintf("render thử thất bại cho %q: %v", ch.Name, err),
			map[string]any{"character_id": req.CharacterID, "error": err.Error()})
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": err.Error()})
		return
	}
	_ = s.Ledger.Decide("human", "avatar_render_test", strptr(fmt.Sprintf("character:%d", req.CharacterID)),
		fmt.Sprintf("render thử cho %q: %s", ch.Name, rel),
		map[string]any{"character_id": req.CharacterID, "video": rel})
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "video": rel})
}

// ------------------------------------------------------- avatar: sidecar

func (s *Server) handleAvatarSidecarStatus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	v := s.avatarSidecarView()
	_ = json.NewEncoder(w).Encode(map[string]any{
		"ok": true, "connected": v.Connected, "state": v.State, "detail": v.Detail,
		"model_configured": v.ModelConfigured, "model_present": v.ModelPresent,
	})
}

// avatarDownloadProgress mirrors the VieNeu download-progress pattern.
func (s *Server) handleAvatarSidecarEnsure(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if s.AvatarSidecar == nil {
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": "sidecar chưa được kết nối"})
		return
	}
	s.kickAvatarEnsure()
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
}

func (s *Server) handleAvatarSidecarProgress(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	s.avatarDlMu.Lock()
	defer s.avatarDlMu.Unlock()
	out := map[string]any{
		"done":       s.avatarDlDone,
		"downloaded": s.avatarDlDownloaded,
		"total":      s.avatarDlTotal,
	}
	if s.avatarDlErr != "" {
		out["error"] = s.avatarDlErr
	}
	_ = json.NewEncoder(w).Encode(out)
}

func (s *Server) handleAvatarSidecarRestart(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if s.AvatarSidecar == nil {
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": "sidecar chưa được kết nối"})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 6*time.Minute)
	defer cancel()
	if err := s.AvatarSidecar.Restart(ctx); err != nil {
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": err.Error()})
		return
	}
	_ = s.Ledger.Decide("human", "avatar_sidecar_restart", nil, "restart avatar sidecar từ dashboard", map[string]any{})
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
}

func strptr(s string) *string { return &s }
