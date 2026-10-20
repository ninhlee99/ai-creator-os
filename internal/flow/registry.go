package flow

import (
	"fmt"
	"sync"
)

// Registry manages all registered Node definitions in the system.
type Registry struct {
	mu    sync.RWMutex
	nodes map[string]NodeDef
}

var globalRegistry = NewRegistry()

// GlobalRegistry returns the singleton registry.
func GlobalRegistry() *Registry {
	return globalRegistry
}

// NewRegistry initializes an empty Node registry.
func NewRegistry() *Registry {
	return &Registry{
		nodes: make(map[string]NodeDef),
	}
}

// Register adds a NodeDef to the registry.
func (r *Registry) Register(def NodeDef) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.nodes[def.Type] = def
}

// Get retrieves a NodeDef by its type name.
func (r *Registry) Get(nodeType string) (NodeDef, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	def, ok := r.nodes[nodeType]
	return def, ok
}

// List returns all registered NodeDefs.
func (r *Registry) List() []NodeDef {
	r.mu.RLock()
	defer r.mu.RUnlock()
	list := make([]NodeDef, 0, len(r.nodes))
	for _, def := range r.nodes {
		list = append(list, def)
	}
	return list
}

// CreateNode instantiates a new Node from a registered NodeDef.
func (r *Registry) CreateNode(id, nodeType, label string) (*Node, error) {
	def, ok := r.Get(nodeType)
	if !ok {
		return nil, fmt.Errorf("node type '%s' chưa được đăng ký trong registry", nodeType)
	}

	if label == "" {
		label = def.Label
	}

	params := make(map[string]interface{})
	for k, v := range def.DefaultData {
		params[k] = v
	}

	inputs := make([]Pin, len(def.Inputs))
	copy(inputs, def.Inputs)

	outputs := make([]Pin, len(def.Outputs))
	copy(outputs, def.Outputs)

	return &Node{
		ID:           id,
		Type:         nodeType,
		Label:        label,
		Category:     def.Category,
		Inputs:       inputs,
		Outputs:      outputs,
		Params:       params,
		Status:       StatusPending,
		OutputValues: make(map[string]interface{}),
	}, nil
}

// Standard Node Types registered across the system.
const (
	// Input Nodes
	NodeTikTokTrend       = "input.tiktok_trend"
	NodeProductOutfit     = "input.product_outfit"
	NodeCharacterFace     = "input.character_face"
	NodeStoryScript       = "input.story_script"

	// Web Adapter Nodes (100% Cloud / Web AI - Zero Local Model Weights)
	NodeWebImageGen       = "adapter.web_image_gen"
	NodeWebVirtualTryOn   = "adapter.web_virtual_tryon"
	NodeWebMotionTransfer = "adapter.web_motion_transfer"
	NodeWebVideoGen       = "adapter.web_video_gen"
	NodeWebStoryDecompose = "adapter.web_story_decomposer"
	NodeWebFaceSwap       = "adapter.web_faceswap"
	NodeWebTTS            = "adapter.web_tts"
	NodeGeminiTTS         = "adapter.gemini_tts"
	NodeInsightFaceSwap   = NodeWebFaceSwap // backwards-compatible alias
	NodeVieNeuTTS         = NodeGeminiTTS   // backwards-compatible alias

	// Local Media Assembly Nodes (FFmpeg Utilities - No AI Models, Zero Memory Overhead)
	NodeAcousticFoleyBed  = "local.acoustic_foley_bed"
	NodeAntiDistortion    = "local.antidistortion_crop"
	NodeVideoRemaster     = "local.video_remaster"
	NodeCapCutBeatMux     = "local.capcut_beat_mux"

	// QC & Output Nodes
	NodeSoulIndexQC       = "qc.soul_index"
	NodeExportTarget      = "output.export_target"

	// Chinese AI Cinema Micro-Drama Nodes
	NodeCinemaScript      = "input.cinema_script"
	NodeActorBank         = "input.actor_bank"
	NodeCinematicCamera   = "local.cinematic_camera"
	NodeWebKlingVideo     = "adapter.web_kling_video"
	NodeWebLipSync        = "adapter.web_lip_sync"
	NodeAudioDuckingFoley = "local.audio_ducking_foley"
	NodeColorGradingLUT   = "local.color_grading_lut"
	NodeCinemaMaster      = "output.cinema_master"
)

