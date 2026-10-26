package studio

import (
	"context"
	"database/sql"
	"os"
	"strings"
	"sync"

	"github.com/ninhlee99/ai-creator-os/internal/webai"
)

// LLM is the text-generation backend for the director. It matches
// engines.LLMProvider so the chain can be passed directly.
type LLM interface {
	Complete(ctx context.Context, system, prompt string) (string, error)
	Name() string
	Healthy(ctx context.Context) bool
}

// Narrator synthesizes narration WAV bytes (TTS chain), or nil for silent.
type Narrator func(ctx context.Context, text, voice string) ([]byte, error)

// Job kinds.
const (
	KindAffiliate = "affiliate"
)

// Job statuses.
const (
	StatusQueued  = "queued"
	StatusRunning = "running"
	StatusDone    = "done"
	StatusFailed  = "failed"
)

// AffiliateModePhoto = legacy image montage; AffiliateModeShots = live
// motion clips. ViralModeFullCharacterEdit requires full-person video editing.
const (
	AffiliateModePhoto             = "photo"
	AffiliateModeShots             = "shots"
	AffiliateVideoEngineGoogleFlow = "googleflow"
	AffiliateVideoEngineManual     = "manual"
	VideoOutput4K                  = "4k"
	VideoOutput8K                  = "8k"
	ViralModeFullCharacterEdit     = "full_character_edit"
)

// AffiliateParams describes one affiliate-video job.
type ChannelCreativeProfile struct {
	Model       string   `json:"model,omitempty"`
	Positioning string   `json:"positioning,omitempty"`
	Audience    string   `json:"audience,omitempty"`
	Tone        string   `json:"tone,omitempty"`
	VisualStyle string   `json:"visual_style,omitempty"`
	RedLines    []string `json:"red_lines,omitempty"`
}

type AffiliateParams struct {
	Mode             string   `json:"mode"`                 // photo | shots
	AccountID        int64    `json:"account_id,omitempty"` // set by autopilot
	ProductID        int64    `json:"product_id,omitempty"` // approved catalog offer, when selected
	ChannelMemoryKey string   `json:"channel_memory_key,omitempty"`
	Niche            string   `json:"niche"`
	ProductName      string   `json:"product_name"`
	ProductPrice     string   `json:"product_price,omitempty"`
	ProductCategory  string   `json:"product_category,omitempty"` // apparel | footwear | bag | jewelry | cosmetics | home | tech | food | other
	ProductUsage     string   `json:"product_usage,omitempty"`    // optional exact use action supplied by the operator
	ProductFocus     string   `json:"product_focus,omitempty"`    // optional visible detail to emphasize; never a claim
	ModelPhoto       string   `json:"model_photo"`                // local path (identity lock)
	ModelPhotos      []string `json:"model_photos,omitempty"`     // multi-angle local paths (identity lock)
	ProductPhoto     string   `json:"product_photo"`              // local path (product lock)
	// ExtraPhotos: ảnh thêm (góc khác, chi tiết, ảnh listing) — chế độ
	// không key dựng từ cả bộ để video không lặp 1 ảnh.
	ExtraPhotos []string `json:"extra_photos,omitempty"`
	// FixedCaption: chú thích người dùng tự viết (Chỉnh sửa & dựng lại) —
	// có thì dùng nguyên văn, không gọi AI/mẫu.
	FixedCaption             string `json:"fixed_caption,omitempty"`
	Seconds                  int    `json:"seconds"`
	SourceMusicTitle         string `json:"source_music_title,omitempty"`
	SourceMusicVideoURL      string `json:"source_music_video_url,omitempty"`
	SourceMusicVideoID       string `json:"source_music_video_id,omitempty"`
	SourceMusicAuthor        string `json:"source_music_author,omitempty"`
	SourceMusicCollectedAt   string `json:"source_music_collected_at,omitempty"`
	VideoEngine              string `json:"video_engine,omitempty"` // googleflow | manual
	FlowCreditsAcknowledged  bool   `json:"flow_credits_acknowledged,omitempty"`
	OutputResolution         string `json:"output_resolution,omitempty"` // 4k | 8k
	AutoGenerateProductPhoto bool   `json:"auto_generate_product_photo,omitempty"`
	// AffLink: link affiliate Accesstrade của sản phẩm (đợt Q). "" = không
	// có — video vẫn đăng được, chỉ thiếu link kiếm tiền trong mô tả.
	AffLink                string `json:"aff_link,omitempty"`
	AffiliateAttributionID string `json:"affiliate_attribution_id,omitempty"`
	// Clips are optional in Google Flow mode; manual mode needs one real clip
	// for every approved shot.
	Clips []string `json:"clips,omitempty"`
	// PreparedPlanID binds a web-created shot plan to the exact job inputs.
	// The worker receives the saved profile and shots after the plan is consumed.
	PreparedPlanID       string                   `json:"prepared_plan_id,omitempty"`
	PreparedProductBrief *AffiliateProductProfile `json:"prepared_product_brief,omitempty"`
	PreparedShots        []ProShot                `json:"prepared_shots,omitempty"`
	ChannelProfile       ChannelCreativeProfile   `json:"channel_profile,omitempty"`
	// VoiceoverPath: file âm thanh lồng tiếng (được tạo hoặc tải lên)
	VoiceoverPath string `json:"voiceover_path,omitempty"`
	VoiceoverText string `json:"voiceover_text,omitempty"`
	// AspectRatio: tỉ lệ khung hình ("9:16" hoặc "16:9")
	AspectRatio string `json:"aspect_ratio,omitempty"`
	// AssignedDepts: danh sách phòng ban được phân bổ cho job (DEP-01, DEP-02,...)
	AssignedDepts []string `json:"assigned_depts,omitempty"`
}

