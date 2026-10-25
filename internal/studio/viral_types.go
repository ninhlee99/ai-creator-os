package studio

// KindViral là job thay nhận diện nhân vật trên video nguồn đã chọn.
const KindViral = "viral"

// ViralParams chứa cấu hình đầu vào để tạo 1 video viral.
type ViralParams struct {
	Topic                   string                 `json:"topic"`
	Genre                   string                 `json:"genre"`       // mystery, lifehack, trendjack, tech_fact, koc_review
	VisualMood              string                 `json:"visual_mood"` // cinematic, vibrant_3d, realistic, cyber_neon
	Voice                   string                 `json:"voice"`       // TTS provider voice identifier
	Account                 string                 `json:"account"`     // username kênh để đọc Brand DNA
	AccountID               int64                  `json:"account_id,omitempty"`
	ModelPhoto              string                 `json:"model_photo,omitempty"`       // local file path to character face reference image
	ModelPhotos             []string               `json:"model_photos,omitempty"`      // multi-angle face reference images for character locking
	TargetDuration          float64                `json:"target_duration"`             // exact source duration in seconds
	OutputResolution        string                 `json:"output_resolution,omitempty"` // 4k | 8k
	FlowCreditsAcknowledged bool                   `json:"flow_credits_acknowledged,omitempty"`
	SourceVideoURL          string                 `json:"source_video_url,omitempty"`
	SourceVideoID           string                 `json:"source_video_id,omitempty"`
	SourceVideoAuthor       string                 `json:"source_video_author,omitempty"`
	SourceMusicTitle        string                 `json:"source_music_title,omitempty"`
	SourceCollectedAt       string                 `json:"source_collected_at,omitempty"`
	SourcePerformance       ViralSourcePerformance `json:"source_performance,omitempty"`
	SampleVideo             string                 `json:"sample_video,omitempty"` // local file path to sample motion video
	AIMode                  string                 `json:"ai_mode,omitempty"`      // full_character_edit replaces the complete person in source footage
	ReferenceSummary        string                 `json:"reference_summary,omitempty"`
	ReferenceBeats          []ViralReferenceBeat   `json:"reference_beats,omitempty"`
}

// ViralReferenceBeat is one observed beat from sampled source frames.
type ViralReferenceBeat struct {
	TimeSec        float64 `json:"time_sec"`
	Framing        string  `json:"framing"`
	SubjectAction  string  `json:"subject_action"`
	BodyMovement   string  `json:"body_movement"`
	CameraMovement string  `json:"camera_movement"`
	EditTransition string  `json:"edit_transition"`
	Energy         string  `json:"energy"`
	Uncertainty    string  `json:"uncertainty,omitempty"`
}

// ViralScene mô tả 1 phân cảnh trong kịch bản video viral.
type ViralScene struct {
	Index          int     `json:"index"`
	Phase          string  `json:"phase"`        // source_beat
	VoiceText      string  `json:"voice_text"`   // Lời thoại lôi cuốn (tiếng Việt)
	ImagePrompt    string  `json:"image_prompt"` // Prompt tiếng Anh cho AI MediaGen vẽ ảnh 9:16
	VideoPrompt    string  `json:"video_prompt"` // Chuyển động tái hiện hành động/camera của nguồn
	VisualHook     string  `json:"visual_hook"`  // Text giật tít hiển thị trên màn hình
	DurationSec    float64 `json:"duration_sec"`
	SourceStartSec float64 `json:"source_start_sec"`
	SourceEndSec   float64 `json:"source_end_sec"`
}

// ViralScript là kịch bản hoàn chỉnh của một video viral.
type ViralScript struct {
	Title             string                 `json:"title"`
	Hook3s            string                 `json:"hook_3s"`
	CoreAngle         string                 `json:"core_angle"`
	Hashtags          []string               `json:"hashtags"`
	CallToAction      string                 `json:"call_to_action"`
	Scenes            []ViralScene           `json:"scenes"`
	SourcePerformance ViralSourcePerformance `json:"source_performance"`
}

// ViralSourcePerformance carries measured Radar counters into the analysis UI.
// Missing counters remain explicitly unknown; these metrics do not prove why a
// video became popular.
type ViralSourcePerformance struct {
	Views           int64   `json:"views"`
	ViewsKnown      bool    `json:"views_known"`
	Likes           int64   `json:"likes"`
	LikesKnown      bool    `json:"likes_known"`
	Comments        int64   `json:"comments"`
	CommentsKnown   bool    `json:"comments_known"`
	Shares          int64   `json:"shares"`
	SharesKnown     bool    `json:"shares_known"`
	ViewsPerHour    float64 `json:"views_per_hour"`
	VelocityKnown   bool    `json:"velocity_known"`
	EngagementRate  float64 `json:"engagement_rate"`
	EngagementKnown bool    `json:"engagement_known"`
}

type viralEditSegment struct {
	Path     string
	Start    float64
	Duration float64
}

// ViralOriginalProfile lưu trữ đặc tính kỹ thuật & phong cách của video viral gốc để phản chiếu sang video AI.
type ViralOriginalProfile struct {
	Path           string  `json:"path"`
	Duration       float64 `json:"duration"`
	Width          int     `json:"width"`
	Height         int     `json:"height"`
	HasAudio       bool    `json:"has_audio"`
	AudioExtracted string  `json:"audio_extracted,omitempty"`
	MotionVibe     string  `json:"motion_vibe"` // "energetic_beat", "smooth_sensual", "dynamic_street", "natural_lookbook"
	VisualMood     string  `json:"visual_mood"` // "neon_aesthetic", "warm_studio", "natural_daylight", "cinematic_dramatic"
	PacingSec      float64 `json:"pacing_sec"`
}
