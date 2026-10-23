package web

import (
	"fmt"
	"github.com/ninhlee99/ai-creator-os/internal/studio"
	"net/http"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

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

// handleStudioGenerateImage generates an image via Gemini or a neutral card fallback.
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
			geminiPrompt = fmt.Sprintf("High-resolution photorealistic product scene of %s, %s, cinematic studio lighting, no text, no watermark", productName, niche)
		}
		if gerr := s.Studio.MediaGen().GenerateImage(r.Context(), geminiPrompt, nil, outPath); gerr == nil {
			source = "gemini"
		}
	}

	// If Gemini was not used or failed, generate high-quality visual card
	if source != "gemini" {
		if err := studio.GenerateProductVisual(productName, niche, "", wPx, hPx, outPath); err != nil {
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
