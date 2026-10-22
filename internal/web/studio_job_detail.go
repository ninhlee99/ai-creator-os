package web

import (
	"encoding/json"
	"fmt"
	"github.com/ninhlee99/ai-creator-os/internal/studio"
	"net/http"
	"path/filepath"
	"strings"
)

// studioAssetView projects a storyboard asset for JSON.
// Label/Class are server-rendered so clients never redefine them (R2-W3).
type studioAssetView struct {
	ID       int64  `json:"id"`
	Idx      int    `json:"idx"`
	Kind     string `json:"kind"`
	Status   string `json:"status"`
	Prompt   string `json:"prompt"`
	Preview  string `json:"preview"`
	ImageURL string `json:"image_url,omitempty"`
	VideoURL string `json:"video_url,omitempty"`
	Label    string `json:"label"`
	Class    string `json:"class"`
}

// handleStudioJobDetail returns one job + its storyboard assets as JSON.
func (s *Server) handleStudioJobDetail(w http.ResponseWriter, r *http.Request) {
	if s.Studio == nil {
		http.NotFound(w, r)
		return
	}
	id := r.PathValue("id")
	j, ok := s.Studio.GetJob(id)
	if !ok {
		http.NotFound(w, r)
		return
	}
	assets := s.Studio.ListAssets(id)
	views := make([]studioAssetView, 0, len(assets))
	jobDir := filepath.Join(s.Studio.WorkDir(), id)

	for _, a := range assets {
		v := studioAssetView{
			ID:     a.ID,
			Idx:    a.Idx,
			Kind:   a.Kind,
			Status: a.Status,
			Prompt: a.Prompt,
			Label:  statusLabel(a.Status),
			Class:  statusClass(a.Status),
		}

		if a.Status == "done" {
			// 1. Check for Image Asset
			imgExts := []string{".png", ".jpg", ".jpeg", ".webp"}
			isImg := false
			for _, ext := range imgExts {
				if strings.HasSuffix(strings.ToLower(a.Path), ext) {
					isImg = true
					break
				}
			}

			if isImg && a.Path != "" {
				v.ImageURL = "/studio/assets/" + id + "/" + filepath.Base(a.Path)
			} else {
				// Search disk in jobDir for keyframe image
				imgCandidates := []string{
					fmt.Sprintf("scene_%02d.png", a.Idx),
					fmt.Sprintf("scene_%02d.jpg", a.Idx),
					fmt.Sprintf("scene-%02d.png", a.Idx),
					fmt.Sprintf("scene-%02d.jpg", a.Idx),
					fmt.Sprintf("photo-%02d.png", a.Idx),
					fmt.Sprintf("photo-%02d.png", a.Idx-1),
					fmt.Sprintf("photo-%02d.jpg", a.Idx),
					fmt.Sprintf("photo-%02d.jpg", a.Idx-1),
					fmt.Sprintf("own%d_0.png", a.Idx),
					fmt.Sprintf("own%d_0.jpg", a.Idx),
				}
				for _, c := range imgCandidates {
					if fileExists(filepath.Join(jobDir, c)) {
						v.ImageURL = "/studio/assets/" + id + "/" + c
						break
					}
				}
			}

			// 2. Check for Video Asset
			vidExts := []string{".mp4", ".mov", ".webm"}
			isVid := false
			for _, ext := range vidExts {
				if strings.HasSuffix(strings.ToLower(a.Path), ext) {
					isVid = true
					break
				}
			}

			if isVid && a.Path != "" {
				v.VideoURL = "/studio/assets/" + id + "/" + filepath.Base(a.Path)
			} else {
				// Search disk in jobDir for motion clip
				vidCandidates := []string{
					fmt.Sprintf("motion_%02d.mp4", a.Idx),
					fmt.Sprintf("motion_%02d.mp4", a.Idx+1),
					fmt.Sprintf("clip_%02d.mp4", a.Idx),
					fmt.Sprintf("clip_%02d.mp4", a.Idx+1),
				}
				for _, c := range vidCandidates {
					if fileExists(filepath.Join(jobDir, c)) {
						v.VideoURL = "/studio/assets/" + id + "/" + c
						break
					}
				}
			}

			// 3. Set Preview
			if v.ImageURL != "" {
				v.Preview = v.ImageURL
			} else if v.VideoURL != "" {
				v.Preview = v.VideoURL
			} else if a.Path != "" {
				v.Preview = "/studio/assets/" + id + "/" + filepath.Base(a.Path)
			}
		}

		views = append(views, v)
	}

	resp := map[string]any{
		"job":    j,
		"assets": views,
	}
	// Checklist lên xu hướng cho video affiliate đã xong (đo từ file thật).
	if j.Kind == studio.KindAffiliate && j.Status == studio.StatusDone && j.Output != "" {
		var p studio.AffiliateParams
		_ = json.Unmarshal([]byte(j.Params), &p)
		outPath := j.Output
		if s.Studio != nil {
			outPath = s.Studio.OutputPath(j.Output)
		}
		resp["trend"] = studio.TrendReadiness(r.Context(), outPath, p, j.Caption)
	}
	s.writeJSON(w, resp)
}