func init() {
	reg := GlobalRegistry()

	// 1. TikTok Trend Input
	reg.Register(NodeDef{
		Type:        NodeTikTokTrend,
		Label:       "TikTok / Douyin Trend Radar",
		Category:    "input",
		Description: "Bắt video xu hướng vũ đạo từ TikTok VN hoặc Douyin CN với dữ liệu 100% thời gian thực.",
		Inputs: []Pin{
			{ID: "url", Name: "Video URL / Keyword", Type: TypeString, Required: false},
		},
		Outputs: []Pin{
			{ID: "video_url", Name: "Source Video URL", Type: TypeVideo},
			{ID: "audio_url", Name: "Trending Audio URL", Type: TypeAudio},
			{ID: "music_title", Name: "Music Title", Type: TypeString},
			{ID: "bpm", Name: "BPM / Beat Drop", Type: TypeNumber},
		},
		DefaultData: map[string]interface{}{
			"region": "VN",
			"limit":  1,
		},
	})

	// 2. Product Outfit Input
	reg.Register(NodeDef{
		Type:        NodeProductOutfit,
		Label:       "Sản Phẩm & Trang Phục (Accesstrade / Ecom)",
		Category:    "input",
		Description: "Tải trang phục, váy áo hoặc set đồ từ link Accesstrade hoặc Shopee/Lazada.",
		Inputs: []Pin{
			{ID: "product_url", Name: "Product URL", Type: TypeString, Required: false},
		},
		Outputs: []Pin{
			{ID: "outfit_image", Name: "Outfit Garment Image", Type: TypeImage},
			{ID: "product_title", Name: "Product Title", Type: TypeString},
			{ID: "category", Name: "Clothing Category", Type: TypeString},
		},
		DefaultData: map[string]interface{}{
			"category": "vay_nu",
		},
	})

	// 3. Character Face Identity Input
	reg.Register(NodeDef{
		Type:        NodeCharacterFace,
		Label:       "Nhân Vật Kênh (Creator Face ID)",
		Category:    "input",
		Description: "Nạp ảnh gốc khuôn mặt creator để trích xuất Identity Embedding khóa mặt toàn bộ video.",
		Inputs: []Pin{
			{ID: "profile_id", Name: "Profile ID / Channel", Type: TypeString, Required: false},
		},
		Outputs: []Pin{
			{ID: "face_image", Name: "Face Portrait Image", Type: TypeImage},
			{ID: "face_latent", Name: "Face Latent Embedding (512-d)", Type: TypeFace},
		},
		DefaultData: map[string]interface{}{
			"channel": "default",
		},
	})

	// 4. Story Script Input
	reg.Register(NodeDef{
		Type:        NodeStoryScript,
		Label:       "Kịch Bản Kể Chuyện Audio (Story Script)",
		Category:    "input",
		Description: "Nạp đề tài, thể loại (kinh dị, tiên hiệp, trinh thám) và văn bản cốt truyện.",
		Inputs: []Pin{
			{ID: "topic", Name: "Chủ Đề", Type: TypeString, Required: true},
			{ID: "genre", Name: "Thể Loại", Type: TypeString, Required: true},
			{ID: "duration_min", Name: "Thời Lượng (Phút)", Type: TypeNumber, Required: false},
		},
		Outputs: []Pin{
			{ID: "story_title", Name: "Tiêu Đề Truyện", Type: TypeString},
			{ID: "raw_text", Name: "Toàn Bộ Lời Kể", Type: TypeString},
			{ID: "scenes_json", Name: "Cấu Trúc Phân Đoạn", Type: TypeJSON},
		},
		DefaultData: map[string]interface{}{
			"genre":        "kinh_di",
			"duration_min": 15,
		},
	})

	// 5. Web Virtual Try-On Adapter (Gemini / Claude Web Prompting - 0đ)
	reg.Register(NodeDef{
		Type:        NodeWebVirtualTryOn,
		Label:       "Web Adapter: Phối Đồ Tự Nhiên (Gemini / Claude Web)",
		Category:    "adapter",
		Description: "Phối đồ thông minh lên dáng người mẫu của bạn bằng Gemini / Claude Web Adapter (100% Miễn Phí, không dùng API trả phí).",
		Inputs: []Pin{
			{ID: "model_image", Name: "Human Model Image", Type: TypeImage, Required: true},
			{ID: "outfit_image", Name: "Garment Clothes Image", Type: TypeImage, Required: true},
		},
		Outputs: []Pin{
			{ID: "dressed_image", Name: "Dressed Model Image", Type: TypeImage},
		},
		DefaultData: map[string]interface{}{
			"resolution": "1080p",
			"fidelity":   0.9,
			"provider":   "gemini_web",
		},
	})

	// 6. Web Motion Transfer Adapter (Muse AI Video Studio - Free)
	reg.Register(NodeDef{
		Type:        NodeWebMotionTransfer,
		Label:       "Web Adapter: Vũ Đạo Sexy Dance (Muse AI Free Studio)",
		Category:    "adapter",
		Description: "Tạo chuyển động vũ đạo mượt mà 60fps từ mẫu Douyin bằng Muse AI Video Web Studio (Tài khoản Free không tốn phí).",
		Inputs: []Pin{
			{ID: "reference_video", Name: "Dance Reference Video", Type: TypeVideo, Required: true},
			{ID: "character_image", Name: "Character Target Image", Type: TypeImage, Required: true},
		},
		Outputs: []Pin{
			{ID: "motion_video", Name: "Motion Dancing Video", Type: TypeVideo},
		},
		DefaultData: map[string]interface{}{
			"fps":        30,
			"background": "preserve",
			"provider":   "muse_ai",
		},
	})

	// 7. Web Video Gen Adapter (Google Flow Veo - 1.000 Credits Free)
	reg.Register(NodeDef{
		Type:        NodeWebVideoGen,
		Label:       "Web Adapter: Google Flow (Veo 1.000 Credits Free)",
		Category:    "adapter",
		Description: "Sinh video chuyển động tự nhiên từ ảnh keyframe bằng Google Flow (Tận dụng 1.000 credits miễn phí mỗi tháng) hoặc Muse AI Video.",
		Inputs: []Pin{
			{ID: "keyframe_image", Name: "First Frame Image", Type: TypeImage, Required: true},
			{ID: "motion_prompt", Name: "Camera Motion Prompt", Type: TypeString, Required: true},
		},
		Outputs: []Pin{
			{ID: "video_clip", Name: "Rendered Video Clip (480p)", Type: TypeVideo},
		},
		DefaultData: map[string]interface{}{
			"duration_sec": 5,
			"aspect":       "9:16",
			"provider":     "google_flow",
		},
	})

	// 8. Web Image Gen Adapter (Gemini 3.8 Flash / Muse)
	reg.Register(NodeDef{
		Type:        NodeWebImageGen,
		Label:       "Web Adapter: Tạo Ảnh 4K (Gemini 3.8 Flash / Muse)",
		Category:    "adapter",
		Description: "Sinh ảnh bối cảnh, phân cảnh điện ảnh hoặc người mẫu chân dung 4K không watermark.",
		Inputs: []Pin{
			{ID: "prompt", Name: "Visual Cinematic Prompt", Type: TypeString, Required: true},
			{ID: "aspect", Name: "Aspect Ratio (9:16 / 16:9)", Type: TypeString, Required: false},
		},
		Outputs: []Pin{
			{ID: "image_url", Name: "Generated Image URL", Type: TypeImage},
		},
		DefaultData: map[string]interface{}{
			"aspect":  "9:16",
			"quality": "4k_cinematic",
		},
	})

	// 9. Storyboard Decomposer Adapter
	reg.Register(NodeDef{
		Type:        NodeWebStoryDecompose,
		Label:       "Web Adapter: Phân Đoạn Không-Thời Gian Điện Ảnh",
		Category:    "adapter",
		Description: "Phân tách cốt truyện thành chuỗi phân cảnh 8-15s với đặc tả góc máy, ánh sáng, thời gian.",
		Inputs: []Pin{
			{ID: "story_text", Name: "Raw Story Text", Type: TypeString, Required: true},
			{ID: "genre", Name: "Genre (Kinh dị, Tiên hiệp...)", Type: TypeString, Required: false},
		},
		Outputs: []Pin{
			{ID: "scene_prompts", Name: "Cinematic Scene Prompts", Type: TypeList},
			{ID: "voiceover_chunks", Name: "Voiceover Sub-paragraphs", Type: TypeList},
		},
		DefaultData: map[string]interface{}{
			"seconds_per_scene": 10,
		},
	})

	// 10. Web Face Swap Adapter Node (FaceFusion / Vidnoz / Remaker Web)
	reg.Register(NodeDef{
		Type:        NodeWebFaceSwap,
		Label:       "Web Adapter: Khóa & Đổi Mặt AI (Cloud Face Swap)",
		Category:    "adapter",
		Description: "100% Web Cloud Adapter: Ghép khuôn mặt creator vào từng khung hình video với độ tương thích giải phẫu học cao, không tốn GPU/RAM máy cá nhân.",
		Inputs: []Pin{
			{ID: "target_video", Name: "Target Motion Video", Type: TypeVideo, Required: true},
			{ID: "source_face", Name: "Creator Face Image / Latent", Type: TypeImage, Required: true},
		},
		Outputs: []Pin{
			{ID: "swapped_video", Name: "Identity Locked Video", Type: TypeVideo},
		},
		DefaultData: map[string]interface{}{
			"blend_ratio": 0.95,
			"detector":    "scrfd_10g",
		},
	})

	// 11. Gemini Voice Cloud TTS Node (Google AI Studio API)
	geminiTTSDef := NodeDef{
		Type:        NodeGeminiTTS,
		Label:       "Gemini Voice Cloud TTS (Google AI Studio)",
		Category:    "adapter",
		Description: "Lồng tiếng biểu cảm cao bằng Google Gemini API (responseModalities AUDIO), hỗ trợ phong cách thoại kịch tính/truyền cảm, 100% Cloud zero local model.",
		Inputs: []Pin{
			{ID: "text", Name: "Text Chunks", Type: TypeString, Required: true},
			{ID: "voice_style", Name: "Voice Tone", Type: TypeString, Required: false},
		},
		Outputs: []Pin{
			{ID: "speech_audio", Name: "Master Speech Audio", Type: TypeAudio},
		},
		DefaultData: map[string]interface{}{
			"sample_rate": 24000,
			"provider":    "gemini",
			"voice":       "Aoede",
			"emotion":     "storyteller",
		},
	}
	reg.Register(geminiTTSDef)

	webTTSDef := geminiTTSDef
	webTTSDef.Type = NodeWebTTS
	reg.Register(webTTSDef)

	// 12. Local Acoustic Foley Bed Node
	reg.Register(NodeDef{
		Type:        NodeAcousticFoleyBed,
		Label:       "Local Audio: Hòa Âm Foley & Nhạc Nền (-18dB)",
		Category:    "local",
		Description: "Trộn âm thanh môi trường (tiếng gió hú, mưa rơi, tiếng kiếm) với nhạc nền điện ảnh lót nhẹ.",
		Inputs: []Pin{
			{ID: "voice_audio", Name: "Voice Audio", Type: TypeAudio, Required: true},
			{ID: "mood", Name: "Mood / Genre Foley", Type: TypeString, Required: false},
		},
		Outputs: []Pin{
			{ID: "mixed_audio", Name: "Polished Multi-Layer Audio", Type: TypeAudio},
		},
		DefaultData: map[string]interface{}{
			"bgm_level_db":   -18.0,
			"foley_level_db": -14.0,
		},
	})

	// 13. Local Anti-Distortion Crop Node
	reg.Register(NodeDef{
		Type:        NodeAntiDistortion,
		Label:       "Local Video: Cắt Chuẩn 9:16 Chống Méo Hình",
		Category:    "local",
		Description: "Bảo toàn giải phẫu học cơ thể và tỷ lệ sản phẩm, chống kéo dãn méo mó hình học.",
		Inputs: []Pin{
			{ID: "input_media", Name: "Input Media (Video/Image)", Type: TypeVideo, Required: true},
		},
		Outputs: []Pin{
			{ID: "cropped_media", Name: "1080x1920 Clean 9:16 Media", Type: TypeVideo},
		},
		DefaultData: map[string]interface{}{
			"target_w": 1080,
			"target_h": 1920,
		},
	})

	// 14. Local Video Remaster Node
	reg.Register(NodeDef{
		Type:        NodeVideoRemaster,
		Label:       "Local Upscale: Nâng Cấp 480p Lên 4K Crisp",
		Category:    "local",
		Description: "Remaster video Veo SD 480p lên độ phân giải cao 1080p/4K 60fps mượt mà.",
		Inputs: []Pin{
			{ID: "sd_video", Name: "SD 480p Video", Type: TypeVideo, Required: true},
		},
		Outputs: []Pin{
			{ID: "hd_video", Name: "Remastered Crisp Video", Type: TypeVideo},
		},
		DefaultData: map[string]interface{}{
			"scale_factor": 2,
		},
	})

	// 15. Local CapCut Beat Mux Node
	reg.Register(NodeDef{
		Type:        NodeCapCutBeatMux,
		Label:       "Local Edit: Cắt Ghép Nhịp Beat CapCut",
		Category:    "local",
		Description: "Cắt ghép chuyển cảnh đồng bộ tuyệt đối với beat drop âm nhạc theo phong cách CapCut Pro.",
		Inputs: []Pin{
			{ID: "video_stream", Name: "Video Stream", Type: TypeVideo, Required: true},
			{ID: "audio_stream", Name: "Audio Stream", Type: TypeAudio, Required: true},
			{ID: "bpm", Name: "BPM Beat Markers", Type: TypeNumber, Required: false},
		},
		Outputs: []Pin{
			{ID: "final_video", Name: "Beat-Synced Final Video", Type: TypeVideo},
		},
		DefaultData: map[string]interface{}{
			"transition": "beat_drop_cut",
		},
	})

	// 16. QC Soul Index Node
	reg.Register(NodeDef{
		Type:        NodeSoulIndexQC,
		Label:       "QC Scorecard: Thất Pháp Video Có Hồn (Soul Index)",
		Category:    "qc",
		Description: "Kiểm định 7 tiêu chí: không méo hình, người thật chuyển động, không giật lắc, âm thanh 3 lớp.",
		Inputs: []Pin{
			{ID: "video_file", Name: "Finished Video", Type: TypeVideo, Required: true},
		},
		Outputs: []Pin{
			{ID: "score", Name: "QC Score (0-100)", Type: TypeNumber},
			{ID: "verdict", Name: "Verdict (Pass/Fail)", Type: TypeString},
		},
		DefaultData: map[string]interface{}{
			"min_score": 85,
		},
	})

	// 17. Export Target Node
	reg.Register(NodeDef{
		Type:        NodeExportTarget,
		Label:       "Đích Xuất Bản (Export & Package)",
		Category:    "output",
		Description: "Gói file video MP4, nhạc và thumbnail; hỗ trợ mở ngay trong CapCut Pro hoặc đăng tải.",
		Inputs: []Pin{
			{ID: "final_media", Name: "Final Media File", Type: TypeVideo, Required: true},
			{ID: "title", Name: "Title / Caption", Type: TypeString, Required: false},
		},
		Outputs: []Pin{
			{ID: "output_path", Name: "Export File Path", Type: TypeString},
		},
		DefaultData: map[string]interface{}{
			"package_capcut": true,
		},
	})

	// 18. Cinema Script Breakdown Node
	reg.Register(NodeDef{
		Type:        NodeCinemaScript,
		Label:       "Kịch Bản & Phân Cảnh Đạo Diễn (Shot List)",
		Category:    "input",
		Description: "Phân tách kịch bản vi kịch 3 hồi thành danh sách phân cảnh kỹ thuật (Shot size, Camera, Thoại).",
		Inputs: []Pin{
			{ID: "script_prompt", Name: "Script Prompt / Idea", Type: TypeString, Required: true},
			{ID: "genre", Name: "Genre (Tiên hiệp, Trinh thám...)", Type: TypeString, Required: false},
		},
		Outputs: []Pin{
			{ID: "shot_list", Name: "Technical Shot List", Type: TypeJSON},
			{ID: "dialogue_list", Name: "Character Dialogues", Type: TypeList},
			{ID: "film_title", Name: "Film Title", Type: TypeString},
		},
		DefaultData: map[string]interface{}{
			"genre":     "tien_hiep",
			"num_shots": 6,
		},
	})

	// 19. Actor Bank 360° Turnaround Node
	reg.Register(NodeDef{
		Type:        NodeActorBank,
		Label:       "Ngân Hàng Diễn Viên 360° (Actor Bank)",
		Category:    "input",
		Description: "Khóa nhân vật nhất quán 360 độ (Chính diện, 45°, 90°, sau lưng) và bảng màu trang phục.",
		Inputs: []Pin{
			{ID: "actor_id", Name: "Actor ID / Profile", Type: TypeString, Required: false},
		},
		Outputs: []Pin{
			{ID: "turnaround_sheet", Name: "360° Turnaround Image", Type: TypeImage},
			{ID: "face_latent", Name: "Face Latent Embedding", Type: TypeFace},
			{ID: "wardrobe_meta", Name: "Wardrobe Consistency", Type: TypeJSON},
		},
		DefaultData: map[string]interface{}{
			"actor_id": "actor_lead_wuxia",
		},
	})

	// 20. Cinematic Camera Motion Node
	reg.Register(NodeDef{
		Type:        NodeCinematicCamera,
		Label:       "Đạo Diễn Góc Máy Điện Ảnh (Camera Motion)",
		Category:    "local",
		Description: "Chỉ định chuyển động camera điện ảnh chính xác (Slow Dolly In, Orbit 360°, Tracking, Whip Pan).",
		Inputs: []Pin{
			{ID: "shot_index", Name: "Shot Index", Type: TypeNumber, Required: false},
			{ID: "movement", Name: "Movement (dolly_in, orbit...)", Type: TypeString, Required: false},
		},
		Outputs: []Pin{
			{ID: "camera_prompt", Name: "Camera Trajectory Prompt", Type: TypeString},
		},
		DefaultData: map[string]interface{}{
			"movement": "dolly_in",
		},
	})

	// 21. Web Google Flow / Muse AI Video Gen Adapter (0đ)
	reg.Register(NodeDef{
		Type:        NodeWebKlingVideo,
		Label:       "Web Adapter: Google Flow & Muse AI Video Keyframes",
		Category:    "adapter",
		Description: "Sinh video chuyển động điện ảnh bằng Google Flow (1.000 Credits Free) và Muse AI Video Studio (100% Miễn Phí, không tốn tiền dịch vụ trả phí).",
		Inputs: []Pin{
			{ID: "start_frame", Name: "First Frame Image", Type: TypeImage, Required: true},
			{ID: "end_frame", Name: "Last Frame Image", Type: TypeImage, Required: false},
			{ID: "motion_prompt", Name: "Camera Motion Prompt", Type: TypeString, Required: true},
		},
		Outputs: []Pin{
			{ID: "rendered_clip", Name: "Keyframe Rendered Video", Type: TypeVideo},
		},
		DefaultData: map[string]interface{}{
			"model": "google_flow_veo_free",
			"mode":  "first_last_frame",
		},
	})

	// 22. Web Lip Sync Adapter (Muse AI Web Sync - Free)
	reg.Register(NodeDef{
		Type:        NodeWebLipSync,
		Label:       "Web Adapter: Khớp Khẩu Hình (Muse AI Web Sync Free)",
		Category:    "adapter",
		Description: "Khớp từng âm tiết của lời thoại lồng tiếng vào khẩu hình môi bằng Muse AI Web Sync (Tài khoản Free không tốn phí).",
		Inputs: []Pin{
			{ID: "video_clip", Name: "Actor Video Clip", Type: TypeVideo, Required: true},
			{ID: "audio_dialogue", Name: "Voice Dialogue Audio", Type: TypeAudio, Required: true},
		},
		Outputs: []Pin{
			{ID: "synced_video", Name: "Lip Synced Cinema Video", Type: TypeVideo},
		},
		DefaultData: map[string]interface{}{
			"engine": "muse_ai_web_free",
		},
	})

	// 23. Audio Ducking 4-Layer Foley Node
	reg.Register(NodeDef{
		Type:        NodeAudioDuckingFoley,
		Label:       "Hòa Âm Điện Ảnh 4 Tầng & Auto-Ducking",
		Category:    "local",
		Description: "Hòa âm Thoại + Foley động tác + Foley môi trường + Nhạc nền, tự động hạ nhạc -18dB khi nhân vật nói thoại.",
		Inputs: []Pin{
			{ID: "voice_audio", Name: "Dialogue Voice Audio", Type: TypeAudio, Required: true},
			{ID: "bgm_audio", Name: "Cinematic BGM", Type: TypeAudio, Required: false},
			{ID: "ambience_audio", Name: "Ambience Foley", Type: TypeAudio, Required: false},
		},
		Outputs: []Pin{
			{ID: "master_audio", Name: "4-Layer Master Audio Mix", Type: TypeAudio},
		},
		DefaultData: map[string]interface{}{
			"ducking_depth_db": -18.0,
			"attack_ms":        200,
			"release_ms":       400,
		},
	})

	// 24. Color Grading LUT Node
	reg.Register(NodeDef{
		Type:        NodeColorGradingLUT,
		Label:       "Chỉnh Màu Điện Ảnh LUT (Color Grading)",
		Category:    "local",
		Description: "Áp bảng màu điện ảnh Hollywood Teal-Orange, Wuxia Hổ Phách Cổ Phong hoặc Neo-Noir Chiaroscuro.",
		Inputs: []Pin{
			{ID: "input_video", Name: "Input Video", Type: TypeVideo, Required: true},
			{ID: "lut_preset", Name: "LUT Preset", Type: TypeString, Required: false},
		},
		Outputs: []Pin{
			{ID: "graded_video", Name: "Color Graded Master Video", Type: TypeVideo},
		},
		DefaultData: map[string]interface{}{
			"lut_preset": "wuxia_amber_gold",
		},
	})

	// 25. Cinema Master Export Node
	reg.Register(NodeDef{
		Type:        NodeCinemaMaster,
		Label:       "Xuất Phim Master (Cinemascope 21:9 & 9:16)",
		Category:    "output",
		Description: "Xuất phim ngắn điện ảnh hoàn chỉnh tỷ lệ màn ảnh rộng 21:9 hoặc 9:16 Vi Kịch Douyin Full HD.",
		Inputs: []Pin{
			{ID: "final_media", Name: "Graded Video File", Type: TypeVideo, Required: true},
			{ID: "aspect_ratio", Name: "Aspect (21:9 / 9:16)", Type: TypeString, Required: false},
		},
		Outputs: []Pin{
			{ID: "master_path", Name: "Cinema Master Path", Type: TypeString},
		},
		DefaultData: map[string]interface{}{
			"aspect_ratio": "21:9",
			"quality":      "prores_master",
		},
	})
}
