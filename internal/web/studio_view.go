package web

import (
	"encoding/json"
	"github.com/ninhlee99/ai-creator-os/internal/studio"
	"path/filepath"
	"strings"
)

// ---------------------------------------------------------------------------
// Studio page: AI video creation (affiliate)
// ---------------------------------------------------------------------------

// studioJobView is the template/JSON projection of a studio job.
// Label/Class are rendered server-side so clients never redefine status
// labels (R2-W3: một nguồn nhãn duy nhất).
type studioJobView struct {
	ID                     string `json:"id"`
	Kind                   string `json:"kind"`
	Title                  string `json:"title"`
	Status                 string `json:"status"`
	Progress               int    `json:"progress"`
	Output                 string `json:"output"`
	OutputKind             string `json:"output_kind"`
	Caption                string `json:"caption"`
	CreatedAt              string `json:"created_at"`
	FinishedAt             string `json:"finished_at"`
	KindLabel              string `json:"kind_label"`
	Label                  string `json:"label"`
	Class                  string `json:"class"`
	QCVerified             bool   `json:"qc_verified"`
	FailureReason          string `json:"failure_reason,omitempty"`
	SourceMusicTitle       string `json:"source_music_title,omitempty"`
	SourceMusicURL         string `json:"source_music_video_url,omitempty"`
	SourceMusicVideoID     string `json:"source_music_video_id,omitempty"`
	SourceMusicAuthor      string `json:"source_music_author,omitempty"`
	SourceCollectedAt      string `json:"source_music_collected_at,omitempty"`
	ProductID              int64  `json:"product_id,omitempty"`
	ProductName            string `json:"product_name,omitempty"`
	ProductShopID          string `json:"product_shop_id,omitempty"`
	ProductPageURL         string `json:"product_page_url,omitempty"`
	AffiliateLink          string `json:"affiliate_link,omitempty"`
	AffiliateAttributionID string `json:"affiliate_attribution_id,omitempty"`
	TikTokVideoID          string `json:"tiktok_video_id,omitempty"`
	TikTokVideoURL         string `json:"tiktok_video_url,omitempty"`
}

func kindLabel(kind string) string {
	switch kind {
	case studio.KindAffiliate:
		return "Affiliate"
	case "ai_content":
		return "Muse · Flow"
	default:
		return kind
	}
}

func (s *Server) studioJobViews(jobs []studio.Job) []studioJobView {
	out := make([]studioJobView, 0, len(jobs))
	for _, j := range jobs {
		if s.Studio != nil && (j.Kind == studio.KindAffiliate || j.Kind == studio.KindViral) &&
			(j.Params == "" || (j.Status == studio.StatusDone && j.Log == "")) {
			if full, ok := s.Studio.GetJob(j.ID); ok {
				if j.Params == "" {
					j.Params = full.Params
				}
				if j.Status == studio.StatusDone && j.Log == "" {
					j.Log = full.Log
				}
			}
		}
		failureReason := ""
		if j.Status == studio.StatusFailed {
			failureReason = studioFailureSummary(j.Log)
		}
		view := studioJobView{
			ID: j.ID, Kind: j.Kind, Title: j.Title, Status: j.Status,
			Progress: j.Progress, Output: mediaName(j.Output), OutputKind: studioOutputKind(j.Output),
			Caption:   j.Caption,
			CreatedAt: j.CreatedAt, FinishedAt: j.FinishedAt,
			KindLabel:     kindLabel(j.Kind),
			Label:         statusLabel(j.Status),
			Class:         statusClass(j.Status),
			QCVerified:    studioOutputQCVerified(j),
			FailureReason: failureReason,
		}
		if j.Kind == studio.KindAffiliate {
			var p studio.AffiliateParams
			if json.Unmarshal([]byte(j.Params), &p) == nil {
				view.ProductID = p.ProductID
				view.ProductName = strings.TrimSpace(p.ProductName)
				view.AffiliateLink = p.AffLink
				view.AffiliateAttributionID = p.AffiliateAttributionID
				view.SourceMusicTitle = p.SourceMusicTitle
				view.SourceMusicURL = p.SourceMusicVideoURL
				view.SourceMusicVideoID = p.SourceMusicVideoID
				view.SourceMusicAuthor = p.SourceMusicAuthor
				view.SourceCollectedAt = p.SourceMusicCollectedAt
				if p.ProductID > 0 && s.Products != nil {
					if product, err := s.Products.GetByID(p.ProductID); err == nil {
						view.ProductShopID = product.SourceID
						view.ProductPageURL = product.ProductPageURL
						if view.ProductName == "" {
							view.ProductName = product.Title
						}
					}
				}
			}
		} else if j.Kind == studio.KindViral {
			var p studio.ViralParams
			if json.Unmarshal([]byte(j.Params), &p) == nil {
				view.SourceMusicTitle = p.SourceMusicTitle
				view.SourceMusicURL = p.SourceVideoURL
				view.SourceMusicVideoID = p.SourceVideoID
				view.SourceMusicAuthor = p.SourceVideoAuthor
				view.SourceCollectedAt = p.SourceCollectedAt
			}
		}
		if s.Studio != nil && j.Status == studio.StatusDone && view.OutputKind == "video" &&
			(j.Kind == studio.KindAffiliate || j.Kind == studio.KindViral) {
			if publication, ok := s.Studio.TikTokPublicationForJob(j.ID); ok {
				view.TikTokVideoID = publication.PlatformVideoID
				view.TikTokVideoURL = publication.VideoURL
			}
		}
		out = append(out, view)
	}
	return out
}