// HasClips kiểm tra xem job có video clip sẵn (từ Muse AI / video gen) hay không.
func (p AffiliateParams) HasClips() bool {
	for _, c := range p.Clips {
		if strings.TrimSpace(c) != "" {
			return true
		}
	}
	return false
}

// IsMasterQuality4K checks whether 4K mastering is globally enabled or requested.
func IsMasterQuality4K() bool {
	return os.Getenv("MASTER_QUALITY_4K") == "1"
}

// AspectDims4K maps a job aspect to full 4K UHD output dimensions (2160x3840 for 9:16, 3840x2160 for 16:9).
func AspectDims4K(aspect string) (w, h int) {
	if aspect == "16:9" {
		return 3840, 2160
	}
	if aspect == "1:1" {
		return 3840, 3840
	}
	return 2160, 3840
}

// AspectDims8K returns 8K masters for vertical and horizontal video.
func AspectDims8K(aspect string) (w, h int) {
	if aspect == "16:9" {
		return 7680, 4320
	}
	if aspect == "1:1" {
		return 7680, 7680
	}
	return 4320, 7680
}

// AspectDims maps a job aspect to ffmpeg output dimensions. Unknown or
// empty aspects fall back to 9:16 so old jobs/params keep rendering.
// When MASTER_QUALITY_4K=1, delivery renders output full 4K UHD (2160x3840 / 3840x2160).
func AspectDims(aspect string) (w, h int) {
	if os.Getenv("MASTER_QUALITY_4K") == "1" {
		return AspectDims4K(aspect)
	}
	if aspect == "16:9" {
		return 1920, 1080
	}
	return 1080, 1920
}

// Job is one studio render job.
type Job struct {
	ID         string `json:"id"`
	Kind       string `json:"kind"`
	Title      string `json:"title"`
	Status     string `json:"status"`
	Progress   int    `json:"progress"` // 0-100
	Log        string `json:"log"`
	Output     string `json:"output"` // final file, "" until done
	Caption    string `json:"caption"`
	Params     string `json:"params"`
	CreatedAt  string `json:"created_at"`
	FinishedAt string `json:"finished_at"`
}

// Asset is one generated photo/clip inside a job (the storyboard row).
type Asset struct {
	ID     int64  `json:"id"`
	JobID  string `json:"job_id"`
	Idx    int    `json:"idx"`
	Kind   string `json:"kind"` // photo | clip | portrait
	Path   string `json:"path"`
	Prompt string `json:"prompt"`
	Status string `json:"status"` // queued|running|done|failed
	// Method is how this asset was really rendered: "veo", "cinematic"
	// (điện ảnh từ ảnh, chế độ chính Wave 3), "anh-tts" (keyframe + TTS
	// fallback, legacy), "trailer". "" = unknown/legacy.
	Method string `json:"method"`
}

// Studio orchestrates creation jobs: director -> shoot -> assemble ->
// review. Jobs run in background goroutines; state lives in SQLite so the
// dashboard can poll progress.
type Studio struct {
	db       *sql.DB
	mu       sync.Mutex
	llm      LLM
	mg       MediaGen
	narrator Narrator
	outDir   string
	workRoot string
	running  map[string]context.CancelFunc
	onDone   OnDoneFunc
	// BudgetUSD returns the API spend ceiling in USD (0/nil = no ceiling).
	// Wired by the app from the automation settings facade (R2-W4 knob).
	BudgetUSD func() float64
	// QC checks rendered shots against identity drift. Default is the
	// honest no-op (see qc.go) — the storyboard UI is the manual QC loop.
	QC ShotQCChecker
	// jobSem caps concurrent renders (see jobSlot); lazily built because
	// tests also construct Studio as a struct literal.
	jobSem chan struct{}
	// DiskGuard, nếu khác nil, trả về % đĩa đã dùng của workRoot (Đợt V).
	// insertJob từ chối job mới khi vượt MaxDiskPct (mặc định 95).
	DiskGuard  func() (int, error)
	MaxDiskPct int
	// PromptEnhancer supplies channel-scoped lessons to compatible creative prompts.
	PromptEnhancer func(ctx context.Context, niche, channel string) string
	// webAI connects channel production to selected, email-keyed provider accounts.
	webAI             WebAIResolver
	webAIVideo        WebAIVideoRenderer
	webAIAvailability WebAIAvailability
	// FlowStore manages visual node graphs (ComfyUI / n8n workflows) and execution runs.
	flowStore any
}

// WebAISession chứa thông tin phiên đăng nhập web (cookie, session token, email).
type WebAISession struct {
	AccountID    string
	Platform     string
	SessionToken string
	CookieHeader string
	CookieJar    []webai.BrowserCookie
	Email        string
}

// WebAIResolver là hàm truy vấn tài khoản Web AI đang kết nối theo nền tảng.
type WebAIResolver func(platform string) (*WebAISession, error)

// WebAIVideoRequest describes a reference-driven Flow operation. The adapter
// must not silently drop image or source-video inputs.
type WebAIVideoRequest struct {
	Mode        string // ingredients | video_edit
	Prompt      string
	AspectRatio string
	ImagePaths  []string
	VideoPath   string
	ProjectID   string
}

// WebAIVideoRenderer writes a verified downloaded MP4 to outputPath and
// returns the provider project ID for reuse across shots in one job.
type WebAIVideoRenderer func(context.Context, *WebAISession, WebAIVideoRequest, string, func(string)) (string, error)

type WebAIAvailability func(platform string) (bool, string)

// SetWebAIResolver cấu hình hàm truy vấn tài khoản Web AI cho Studio.
