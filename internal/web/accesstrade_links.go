package web

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/ninhlee99/ai-creator-os/internal/accesstrade"
	"github.com/ninhlee99/ai-creator-os/internal/automation"
	"github.com/ninhlee99/ai-creator-os/internal/products"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type linkCreateJSON struct {
	ProductURL string `json:"product_url"`
	CampaignID string `json:"campaign_id"`
	AccountRef string `json:"account_ref"`
	UTMSource  string `json:"utm_source"`
	UTMMedium  string `json:"utm_medium"`
	Sub1       string `json:"sub1"`
	Sub2       string `json:"sub2"`
	Sub3       string `json:"sub3"`
	Sub4       string `json:"sub4"`
}

// handleATCreateLink tạo tracking link qua API, lưu store, trả JSON.
// Chỉ campaign đã duyệt (approval=successful) mới được tạo link.
func (s *Server) handleATCreateLink(w http.ResponseWriter, r *http.Request) {
	cl, err := s.atClient()
	if err != nil {
		writeJSONErr(w, err.Error(), http.StatusPreconditionFailed)
		return
	}
	var in linkCreateJSON
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&in); err != nil {
		writeJSONErr(w, "JSON không hợp lệ.", http.StatusBadRequest)
		return
	}
	in.ProductURL = strings.TrimSpace(in.ProductURL)
	in.CampaignID = strings.TrimSpace(in.CampaignID)
	if in.ProductURL == "" {
		writeJSONErr(w, "Thiếu URL sản phẩm.", http.StatusBadRequest)
		return
	}
	if in.CampaignID == "" {
		writeJSONErr(w, "Thiếu chiến dịch.", http.StatusBadRequest)
		return
	}
	// Chặn tạo link cho campaign chưa duyệt (dựa trên cache đã sync).
	if err := s.atRequireApproved(in.CampaignID); err != nil {
		writeJSONErr(w, err.Error(), http.StatusPreconditionFailed)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), atTimeout)
	defer cancel()
	link, err := cl.CreateProductLink(ctx, accesstrade.LinkRequest{
		URL:        in.ProductURL,
		CampaignID: in.CampaignID,
		UTM:        map[string]string{"source": in.UTMSource, "medium": in.UTMMedium},
		Sub1:       in.Sub1, Sub2: in.Sub2, Sub3: in.Sub3, Sub4: in.Sub4,
	})
	if err != nil {
		writeJSONErr(w, "Tạo link thất bại: "+err.Error(), http.StatusBadGateway)
		return
	}
	if err := s.AT.SaveLink(in.ProductURL, in.CampaignID,
		strings.TrimSpace(in.AccountRef), link.TrackingLink, link.ShortLink); err != nil {
		writeJSONErr(w, "Tạo link OK nhưng lưu thất bại: "+err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]any{
		"ok": true, "tracking_link": link.TrackingLink, "short_link": link.ShortLink,
	})
}

// atRequireApproved chặn tạo link khi campaign chưa ở trạng thái
// "successful" trong cache. Cache trống → yêu cầu sync trước (không đoán).
func (s *Server) atRequireApproved(campaignID string) error {
	cs, err := s.AT.CachedCampaigns()
	if err != nil {
		return err
	}
	if len(cs) == 0 {
		return atFail("chưa có dữ liệu chiến dịch — bấm “Tải chiến dịch” trước")
	}
	for _, c := range cs {
		if c.ID == campaignID {
			if c.Approval == "successful" {
				return nil
			}
			return atFail("chiến dịch “" + c.Name + "” chưa duyệt (" + c.ApprovalLabel() + ") — không tạo được link")
		}
	}
	return atFail("không tìm thấy chiến dịch trong cache — bấm “Tải chiến dịch” để làm mới")
}

func itoa(n int) string { return strconv.Itoa(n) }

// ------------------------------------------------------------------ Đợt C

// handleATHunt chạy hunter tay (AJAX): quét datafeed → kho sản phẩm →
// top N mới tạo video affiliate. Trả JSON cho toast.
func (s *Server) handleATHunt(w http.ResponseWriter, r *http.Request) {
	cl, err := s.atClient()
	if err != nil {
		writeJSONErr(w, err.Error(), http.StatusPreconditionFailed)
		return
	}
	if s.Products == nil {
		writeJSONErr(w, "kho sản phẩm chưa sẵn sàng", http.StatusPreconditionFailed)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Minute)
	defer cancel()
	res, err := cl.HuntTikTokShop(ctx, s.Products, accesstrade.HunterFilter{MaxAgeDays: 180})
	if err != nil {
		writeJSONErr(w, "Săn sản phẩm thất bại: "+err.Error(), http.StatusBadGateway)
		return
	}
	// Tạo video cho top N sản phẩm mới (nối hunter → studio, như tick).
	videos := 0
	want := s.atSettingInt(automation.KeyATHunterVideos, 3)
	auto := s.automation()
	auto.StudioRunner = s.Studio
	accountID, accountErr := auto.HunterAccountID()
	productionNote := ""
	if len(res.ProductionCandidates) == 0 {
		if res.PendingReview > 0 {
			productionNote = fmt.Sprintf("Chưa tạo video: %d sản phẩm mới đang chờ bạn duyệt trong trang Sản phẩm.", res.PendingReview)
		} else {
			productionNote = "Chưa tạo video: cần sản phẩm đã duyệt, đủ giá/hoa hồng và có link sản phẩm hợp lệ."
		}
		want = 0
	}
	if want > 0 && !s.atSettingOn(automation.KeyATHunterFlowCredits, false) {
		productionNote = "Chưa bật quyền dùng Muse AI credits cho video hunter."
		want = 0
	}
	if s.Studio != nil && want > 0 {
		ready, reason := s.Studio.AutoAffiliateProductionReady()
		if !ready {
			productionNote = reason
		} else if accountErr != nil {
			productionNote = "Chưa tạo video: " + accountErr.Error()
		} else {
			for _, p := range res.ProductionCandidates {
				if videos >= want {
					break
				}
				claimed, cerr := s.Products.RecordUse(p.ID, accountID, "")
				if cerr != nil || !claimed {
					continue
				}
				jobID, jerr := s.makeHunterVideo(ctx, p, accountID)
				if jerr != nil {
					_ = s.Products.ReleaseUseClaim(p.ID, accountID)
					log.Printf("web: at hunt video %q: %v", p.Title, jerr)
					continue
				}
				_ = s.Products.SetUsageJobID(p.ID, accountID, jobID)
				videos++
			}
		}
	}
	if s.Ledger != nil {
		_ = s.Ledger.SetSetting("at.hunter_last_run", time.Now().Format(time.RFC3339))
	}
	writeJSON(w, map[string]any{
		"ok": true, "new": res.New, "updated": res.Updated,
		"skipped": res.Skipped, "watchlist": res.Watchlist, "pending_review": res.PendingReview,
		"production_candidates": len(res.ProductionCandidates),
		"videos":                videos, "note": res.Note, "production_note": productionNote,
	})
}

// makeHunterVideo tạo 1 studio job affiliate từ sản phẩm hunter (bản web
// của automation tick — tải ảnh listing + gọi CreateAffiliateJob).
func (s *Server) makeHunterVideo(ctx context.Context, p products.Product, accountID int64) (string, error) {
	auto := s.automation()
	auto.StudioRunner = s.Studio
	return auto.MakeHunterVideoForAccount(ctx, p, accountID)
}

// handleATOrderSync đồng bộ đơn tay (AJAX): order-list → upsert → JSON
// số liệu đối soát cho toast + cập nhật card.
