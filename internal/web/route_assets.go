package web

import (
	"net/http"
	"path/filepath"
)

func setAssetCache(w http.ResponseWriter, r *http.Request) {
	if r.URL.Query().Get("v") == assetVersion {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		return
	}
	w.Header().Set("Cache-Control", "no-cache")
}

// handleStaticCSS serves the embedded dashboard stylesheet.
func (s *Server) handleStaticCSS(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/css; charset=utf-8")
	setAssetCache(w, r)
	_, _ = w.Write(staticCSS)
}

func (s *Server) handleStaticHTMX(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
	setAssetCache(w, r)
	_, _ = w.Write(staticHTMX)
}

// handleStaticJS serves the embedded shared client script (toast, confirm
// modal, studio polling — R2-W3: một nguồn JS dùng chung toàn app).
func (s *Server) handleStaticJS(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
	setAssetCache(w, r)
	_, _ = w.Write(staticJS)
}

func (s *Server) handleStaticWebAILoginJS(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
	setAssetCache(w, r)
	_, _ = w.Write(staticWebAILoginJS)
}

func (s *Server) handleStaticFlowCanvasJS(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
	setAssetCache(w, r)
	_, _ = w.Write(staticFlowCanvasJS)
}

// handleStaticPreviewAudio serves the bundled sample preview music.
func (s *Server) handleStaticPreviewAudio(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "audio/mpeg")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	_, _ = w.Write(staticPreviewAudio)
}

// handleStaticSampleVideo serves the bundled 9:16 sample video for flow canvas previews.
func (s *Server) handleStaticSampleVideo(w http.ResponseWriter, r *http.Request) {
	p := filepath.Join("internal", "web", "static", "sample_916.mp4")
	w.Header().Set("Content-Type", "video/mp4")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	http.ServeFile(w, r, p)
}

// --------------------------------------------------------------- dashboard
