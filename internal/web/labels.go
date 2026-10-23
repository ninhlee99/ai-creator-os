package web

import (
	_ "modernc.org/sqlite" // registers the "sqlite" driver (pure Go, no cgo)
	"regexp"
	"strings"
)

// statusLabel maps raw status slugs (accounts, slots, jobs, assets,
// products, decisions) to Vietnamese labels with diacritics. Unknown slugs
// fall back to a humanized form so nothing renders with underscores.
func statusLabel(s string) string {
	if l, ok := statusLabels[s]; ok {
		return l
	}
	if s == "" {
		return "—"
	}
	return strings.ReplaceAll(s, "_", " ")
}

func publisherPlatformLabel(platform string) string {
	switch platform {
	case "tiktok":
		return "TikTok"
	case "youtube":
		return "YouTube"
	case "facebook":
		return "Facebook"
	case "instagram":
		return "Instagram"
	case "threads":
		return "Threads"
	default:
		return statusLabel(platform)
	}
}

func growthVariantLabel(variant string) string {
	switch variant {
	case "tiktok":
		return "TikTok"
	case "youtube_shorts":
		return "YouTube Shorts"
	case "youtube_long":
		return "YouTube dài"
	default:
		return statusLabel(variant)
	}
}

// reasonLabel dịch lý do kỹ thuật trong feed hoạt động: "a_b -> c_d" →
// nhãn tiếng Việt hai đầu, "(missing-key)" → "(thiếu key)". Lý do khác
// giữ nguyên.
func reasonLabel(r string) string {
	r = strings.ReplaceAll(r, "(missing-key)", "(thiếu key)")
	// Bản ghi cũ (trước khi việt hóa) vẫn nằm trong sổ cái: dịch khi hiện.
	if m := legacyTopicRe.FindStringSubmatch(r); m != nil {
		return "đã chốt chủ đề (nguồn: " + sourceLabel(m[1]) + ")"
	}
	if m := legacyAssignRe.FindStringSubmatch(r); m != nil {
		return "gán tính cách " + m[1] + " cho chủ đề “" + m[2] + "”"
	}
	if strings.HasPrefix(r, "đã chốt chủ đề (nguồn: ") {
		src := strings.TrimSuffix(strings.TrimPrefix(r, "đã chốt chủ đề (nguồn: "), ")")
		return "đã chốt chủ đề (nguồn: " + sourceLabel(src) + ")"
	}
	if a, b, ok := strings.Cut(r, " -> "); ok {
		// Đổi nhà cung cấp AI sau lỗi quota/key.
		if pa, ok1 := providerLabels[a]; ok1 {
			bName, why, _ := strings.Cut(b, " ")
			if pb, ok2 := providerLabels[bName]; ok2 {
				out := pa + " → " + pb
				if why = strings.Trim(why, "()"); why != "" {
					out += " (lý do: " + why + ")"
				}
				return out
			}
		}
		if !strings.ContainsAny(a+b, " ") {
			return statusLabel(a) + " → " + statusLabel(b)
		}
	}
	return r
}

var (
	legacyTopicRe  = regexp.MustCompile(`^topic resolved \(source=(\w+)\)$`)
	legacyAssignRe = regexp.MustCompile(`^assigned (.+) for researched niche '(.*)'$`)
)

// providerLabels: tên nhà cung cấp AI dễ hiểu cho feed hoạt động.
var providerLabels = map[string]string{
	"gemini": "Gemini (Google)",
	"edge":   "giọng Edge miễn phí",
}

func sourceLabel(s string) string {
	switch s {
	case "fallback":
		return "mặc định theo tính cách, vì chưa có AI"
	case "hint":
		return "gợi ý của bạn"
	case "auto":
		return "AI tự chọn"
	}
	return s
}

