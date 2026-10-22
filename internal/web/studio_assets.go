package web

import (
	"net/http"
	"path/filepath"
	"regexp"
	"strings"
)

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