func studioFailureSummary(log string) string {
	lines := strings.Split(strings.TrimSpace(log), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if marker := strings.Index(line, "LỖI"); marker >= 0 {
			line = strings.TrimSpace(strings.TrimPrefix(line[marker+len("LỖI"):], ":"))
			if line != "" {
				return truncateStudioFailure(line)
			}
		}
	}
	if len(lines) > 0 && lines[len(lines)-1] != "" {
		return truncateStudioFailure(lines[len(lines)-1])
	}
	return "Tác vụ dừng nhưng không lưu nguyên nhân."
}

func truncateStudioFailure(reason string) string {
	runes := []rune(strings.TrimSpace(reason))
	if len(runes) > 500 {
		return string(runes[:497]) + "..."
	}
	return string(runes)
}

func studioOutputKind(output string) string {
	switch strings.ToLower(filepath.Ext(output)) {
	case ".mp4", ".mov", ".webm":
		return "video"
	case ".png", ".jpg", ".jpeg", ".webp":
		return "image"
	case ".md", ".txt":
		return "text"
	default:
		return "file"
	}
}

// Keep render status separate from pipeline QC. Each supported producer needs
// evidence from its own QC stage; unknown and legacy outputs fail closed.
func studioOutputQCVerified(j studio.Job) bool {
	if j.Status != studio.StatusDone || strings.TrimSpace(j.Output) == "" {
		return false
	}
	switch j.Kind {
	case "ai_content":
		return false
	case studio.KindAffiliate, studio.KindViral:
	default:
		return false
	}
	qc, qcAt := pipelineMarkerState(j.Log, "VISUAL_QC_START:", "VISUAL_QC_PASS:", "VISUAL_QC_FAIL:")
	if qc != "pass" {
		return false
	}
	if j.Kind == studio.KindAffiliate {
		analysis, analysisAt := pipelineMarkerState(j.Log, "PRODUCT_ANALYSIS_START:", "PRODUCT_ANALYSIS_PASS:", "PRODUCT_ANALYSIS_FAIL:")
		director, directorAt := pipelineMarkerState(j.Log, "DIRECTOR_START:", "DIRECTOR_PASS:", "DIRECTOR_FAIL:")
		production, productionAt := pipelineMarkerState(j.Log, "PRODUCTION_START:", "PRODUCTION_PASS:", "PRODUCTION_FAIL:")
		return analysis == "pass" && director == "pass" && production == "pass" && analysisAt < directorAt && directorAt < productionAt && productionAt < qcAt
	} else {
		replacement, replacementAt := pipelineMarkerState(j.Log, "CHARACTER_REPLACEMENT_START:", "CHARACTER_REPLACEMENT_PASS:", "CHARACTER_REPLACEMENT_FAIL:")
		validation, validationAt := pipelineMarkerState(j.Log, "OUTPUT_VALIDATION_START:", "OUTPUT_VALIDATION_PASS:", "OUTPUT_VALIDATION_FAIL:")
		return replacement == "pass" && validation == "pass" && replacementAt < validationAt && validationAt < qcAt
	}
}

func pipelineMarkerState(log, startMarker, passMarker, failMarker string) (string, int) {
	startAt := strings.LastIndex(log, startMarker)
	passAt := strings.LastIndex(log, passMarker)
	failAt := strings.LastIndex(log, failMarker)
	if passAt >= 0 && passAt > failAt && passAt > startAt {
		return "pass", passAt
	}
	if failAt >= 0 && failAt > passAt && failAt > startAt {
		return "fail", failAt
	}
	if startAt >= 0 && startAt > passAt && startAt > failAt {
		return "running", startAt
	}
	return "unknown", -1
}

// mediaName trả tên file để phát qua /media/{name} (route chỉ nhận 1
// đoạn tên). j.Output là đường dẫn đầy đủ — dùng thẳng sẽ ra URL hỏng.
func mediaName(output string) string {
	if output == "" {
		return ""
	}
	return filepath.Base(output)
}