var statusLabels = map[string]string{
	// account lifecycle
	"onboarding":       "Đang thiết lập",
	"researching":      "Đang nghiên cứu",
	"persona_assigned": "Đã gán persona",
	"growing":          "Đang tăng trưởng",
	"live_ready":       "Sẵn sàng",       // legacy: trạng thái sẵn sàng từ pipeline cũ (live đã park)
	"live":             "Live (đã park)", // legacy — giữ để không vỡ status cũ
	"paused":           "Đã tạm dừng",
	"penalized":        "Bị phạt",
	"retired":          "Đã nghỉ",
	// live slots
	"planned": "Đã lên lịch",
	"started": "Đang diễn ra",
	"skipped": "Đã bỏ qua",
	// jobs & assets
	"queued":     "Đang chờ",
	"pending":    "Đang chờ",
	"running":    "Đang chạy",
	"done":       "Hoàn thành",
	"publishing": "Đang gửi",
	"failed":     "Thất bại",
	// agent team node states
	"working":  "Đang làm",
	"exchange": "Đang nhận việc",
	"waiting":  "Đang chờ",
	"guard":    "Gác cổng",
	// shop products
	"shelf":  "Đang bán",
	"scaled": "Đang mở rộng",
	// decision actions shown in the activity feed
	"niche_research":               "Nghiên cứu niche",
	"account_transition":           "Chuyển giai đoạn",
	"auto_switch":                  "Công tắc tự động",
	"account_penalized":            "Kênh bị phạt",
	"advance_stage":                "Lên giai đoạn mới",
	"api_budget":                   "Ngân sách API",
	"api_key_add":                  "Thêm API key",
	"api_key_delete":               "Xóa API key",
	"api_switch":                   "Bật/tắt API",
	"at_automation_save":           "Lưu tự động Accesstrade",
	"auth_password":                "Đổi mật khẩu",
	"auto_pause_account":           "Tự tạm dừng kênh",
	"autopublish_failed":           "Tự đăng TikTok lỗi",
	"autopublish_meta_failed":      "Tự đăng Meta lỗi",
	"autopublish_meta_uncertain":   "Kết quả đăng Meta cần đối chiếu",
	"autopublish_yt_failed":        "Tự đăng YouTube lỗi",
	"story_publish_reconcile":      "Đối chiếu đăng truyện",
	"autopublish_yt_uncertain":     "Kết quả đăng YouTube cần đối chiếu",
	"autopublish_tiktok_uncertain": "Kết quả đăng TikTok cần đối chiếu",
	"avatar_character_add":         "Thêm nhân vật",
	"avatar_character_delete":      "Xóa nhân vật",
	"avatar_render_test":           "Thử dựng avatar",
	"avatar_sidecar_restart":       "Khởi động lại avatar",
	"avatar_tier_failover":         "Đổi nhà cung cấp avatar",
	"backup_create":                "Tạo bản sao lưu",
	"backup_offsite":               "Sao lưu ngoài máy",
	"backup_restore":               "Khôi phục sao lưu",
	"breakout_video":               "Video bứt phá",
	"build_schedule":               "Lập lịch",
	"campaign_pending":             "Chiến dịch chờ duyệt",
	"hunt_blocked":                 "Chưa săn được sản phẩm",
	"commission_reconcile":         "Đối soát hoa hồng",
	"daily_plan":                   "Kế hoạch ngày",
	"dedup_regenerate_angle":       "Đổi góc tránh trùng",
	"defer_quota":                  "Hoãn vì hết quota",
	"deny":                         "Từ chối",
	"distributed":                  "Đã phân phối",
	"double_down_format":           "Nhân đôi format tốt",
	"drop_stale_item":              "Bỏ mục quá hạn",
	"dry_run":                      "Chế độ chạy thử",
	"error":                        "Lỗi",
	"generate_plan":                "Tạo kế hoạch",
	"kill_format":                  "Dừng format kém",
	"kill_product":                 "Dừng sản phẩm",
	"kill_switch":                  "Dừng khẩn cấp",
	"login":                        "Đăng nhập",
	"mark_stalled":                 "Đánh dấu chững",
	"master_switch":                "Công tắc chính",
	"milestone_reached":            "Đạt mốc",
	"network_pause":                "Tạm dừng toàn mạng",
	"notify_save":                  "Lưu thông báo",
	"onboard":                      "Thiết lập ban đầu",
	"production_toggle":            "Bật/tắt sản xuất",
	"publish_youtube":              "Đăng YouTube",
	"publish_youtube_failed":       "Đăng YouTube thất bại",
	"viral_produce":                "Sản xuất video viral",
	"viral_auto_toggle":            "Bật/tắt tự động viral",
	"harness_cycle":                "Chu kỳ nhận thức OODA",
	"harness_autonomy":             "Đổi cấp độ tự hành",
	"harness_memory":               "Ghi nhớ kinh nghiệm",
	"scale_product":                "Mở rộng sản phẩm",
	"seed":                         "Khởi tạo",
	"session_topic":                "Chủ đề phiên",
	"shelf_product":                "Lên kệ sản phẩm",
	"skip":                         "Bỏ qua",
	"skip_duplicate_youtube":       "Bỏ video trùng",
	"slot_skipped":                 "Bỏ lượt",
	"thresholds_reset":             "Đặt lại ngưỡng",
	"thresholds_updated":           "Cập nhật ngưỡng",
	"tier_failover":                "Đổi nhà cung cấp AI",
	"live_unlocked":                "Mở khóa live",
	// growth stages (CHANNEL_GROWTH)
	"cold_start":        "Khởi động (canary)",
	"format_testing":    "Đang test format",
	"scaling":           "Đang mở rộng",
	"monetization_push": "Đẩy tới mốc kiếm tiền",
	"monetized":         "Đã qua cửa kiếm tiền",
	"stalled":           "Đang chững (chờ replan)",
	// growth format verdicts + plan item statuses
	"testing":    "Đang test",
	"winner":     "Format thắng",
	"killed":     "Đã dừng format",
	"producing":  "Đang sản xuất",
	"published":  "Đã đăng",
	"dropped":    "Đã loại",
	"active":     "Đang hiệu lực",
	"superseded": "Đã thay thế",
	// growth phase-2 production/publish states
	"produced":        "Đã sản xuất",
	"qc":              "Đang kiểm tra chất lượng",
	"waiting_connect": "Chờ kết nối YouTube",
	"waiting_quota":   "Chờ quota YouTube",
	"needs_review":    "Cần đối chiếu kết quả đăng",
	// Growth alert kinds.
	"breakout":       "Video bứt phá",
	"dedup":          "Chống trùng nội dung",
	"disk_guard":     "Dung lượng ổ đĩa",
	"format":         "Hiệu quả format",
	"milestone":      "Mốc tăng trưởng",
	"penalty_signal": "Tín hiệu vi phạm",
	"plan":           "Kế hoạch nội dung",
	"quota":          "Quota YouTube",
	"token_expired":  "Token hết hạn",
}

// statusClass maps a raw status slug to a semantic badge class suffix
// (ok/warn/err/info/no/running/queued/draft) defined in style.css.
func statusClass(s string) string {
	switch s {
	case "live", "live_ready", "done", "started", "shelf", "scaled", "monetized", "winner", "published", "active":
		return "ok"
	case "running", "working", "producing", "publishing":
		return "running"
	case "queued", "pending":
		return "queued"
	case "failed", "penalized", "killed":
		return "err"
	case "onboarding", "paused", "skipped", "cold_start", "stalled":
		return "warn"
	case "researching", "persona_assigned", "growing", "planned", "exchange", "format_testing", "monetization_push", "scaling", "testing", "produced", "qc":
		return "info"
	case "waiting_connect", "waiting_quota", "needs_review":
		return "warn"
	case "dropped", "superseded":
		return "no"
	default:
		return "no"
	}
}
