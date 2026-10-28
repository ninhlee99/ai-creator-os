package flow

import (
	"sort"
	"sync"
)

var (
	globalRegistry = make(map[string]NodeDefinition)
	registryMu     sync.RWMutex
)

// RegisterNode đăng ký một mẫu node mới vào hệ thống.
func RegisterNode(def NodeDefinition) {
	registryMu.Lock()
	defer registryMu.Unlock()
	globalRegistry[def.Type] = def
}

// GetNodeDefinition lấy định nghĩa của một loại node theo type.
func GetNodeDefinition(nodeType string) (NodeDefinition, bool) {
	registryMu.RLock()
	defer registryMu.RUnlock()
	def, ok := globalRegistry[nodeType]
	return def, ok
}

// ListDefinitions trả về danh sách tất cả các loại node có sẵn trong hệ thống.
func ListDefinitions() []NodeDefinition {
	registryMu.RLock()
	defer registryMu.RUnlock()
	list := make([]NodeDefinition, 0, len(globalRegistry))
	for _, def := range globalRegistry {
		if isUnsupportedExperimental3DNode(def.Type) {
			continue
		}
		list = append(list, def)
	}
	sort.Slice(list, func(i, j int) bool {
		if list[i].Category != list[j].Category {
			return list[i].Category < list[j].Category
		}
		if list[i].Title != list[j].Title {
			return list[i].Title < list[j].Title
		}
		return list[i].Type < list[j].Type
	})
	return list
}

func isUnsupportedExperimental3DNode(nodeType string) bool {
	switch nodeType {
	case "character_identity", "face_lock_refiner", "body_skeleton_pose", "garment_mesh_extractor",
		"surface_cloth_warper", "virtual_camera_rig", "three_point_light_rig", "hdri_environment_lighting",
		"neural_composite_shader":
		return true
	default:
		return false
	}
}

func init() {
	registerAllBuiltinNodes()
}

func registerAllBuiltinNodes() {
	// 1. Character Input Node
	RegisterNode(NodeDefinition{
		Type:        "character_input",
		Title:       "Nhân vật mẫu (Character)",
		Category:    CategoryInput,
		Description: "Chọn ảnh nhân vật đã tải lên hoặc lấy ảnh từ thư viện của account đã chọn",
		Icon:        "user",
		Inputs:      []FlowPort{},
		Outputs: []FlowPort{
			{ID: "character_image", Name: "Ảnh nhân vật", Type: PortTypeImage, Description: "Ảnh khuôn mặt / chân dung nhân vật đại diện"},
			{ID: "character_images", Name: "Danh sách ảnh", Type: PortTypeImageList, Description: "Tập hợp các góc mặt của nhân vật mẫu"},
			{ID: "character_name", Name: "Tên nhân vật", Type: PortTypeText, Description: "Tên hoặc mô tả ngắn của nhân vật"},
		},
		DefaultParams: map[string]any{
			"account_id":        0,
			"model_path":        "",
			"model_paths":       []string{},
			"model_photo_index": 0,
			"model_name":        "",
		},
		Executor: executeCharacterInput,
	})

	// 2. Product Input Node
	RegisterNode(NodeDefinition{
		Type:        "product_input",
		Title:       "Sản phẩm mẫu (Product)",
		Category:    CategoryInput,
		Description: "Chọn sản phẩm đã duyệt trong catalog hoặc nhập ảnh, tên và affiliate URL thật",
		Icon:        "package",
		Inputs:      []FlowPort{},
		Outputs: []FlowPort{
			{ID: "product_image", Name: "Ảnh sản phẩm", Type: PortTypeImage, Description: "Ảnh gốc của sản phẩm cần tiếp thị"},
			{ID: "product_images", Name: "Ảnh sản phẩm bổ sung", Type: PortTypeImageList, Description: "Tối đa bốn ảnh listing đã tải"},
			{ID: "product_title", Name: "Tên sản phẩm", Type: PortTypeText, Description: "Tiêu đề sản phẩm"},
			{ID: "product_info", Name: "Dữ liệu sản phẩm", Type: PortTypeProduct, Description: "Chi tiết giá, link và hoa hồng sản phẩm"},
		},
		DefaultParams: map[string]any{
			"product_id":     "",
			"product_image":  "",
			"product_images": []string{},
			"product_title":  "",
			"affiliate_url":  "",
		},
		Executor: executeProductInput,
	})

	// 3. Video Source Node
	RegisterNode(NodeDefinition{
		Type:        "video_source",
		Title:       "Video mẫu gốc (Radar Video)",
		Category:    CategoryInput,
		Description: "Chọn video TikTok đã xác thực trong Radar hoặc video upload/URL trực tiếp được cho phép",
		Icon:        "video",
		Inputs:      []FlowPort{},
		Outputs: []FlowPort{
			{ID: "video_path", Name: "Video nguồn", Type: PortTypeVideo, Description: "File video mp4 đầy đủ của trend nguồn"},
			{ID: "audio_track", Name: "Âm thanh gốc", Type: PortTypeAudio, Description: "Track âm thanh / nhạc nền gốc từ video"},
			{ID: "duration", Name: "Thời lượng (s)", Type: PortTypeNumber, Description: "Độ dài video nguồn tính theo giây"},
			{ID: "video_id", Name: "Radar video ID", Type: PortTypeText, Description: "ID Radar nếu nguồn đã được xác thực"},
			{ID: "author", Name: "Tác giả nguồn", Type: PortTypeText, Description: "Tác giả do Radar lưu"},
			{ID: "sound_title", Name: "Tên âm thanh nguồn", Type: PortTypeText, Description: "Tên âm thanh do Radar lưu"},
		},
		DefaultParams: map[string]any{
			"video_url":           "",
			"video_path":          "",
			"require_radar_dance": false,
			"allow_external_url":  false,
		},
		Executor: executeVideoSource,
	})

	// 4. Audio Source Node
	RegisterNode(NodeDefinition{
		Type:        "audio_source",
		Title:       "Âm thanh / Nhạc nền (Audio)",
		Category:    CategoryInput,
		Description: "Tùy chọn: nạp file audio hoặc URL trực tiếp; để trống sẽ giữ âm thanh video nếu có",
		Icon:        "music",
		Inputs:      []FlowPort{},
		Outputs: []FlowPort{
			{ID: "audio_file", Name: "Track âm thanh", Type: PortTypeAudio, Description: "File âm thanh mp3/aac/wav"},
			{ID: "audio_title", Name: "Tên bài nhạc", Type: PortTypeText, Description: "Tiêu đề âm thanh"},
		},
		DefaultParams: map[string]any{
			"audio_path":  "",
			"audio_url":   "",
			"audio_title": "",
		},
		Executor: executeAudioSource,
	})
	RegisterNode(NodeDefinition{
		Type:        "text_to_speech",
		Title:       "Tạo giọng đọc (Text to Speech)",
		Category:    CategoryAI,
		Description: "Chuyển văn bản hoặc kịch bản thành file WAV bằng chuỗi TTS đã cấu hình trong Cài đặt.",
		Icon:        "mic",
		Inputs: []FlowPort{
			{ID: "text_in", Name: "Văn bản / kịch bản", Type: PortTypeText, Required: true},
			{ID: "voice_in", Name: "Phong cách giọng đọc", Type: PortTypeText, Required: false},
		},
		Outputs: []FlowPort{
			{ID: "audio_out", Name: "Giọng đọc (WAV)", Type: PortTypeAudio, Description: "File WAV 24 kHz mono đã tổng hợp."},
			{ID: "text_out", Name: "Văn bản đã đọc", Type: PortTypeText, Description: "Văn bản truyền tiếp để lưu hoặc hiển thị."},
		},
		DefaultParams: map[string]any{"voice": "warm-female"},
		Executor:      executeTextToSpeech,
	})

	// 5. Prompt Template Node
	RegisterNode(NodeDefinition{
		Type:        "prompt_template",
		Title:       "Tạo câu lệnh (Prompt Template)",
		Category:    CategoryProcess,
		Description: "Thay biến nhân vật, sản phẩm và chuyển động trong template prompt",
		Icon:        "edit-3",
		Inputs: []FlowPort{
			{ID: "character_desc", Name: "Mô tả nhân vật", Type: PortTypeText, Required: false},
			{ID: "product_desc", Name: "Mô tả sản phẩm", Type: PortTypeText, Required: false},
			{ID: "motion_desc", Name: "Mô tả chuyển động", Type: PortTypeText, Required: false},
			{ID: "character_name", Name: "Tên nhân vật", Type: PortTypeText, Required: false},
			{ID: "product_title", Name: "Tên sản phẩm", Type: PortTypeText, Required: false},
			{ID: "outfit_desc", Name: "Mô tả trang phục", Type: PortTypeText, Required: false},
			{ID: "environment_desc", Name: "Mô tả bối cảnh", Type: PortTypeText, Required: false},
			{ID: "lighting_desc", Name: "Mô tả ánh sáng", Type: PortTypeText, Required: false},
			{ID: "variables", Name: "Biến tùy chỉnh", Type: PortTypeConfig, Required: false},
			{ID: "duration_seconds", Name: "Thời lượng (giây)", Type: PortTypeNumber, Required: false},
		},
		Outputs: []FlowPort{
			{ID: "prompt", Name: "Câu lệnh hoàn chỉnh", Type: PortTypeText, Description: "Prompt chuẩn hóa đã chèn các biến"},
			{ID: "workflow_context", Name: "Context workflow con", Type: PortTypeConfig, Description: "Map biến có thể truyền vào Workflow Map hoặc Workflow Call"},
		},
		DefaultParams: map[string]any{
			"template": "Cinematic vertical 9:16 video of {character} {motion}, showcasing {product}. Beautiful soft lighting, 4k ultra realistic, smooth motion.",
		},
		Executor: executePromptTemplate,
	})

	// 6. Product Extractor Node (Bóc tách sản phẩm)
	RegisterNode(NodeDefinition{
		Type:        "product_extractor",
		Title:       "Bóc tách sản phẩm (Extract Product)",
		Category:    CategoryProcess,
		Description: "Trích frame video nếu cần, rồi tách nền bằng model local hoặc heuristic; kiểm tra cutout trước khi dùng",
		Icon:        "scissors",
		Inputs: []FlowPort{
			{ID: "source_image", Name: "Ảnh hoặc Video nguồn", Type: PortTypeAny, Required: false},
			{ID: "source_video", Name: "Video nguồn", Type: PortTypeVideo, Required: false},
		},
		Outputs: []FlowPort{
			{ID: "cutout_image", Name: "Ảnh sản phẩm đã tách", Type: PortTypeImage, Description: "Sản phẩm độc lập đã xóa phông nền trong suốt"},
			{ID: "mask_image", Name: "Mặt nạ (Mask)", Type: PortTypeImage, Description: "Ảnh mặt nạ nhị phân phục vụ inpainting"},
			{ID: "source_frame", Name: "Khung hình gốc", Type: PortTypeImage, Description: "Khung hình trích xuất từ video nguồn"},
			{ID: "product_image", Name: "Sản phẩm đầu ra", Type: PortTypeImage, Description: "Tương thích nối tiếp với các node nhận product_image"},
		},
		DefaultParams: map[string]any{
			"video_timestamp": "auto",
			"refine_edges":    true,
			"auto_crop":       true,
		},
		Executor: executeProductExtractor,
	})

	// 7. Image Composite Node (Ghép nối sản phẩm với nhân vật mẫu)
	RegisterNode(NodeDefinition{
		Type:        "image_composite",
		Title:       "Ghép sản phẩm với nhân vật (Composite)",
		Category:    CategoryProcess,
		Description: "Ghép ảnh sản phẩm dạng overlay lên ảnh nhân vật tại vị trí xấp xỉ; không tạo tư thế cầm sản phẩm",
		Icon:        "layers",
		Inputs: []FlowPort{
			{ID: "character_image", Name: "Ảnh nhân vật", Type: PortTypeImage, Required: true},
			{ID: "product_image", Name: "Ảnh sản phẩm", Type: PortTypeImage, Required: true},
		},
		Outputs: []FlowPort{
			{ID: "composite_image", Name: "Ảnh overlay", Type: PortTypeImage, Description: "Ảnh sản phẩm được overlay tại vị trí xấp xỉ"},
		},
		DefaultParams: map[string]any{
			"placement": "holding_hand",
			"scale":     0.45,
		},
		Executor: executeImageComposite,
	})

	// 8. Dance Motion Extractor Node (Bóc tách điệu nhảy từ video gốc)
	RegisterNode(NodeDefinition{
		Type:        "dance_motion_extractor",
		Title:       "Bóc tách điệu nhảy (Extract Dance)",
		Category:    CategoryProcess,
		Description: "Trích keyframe theo thời lượng video rồi dùng Vision AI mô tả chuỗi chuyển động; không đo BPM",
		Icon:        "activity",
		Inputs: []FlowPort{
			{ID: "video_in", Name: "Video gốc", Type: PortTypeVideo, Required: true},
		},
		Outputs: []FlowPort{
			{ID: "motion_data", Name: "Dữ liệu vũ đạo", Type: PortTypeMotion, Description: "Mô tả thứ tự chuyển động và thời lượng; không đo BPM"},
			{ID: "keyframes", Name: "Keyframe vũ đạo", Type: PortTypeImageList, Description: "Các frame tư thế chính trích xuất từ video"},
			{ID: "beat_audio", Name: "Âm thanh trích xuất", Type: PortTypeAudio, Description: "Track audio gốc trích xuất, không phân tích nhịp"},
			{ID: "dance_style", Name: "Mô tả chuyển động", Type: PortTypeText, Description: "Mô tả chuyển động do Vision AI tạo"},
			{ID: "source_video", Name: "Video chuyển động nguồn", Type: PortTypeVideo, Description: "Video chỉ dùng làm tham chiếu chuyển động"},
		},
		DefaultParams: map[string]any{
			"dance_style":              "",
			"analysis_prompt_template": "",
			"keyframe_count":           5,
			"require_audio":            false,
			"strict_analysis":          false,
			"analysis_timeout_seconds": 25,
		},
		Executor: executeDanceMotionExtractor,
	})

	// 9. Dance Retarget Node (Ghép nối điệu nhảy, biểu cảm vào nhân vật)
	RegisterNode(NodeDefinition{
		Type:        "dance_retarget",
		Title:       "Tạo prompt vũ đạo",
		Category:    CategoryProcess,
		Description: "Tạo prompt video từ phân tích chuyển động và ảnh tham chiếu; không thực hiện motion capture/retarget",
		Icon:        "shuffle",
		Inputs: []FlowPort{
			{ID: "character_image", Name: "Ảnh nhân vật mẫu", Type: PortTypeImage, Required: true},
			{ID: "character_images", Name: "Ảnh nhân vật bổ sung", Type: PortTypeImageList, Required: false},
			{ID: "motion_data", Name: "Dữ liệu vũ đạo", Type: PortTypeMotion, Required: true},
			{ID: "keyframes", Name: "Keyframes vũ đạo", Type: PortTypeImageList, Required: false},
			{ID: "reference_video", Name: "Video chuyển động nguồn", Type: PortTypeVideo, Required: false},
			{ID: "ingredients", Name: "Ảnh tham chiếu bổ sung", Type: PortTypeImageList, Required: false},
			{ID: "environment_desc", Name: "Mô tả không gian", Type: PortTypeText, Required: false},
			{ID: "lighting_desc", Name: "Mô tả ánh sáng", Type: PortTypeText, Required: false},
			{ID: "outfit_desc", Name: "Mô tả trang phục", Type: PortTypeText, Required: false},
		},
		Outputs: []FlowPort{
			{ID: "prompt_choreography", Name: "Prompt vũ đạo", Type: PortTypeText, Description: "Mô tả chuyển động nghệ thuật an toàn cho AI"},
			{ID: "prompt", Name: "Câu lệnh tạo video", Type: PortTypeText, Description: "Prompt chuẩn hóa cho bộ sinh video AI"},
			{ID: "prompt_context", Name: "Biến prompt", Type: PortTypeConfig, Description: "Các biến chuyển động, bối cảnh, ánh sáng và outfit cho prompt template"},
			{ID: "ingredients", Name: "Thành phần chuẩn bị", Type: PortTypeImageList, Description: "Các ảnh nguyên liệu cho bộ tạo video"},
			{ID: "character_images", Name: "Ảnh danh tính nhân vật", Type: PortTypeImageList, Description: "Ảnh nhân vật tách riêng khỏi keyframe chuyển động"},
			{ID: "character_reference_count", Name: "Số ảnh danh tính", Type: PortTypeNumber, Description: "Số ảnh nhân vật liên tiếp đứng đầu danh sách tham chiếu"},
			{ID: "motion_keyframes", Name: "Keyframe kèm thời điểm", Type: PortTypeAny, Description: "Ảnh pose theo thứ tự, mỗi ảnh kèm thời điểm nguồn"},
			{ID: "character_image", Name: "Ảnh nhân vật", Type: PortTypeImage, Description: "Ảnh nhân vật truyền tiếp"},
			{ID: "reference_video", Name: "Video chuyển động nguồn", Type: PortTypeVideo, Description: "Video nguồn chỉ làm tham chiếu chuyển động"},
		},
		DefaultParams: map[string]any{
			"expression":      "confident",
			"dance_type":      "",
			"prompt_template": "",
		},
		Executor: executeDanceRetarget,
	})

	// 10. Google Flow Video Node
	RegisterNode(NodeDefinition{
		Type:        "google_flow_video",
		Title:       "Google Flow Veo 2 (Web AI Video)",
		Category:    CategoryAI,
		Description: "Tạo video bằng Google Flow; cần prompt và ít nhất một ảnh tham chiếu thật",
		Icon:        "zap",
		Inputs: []FlowPort{
			{ID: "prompt", Name: "Câu lệnh prompt", Type: PortTypeText, Required: true},
			{ID: "ingredients", Name: "Thành phần ảnh", Type: PortTypeImageList, Required: false},
			{ID: "character_image", Name: "Ảnh nhân vật", Type: PortTypeImage, Required: false},
		},
		Outputs: []FlowPort{
			{ID: "video_out", Name: "Video tạo bởi Flow", Type: PortTypeVideo, Description: "Video 9:16 độ phân giải cao từ Google Flow"},
		},
		DefaultParams: map[string]any{
			"aspect": "VIDEO_ASPECT_RATIO_PORTRAIT",
			// Blank means use connected prompt input. A custom value overrides it.
			"prompt": "",
		},
		Executor: executeGoogleFlowVideo,
	})

	// 11. Muse AI Video Node
	RegisterNode(NodeDefinition{
		Type:        "muse_ai_video",
		Title:       "Muse AI Video (Web AI)",
		Category:    CategoryAI,
		Description: "Tạo video AI trực tiếp qua tài khoản Web AI Muse AI",
		Icon:        "film",
		Inputs: []FlowPort{
			{ID: "prompt", Name: "Câu lệnh prompt", Type: PortTypeText, Required: true},
			{ID: "reference_image", Name: "Ảnh tham chiếu", Type: PortTypeImage, Required: false},
			{ID: "reference_images", Name: "Các ảnh tham chiếu", Type: PortTypeImageList, Required: false},
			{ID: "reference_video", Name: "Video tham chiếu", Type: PortTypeVideo, Required: false},
		},
		Outputs: []FlowPort{
			{ID: "video_out", Name: "Video từ Muse AI", Type: PortTypeVideo, Description: "Video hoàn chỉnh sinh bởi Muse AI"},
		},
		DefaultParams: map[string]any{
			"aspect": "9:16",
			// Blank means use connected prompt input. A custom value overrides it.
			"prompt": "",
		},
		Executor: executeMuseAIVideo,
	})

	// 12. Gemini Vision Node
	RegisterNode(NodeDefinition{
		Type:        "gemini_vision",
		Title:       "Phân tích Gemini Vision (Visual AI)",
		Category:    CategoryAI,
		Description: "Dùng Gemini phân tích ảnh nhân vật/sản phẩm, bóc tách đặc điểm và tự động viết prompt tối ưu",
		Icon:        "eye",
		Inputs: []FlowPort{
			{ID: "image_in", Name: "Ảnh cần phân tích", Type: PortTypeImage, Required: true},
			{ID: "task", Name: "Nhiệm vụ phân tích", Type: PortTypeText, Required: false},
		},
		Outputs: []FlowPort{
			{ID: "description", Name: "Mô tả chi tiết", Type: PortTypeText, Description: "Phân tích trực quan sâu từ Gemini"},
			{ID: "optimized_prompt", Name: "Prompt tối ưu", Type: PortTypeText, Description: "Prompt tạo video hoàn thiện"},
		},
		DefaultParams: map[string]any{"task": ""},
		Executor:      executeGeminiVision,
	})

	// 13. Custom Web AI Node (Node AI tùy biến mở rộng - legacy alias của custom_universal_ai)
	RegisterNode(NodeDefinition{
		Type:        "custom_web_ai",
		Title:       "Node Web AI tùy biến (Custom AI)",
		Category:    CategoryCustom,
		Description: "Tạo text, ảnh hoặc video qua LLM, image generator, Google Flow hoặc Muse AI đã cấu hình",
		Icon:        "cpu",
		Hidden:      true,
		Inputs: []FlowPort{
			{ID: "prompt_in", Name: "Prompt đầu vào", Type: PortTypeText, Required: true},
			{ID: "asset_in", Name: "File đầu vào", Type: PortTypeAny, Required: false},
		},
		Outputs: []FlowPort{
			{ID: "result_out", Name: "Kết quả xuất", Type: PortTypeAny, Description: "File video hoặc ảnh từ Custom AI"},
			{ID: "text_out", Name: "Văn bản phản hồi", Type: PortTypeText, Description: "Nội dung phản hồi dạng text"},
		},
		DefaultParams: map[string]any{
			"provider_name": "googleflow",
			"output_type":   "video",
			"prompt":        "",
		},
		Executor: executeCustomWebAI,
	})

	// 14. Audio Merge Node
	RegisterNode(NodeDefinition{
		Type:        "audio_merge",
		Title:       "Ghép âm thanh (Audio Merge)",
		Category:    CategoryProcess,
		Description: "Đồng bộ nhạc nền hoặc track âm thanh gốc từ TikTok vào video đã render của AI",
		Icon:        "volume-2",
		Inputs: []FlowPort{
			{ID: "video_in", Name: "Video hình ảnh", Type: PortTypeVideo, Required: true},
			{ID: "audio_in", Name: "Âm thanh gốc (tùy chọn)", Type: PortTypeAudio, Required: false},
		},
		Outputs: []FlowPort{
			{ID: "video_out", Name: "Video có âm thanh", Type: PortTypeVideo, Description: "Video mp4 hoàn chỉnh đồng bộ tiếng và hình"},
		},
		DefaultParams: map[string]any{
			"audio_codec": "aac",
		},
		Executor: executeAudioMerge,
	})
	RegisterNode(NodeDefinition{
		Type:        "audio_mix",
		Title:       "Trộn giọng đọc & nhạc nền (Audio Mix)",
		Category:    CategoryProcess,
		Description: "Trộn track chính với nhạc nền; có thể tự giảm nhạc khi có lời đọc và lặp nhạc nền đến hết track chính.",
		Icon:        "volume-2",
		Inputs: []FlowPort{
			{ID: "voice_in", Name: "Track chính / giọng đọc", Type: PortTypeAudio, Required: false},
			{ID: "music_in", Name: "Nhạc nền tùy chọn", Type: PortTypeAudio, Required: false},
			{ID: "duration", Name: "Thời lượng đầu ra (giây)", Type: PortTypeNumber, Required: false},
		},
		Outputs: []FlowPort{
			{ID: "audio_out", Name: "Audio đã trộn", Type: PortTypeAudio, Description: "Bản phối WAV đã cân bằng âm lượng."},
		},
		DefaultParams: map[string]any{
			"voice_volume": 1.0,
			"music_volume": 0.18,
			"duck_music":   true,
			"loop_music":   true,
			"normalize":    true,
		},
		Executor: executeAudioMix,
	})

	// 15. Video Upscale Node
	RegisterNode(NodeDefinition{
		Type:        "video_upscale",
		Title:       "Làm nét & Upscale 4K (Upscale)",
		Category:    CategoryProcess,
		Description: "Nâng cấp độ phân giải video lên chuẩn 1080p hoặc 4K, tinh chỉnh chi tiết và tương phản",
		Icon:        "maximize",
		Inputs: []FlowPort{
			{ID: "video_in", Name: "Video đầu vào", Type: PortTypeVideo, Required: true},
			{ID: "resolution", Name: "Độ phân giải đầu ra", Type: PortTypeText, Required: false},
		},
		Outputs: []FlowPort{
			{ID: "video_out", Name: "Video nét cao (4K)", Type: PortTypeVideo, Description: "Video master sau khi upscale"},
		},
		DefaultParams: map[string]any{
			"resolution": "4k", // 1080p, 4k
			"crf":        18,
			"fit_mode":   "contain",
		},
		Executor: executeVideoUpscale,
	})

	// 16. Concat Scenes Node
	RegisterNode(NodeDefinition{
		Type:        "concat_scenes",
		Title:       "Ghép nối phân cảnh (Concat Scenes)",
		Category:    CategoryProcess,
		Description: "Ghép các đoạn video ngắn thành video hoàn chỉnh theo đúng thứ tự nhịp điệu",
		Icon:        "film",
		Inputs: []FlowPort{
			{ID: "scenes", Name: "Danh sách phân cảnh", Type: PortTypeAny, Required: false, Description: "Danh sách video theo thứ tự từ Workflow Map."},
			{ID: "scene_1", Name: "Phân cảnh 1", Type: PortTypeVideo, Required: false},
			{ID: "scene_2", Name: "Phân cảnh 2", Type: PortTypeVideo, Required: false},
			{ID: "scene_3", Name: "Phân cảnh 3", Type: PortTypeVideo, Required: false},
		},
		Outputs: []FlowPort{
			{ID: "master_video", Name: "Video Master", Type: PortTypeVideo, Description: "Video hoàn chỉnh nối liền mạch các cảnh"},
		},
		DefaultParams: map[string]any{"fit_mode": "contain"},
		Executor:      executeConcatScenes,
	})
	RegisterNode(NodeDefinition{
		Type:        "video_split",
		Title:       "Chia video theo đoạn (Video Split)",
		Category:    CategoryProcess,
		Description: "Chia video nguồn thành các đoạn liền mạch, đồng thời tách track âm thanh gốc để workflow xử lý và ghép lại.",
		Icon:        "scissors",
		Inputs: []FlowPort{
			{ID: "video_in", Name: "Video nguồn", Type: PortTypeVideo, Required: true},
			{ID: "character_images", Name: "Ảnh danh tính nhân vật", Type: PortTypeImageList, Required: false},
			{ID: "motion_keyframes", Name: "Keyframe kèm thời điểm", Type: PortTypeAny, Required: false},
			{ID: "prompt_context", Name: "Biến prompt", Type: PortTypeConfig, Required: false},
		},
		Outputs: []FlowPort{
			{ID: "segments", Name: "Danh sách đoạn video", Type: PortTypeAny, Description: "Các đoạn theo thứ tự, kèm đường dẫn và thời lượng."},
			{ID: "audio_out", Name: "Âm thanh nguồn", Type: PortTypeAudio, Description: "Track âm thanh gốc đã tách."},
			{ID: "duration", Name: "Thời lượng nguồn", Type: PortTypeNumber},
		},
		DefaultParams: map[string]any{"max_segment_seconds": 10, "require_audio": true},
		Executor:      executeVideoSplit,
	})
	RegisterNode(NodeDefinition{
		Type:        "video_trim",
		Title:       "Khớp thời lượng video (Video Trim)",
		Category:    CategoryProcess,
		Description: "Cắt video theo thời lượng mục tiêu đã nhập; chỉ dùng thời lượng video tham chiếu khi chưa có mục tiêu.",
		Icon:        "timer",
		Inputs: []FlowPort{
			{ID: "video_in", Name: "Video cần khớp", Type: PortTypeVideo, Required: true},
			{ID: "reference_video", Name: "Video lấy thời lượng", Type: PortTypeVideo, Required: false},
			{ID: "duration", Name: "Thời lượng mục tiêu (giây)", Type: PortTypeNumber, Required: false},
		},
		Outputs:       []FlowPort{{ID: "video_out", Name: "Video đúng thời lượng", Type: PortTypeVideo}},
		DefaultParams: map[string]any{"tolerance_seconds": 0.15},
		Executor:      executeVideoTrim,
	})

	// 17. Video Output Node
	RegisterNode(NodeDefinition{
		Type:        "video_output",
		Title:       "Xuất video TikTok (Video Output)",
		Category:    CategoryOutput,
		Description: "Sao chép video thành phẩm vào thư mục output để xem và tải về",
		Icon:        "check-circle",
		Inputs: []FlowPort{
			{ID: "video_in", Name: "Video thành phẩm", Type: PortTypeVideo, Required: true},
			{ID: "title", Name: "Tiêu đề video", Type: PortTypeText, Required: false},
		},
		Outputs: []FlowPort{
			{ID: "final_video", Name: "Video cuối cùng", Type: PortTypeVideo, Description: "Đường dẫn file video master sẵn sàng xuất bản"},
		},
		DefaultParams: map[string]any{},
		Executor:      executeVideoOutput,
	})

	// 18. Image Output Node
	RegisterNode(NodeDefinition{
		Type:        "image_output",
		Title:       "Xuất ảnh hoàn thiện (Image Output)",
		Category:    CategoryOutput,
		Description: "Xuất ảnh poster, lookbook hoặc thumbnail chất lượng cao",
		Icon:        "image",
		Inputs: []FlowPort{
			{ID: "image_in", Name: "Ảnh thành phẩm", Type: PortTypeImage, Required: true},
		},
		Outputs: []FlowPort{
			{ID: "final_image", Name: "Ảnh cuối cùng", Type: PortTypeImage, Description: "File ảnh hoàn thiện"},
		},
		DefaultParams: map[string]any{
			"format": "jpg",
		},
		Executor: executeImageOutput,
	})
	RegisterNode(NodeDefinition{
		Type:        "audio_output",
		Title:       "Xuất audio hoàn thiện (Audio Output)",
		Category:    CategoryOutput,
		Description: "Sao chép file audio thành phẩm vào thư mục output để nghe và tải về",
		Icon:        "volume-2",
		Inputs: []FlowPort{
			{ID: "audio_in", Name: "Audio thành phẩm", Type: PortTypeAudio, Required: true},
		},
		Outputs: []FlowPort{
			{ID: "final_audio", Name: "Audio cuối cùng", Type: PortTypeAudio, Description: "File audio hoàn thiện để nghe và tải về"},
		},
		DefaultParams: map[string]any{},
		Executor:      executeAudioOutput,
	})

	// 19. Outfit Prompt Builder. Image generator does not support true virtual try-on.
	RegisterNode(NodeDefinition{
		Type:        "virtual_tryon",
		Title:       "Chuẩn bị prompt trang phục",
		Category:    CategoryProcess,
		Description: "Giữ ảnh nhân vật và ảnh trang phục gốc làm tham chiếu; tạo mô tả outfit cho node sinh ảnh/video. Không thay quần áo trong ảnh tĩnh.",
		Icon:        "shirt",
		Inputs: []FlowPort{
			{ID: "character_image", Name: "Ảnh nhân vật mẫu", Type: PortTypeImage, Required: true},
			{ID: "character_images", Name: "Ảnh nhân vật bổ sung", Type: PortTypeImageList, Required: false},
			{ID: "garment_image", Name: "Ảnh trang phục", Type: PortTypeImage, Required: false},
			{ID: "product_images", Name: "Ảnh listing bổ sung", Type: PortTypeImageList, Required: false},
			{ID: "outfit_name", Name: "Tên/mô tả outfit", Type: PortTypeText, Required: false},
			{ID: "fit_style", Name: "Phong cách trang phục", Type: PortTypeText, Required: false},
			{ID: "prompt_in", Name: "Prompt đầu vào", Type: PortTypeText, Required: false},
		},
		Outputs: []FlowPort{
			{ID: "fitted_image", Name: "Ảnh nhân vật (tương thích cũ)", Type: PortTypeImage, Description: "Ảnh gốc truyền tiếp; chưa thay trang phục"},
			{ID: "character_image", Name: "Ảnh nhân vật gốc", Type: PortTypeImage, Description: "Ảnh gốc truyền tiếp làm tham chiếu"},
			{ID: "character_reference_count", Name: "Số ảnh danh tính", Type: PortTypeNumber, Description: "Số ảnh nhân vật liên tiếp đứng đầu danh sách tham chiếu"},
			{ID: "garment_image", Name: "Ảnh trang phục gốc", Type: PortTypeImage, Description: "Ảnh trang phục truyền tiếp làm tham chiếu"},
			{ID: "ingredients", Name: "Ảnh tham chiếu", Type: PortTypeImageList, Description: "Ảnh nhân vật và trang phục gốc"},
			{ID: "outfit_desc", Name: "Mô tả trang phục", Type: PortTypeText, Description: "Mô tả trang phục cho AI sinh video"},
		},
		DefaultParams: map[string]any{"prompt": ""},
		Executor:      executeVirtualTryOn,
	})

	// 20. Scene Environment Node (Không gian / Bối cảnh)
	RegisterNode(NodeDefinition{
		Type:        "scene_environment",
		Title:       "Không gian / Bối cảnh (Scene Environment)",
		Category:    CategoryProcess,
		Description: "Thiết lập không gian bối cảnh: căn hộ penthouse, boutique Pháp, studio tối giản, phòng tập vũ đạo neon, sân khấu biểu diễn Kpop, rooftop hoàng hôn",
		Icon:        "map-pin",
		Inputs: []FlowPort{
			{ID: "character_image", Name: "Ảnh nhân vật", Type: PortTypeImage, Required: false},
			{ID: "prompt_in", Name: "Câu lệnh đầu vào", Type: PortTypeText, Required: false},
		},
		Outputs: []FlowPort{
			{ID: "environment_desc", Name: "Mô tả không gian", Type: PortTypeText, Description: "Mô tả bối cảnh chi tiết cho AI"},
			{ID: "environment_prompt", Name: "Câu lệnh không gian", Type: PortTypeText, Description: "Prompt không gian hoàn chỉnh"},
			{ID: "prompt_out", Name: "Câu lệnh tổng hợp", Type: PortTypeText, Description: "Prompt đã chèn không gian"},
			{ID: "character_image", Name: "Ảnh nhân vật", Type: PortTypeImage, Description: "Ảnh nhân vật truyền tiếp"},
		},
		DefaultParams: map[string]any{
			"location_type":   "fashion_boutique",
			"custom_location": "",
			"depth_of_field":  "cinematic_bokeh",
			"prompt":          "",
		},
		Executor: executeSceneEnvironment,
	})

	// 21. Lighting Atmosphere Node (Thời gian & Ánh sáng)
	RegisterNode(NodeDefinition{
		Type:        "lighting_atmosphere",
		Title:       "Thời gian & Ánh sáng (Time & Lighting)",
		Category:    CategoryLighting,
		Description: "Thiết lập thời điểm trong ngày và hiệu ứng ánh sáng: hoàng hôn Golden Hour, ban mai dịu mát, ánh đèn neon đêm, ánh đèn sân khấu biểu diễn",
		Icon:        "sun",
		Inputs: []FlowPort{
			{ID: "prompt_in", Name: "Câu lệnh đầu vào", Type: PortTypeText, Required: false},
			{ID: "environment_desc", Name: "Mô tả không gian", Type: PortTypeText, Required: false},
		},
		Outputs: []FlowPort{
			{ID: "lighting_desc", Name: "Mô tả ánh sáng", Type: PortTypeText, Description: "Mô tả hiệu ứng ánh sáng & thời gian"},
			{ID: "lighting_prompt", Name: "Câu lệnh ánh sáng", Type: PortTypeText, Description: "Prompt ánh sáng hoàn chỉnh"},
			{ID: "prompt_out", Name: "Câu lệnh tổng hợp", Type: PortTypeText, Description: "Prompt đã chèn ánh sáng & thời gian"},
		},
		DefaultParams: map[string]any{
			"time_of_day":       "golden_hour",
			"color_temperature": "warm_golden",
			"light_intensity":   "soft_diffused",
			"prompt":            "",
		},
		Executor: executeLightingAtmosphere,
	})

	// 20. Camera Angles Director Node (Đạo diễn đa góc quay điện ảnh)
	RegisterNode(NodeDefinition{
		Type:        "camera_angles_director",
		Title:       "Đạo diễn đa góc quay (Multi-Angle Director)",
		Category:    CategoryCamera,
		Description: "Tạo ba prompt góc máy cho toàn thân, bán thân và chi tiết chất liệu; không tự render cảnh",
		Icon:        "video",
		Inputs: []FlowPort{
			{ID: "character_image", Name: "Ảnh nhân vật mặc trang phục", Type: PortTypeImage, Required: true},
			{ID: "product_title", Name: "Tên sản phẩm", Type: PortTypeText, Required: false},
			{ID: "character_name", Name: "Tên nhân vật", Type: PortTypeText, Required: false},
			{ID: "outfit_desc", Name: "Mô tả trang phục", Type: PortTypeText, Required: false},
			{ID: "environment_desc", Name: "Mô tả không gian", Type: PortTypeText, Required: false},
			{ID: "lighting_desc", Name: "Mô tả ánh sáng", Type: PortTypeText, Required: false},
		},
		Outputs: []FlowPort{
			{ID: "angle_full", Name: "Góc 1: Toàn thân Runway", Type: PortTypeText, Description: "Prompt góc toàn thân bước đi catwalk"},
			{ID: "angle_medium", Name: "Góc 2: Bán thân xoay dáng", Type: PortTypeText, Description: "Prompt góc bán thân xoay nhẹ khoe form"},
			{ID: "angle_detail", Name: "Góc 3: Cận cảnh chất vải", Type: PortTypeText, Description: "Prompt cận cảnh sợi vải satin ngọc trai"},
			{ID: "storyboard_prompts", Name: "Chuỗi 3 phân cảnh", Type: PortTypeTextList, Description: "Bộ ba prompt góc quay"},
			{ID: "subject_image", Name: "Ảnh tham chiếu", Type: PortTypeImage, Description: "Ảnh gốc cho node sinh cảnh"},
		},
		DefaultParams: map[string]any{
			"shot_style":             "high_fashion_editorial",
			"full_prompt_template":   "",
			"medium_prompt_template": "",
			"detail_prompt_template": "",
		},
		Executor: executeCameraAnglesDirector,
	})

	// Muse AI video generation
	RegisterNode(NodeDefinition{
		Type:        "muse_ai_neural_render",
		Title:       "Tạo video bằng Muse AI",
		Category:    CategoryRender,
		Description: "Gửi prompt và ảnh tham chiếu tới Muse Chat để tạo video. Đây không phải bộ dựng 3D/VFX.",
		Icon:        "cpu",
		Inputs: []FlowPort{
			{ID: "prompt", Name: "Câu lệnh kịch bản", Type: PortTypeText, Required: true},
			{ID: "character_ref", Name: "Ảnh tham chiếu", Type: PortTypeImage, Required: false},
			{ID: "warped_garment", Name: "Ảnh trang phục tham chiếu", Type: PortTypeImage, Required: false},
		},
		Outputs: []FlowPort{
			{ID: "rendered_video", Name: "Video tạo bằng Muse AI", Type: PortTypeVideo, Description: "File video Muse AI đã tạo"},
			{ID: "master_video", Name: "Master Video", Type: PortTypeVideo, Description: "Đường dẫn video master"},
			{ID: "sample_frame", Name: "Khung hình mẫu", Type: PortTypeImage, Description: "Ảnh đại diện trích xuất từ video"},
		},
		Executor: executeMuseAINeuralRender,
	})

	// Contrast and saturation adjustment
	RegisterNode(NodeDefinition{
		Type:        "cinematic_color_grading",
		Title:       "Cân chỉnh tương phản và bão hòa",
		Category:    CategoryPost,
		Description: "Dùng FFmpeg điều chỉnh tương phản và bão hòa video. Không áp dụng LUT.",
		Icon:        "feather",
		Inputs: []FlowPort{
			{ID: "video_in", Name: "Video đầu vào", Type: PortTypeVideo, Required: true},
		},
		Outputs: []FlowPort{
			{ID: "graded_video", Name: "Video đã cân chỉnh", Type: PortTypeVideo, Description: "Video sau điều chỉnh tương phản và bão hòa"},
			{ID: "video_out", Name: "Video đã cân chỉnh (video_out)", Type: PortTypeVideo, Description: "Video sau điều chỉnh tương phản và bão hòa"},
		},
		DefaultParams: map[string]any{
			"contrast_boost": 1.12,
			"saturation":     1.05,
		},
		Executor: executeCinematicColorGrading,
	})

	// 32. Video Master Output Node (FFmpeg export)
	RegisterNode(NodeDefinition{
		Type:        "video_master_output",
		Title:       "Xuất video TikTok (FFmpeg)",
		Category:    CategoryPost,
		Description: "Mã hóa video H.264 theo độ phân giải và FPS đã chọn; có thể ghép thêm một track âm thanh.",
		Icon:        "check-circle",
		Inputs: []FlowPort{
			{ID: "video_in", Name: "Video thành phẩm", Type: PortTypeVideo, Required: true},
			{ID: "audio_track", Name: "Track âm thanh", Type: PortTypeAudio, Required: false},
			{ID: "title", Name: "Tiêu đề video", Type: PortTypeText, Required: false},
		},
		Outputs: []FlowPort{
			{ID: "master_video", Name: "Video Master", Type: PortTypeVideo, Description: "File video Master chất lượng cao nhất"},
			{ID: "final_export", Name: "Đường dẫn xuất xưởng", Type: PortTypeVideo, Description: "Sẵn sàng đăng tải hoặc tải về"},
		},
		DefaultParams: map[string]any{
			"target_resolution": "1080x1920",
			"fps":               24,
		},
		Executor: executeVideoMasterOutput,
	})

	// 33. Preview Media Node (Xem trước ảnh / video / audio)
	RegisterNode(NodeDefinition{
		Type:        "preview_media",
		Title:       "Xem trước Media (Preview Image / Video / Audio)",
		Category:    CategoryOutput,
		Description: "Hiển thị trực tiếp kết quả ảnh, video, âm thanh hoặc mặt nạ trên Canvas ngay sau khi node chạy xong",
		Icon:        "eye",
		Inputs: []FlowPort{
			{ID: "media_in", Name: "Ảnh/Video/Audio cần xem", Type: PortTypeAny, Required: true},
		},
		Outputs: []FlowPort{
			{ID: "media_out", Name: "Truyền tiếp (Passthrough)", Type: PortTypeAny, Description: "Dữ liệu media truyền tiếp sang các node khác"},
		},
		DefaultParams: map[string]any{},
		Executor:      executePreviewMedia,
	})

	// 34. Preview Image Node (Legacy alias gộp vào preview_media)
	RegisterNode(NodeDefinition{
		Type:        "preview_image",
		Title:       "Xem trước ảnh (Preview Image)",
		Category:    CategoryOutput,
		Description: "Hiển thị ảnh hoặc mặt nạ kết quả trực tiếp trên Canvas",
		Icon:        "eye",
		Hidden:      true,
		Inputs: []FlowPort{
			{ID: "images", Name: "Ảnh cần xem", Type: PortTypeAny, Required: true},
		},
		Outputs: []FlowPort{
			{ID: "images", Name: "Truyền tiếp", Type: PortTypeAny, Description: "Dữ liệu ảnh truyền tiếp"},
		},
		DefaultParams: map[string]any{},
		Executor:      executePreviewMedia,
	})

	// 34b. Preview Video Node (Legacy alias gộp vào preview_media)
	RegisterNode(NodeDefinition{
		Type:        "preview_video",
		Title:       "Xem trước video (Preview Video)",
		Category:    CategoryOutput,
		Description: "Hiển thị video kết quả trực tiếp trên Canvas",
		Icon:        "eye",
		Hidden:      true,
		Inputs: []FlowPort{
			{ID: "video_in", Name: "Video cần xem", Type: PortTypeAny, Required: true},
		},
		Outputs: []FlowPort{
			{ID: "video_out", Name: "Truyền tiếp", Type: PortTypeAny, Description: "Dữ liệu video truyền tiếp"},
		},
		DefaultParams: map[string]any{},
		Executor:      executePreviewMedia,
	})

	// 35. Custom AI Prompt Node (Legacy alias gộp vào custom_universal_ai)
	RegisterNode(NodeDefinition{
		Type:        "custom_ai_prompt",
		Title:       "Node Custom (AI Prompt / LLM)",
		Category:    CategoryCustom,
		Description: "Tạo văn bản bằng LLM đang cấu hình, ảnh bằng image generator hoặc video qua Google Flow/Muse AI",
		Icon:        "sparkles",
		Hidden:      true,
		Inputs: []FlowPort{
			{ID: "input", Name: "Dữ liệu nguồn (Input)", Type: PortTypeAny, Required: false},
			{ID: "image", Name: "Ảnh tham chiếu (Image)", Type: PortTypeImage, Required: false},
			{ID: "video", Name: "Video nguồn (Video)", Type: PortTypeVideo, Required: false},
		},
		Outputs: []FlowPort{
			{ID: "output", Name: "Kết quả (Output)", Type: PortTypeAny, Description: "Dữ liệu xuất đa năng"},
			{ID: "text", Name: "Văn bản tạo ra (Text)", Type: PortTypeText, Description: "Nội dung phản hồi hoặc kịch bản AI"},
			{ID: "media", Name: "Tệp media (Video/Image)", Type: PortTypeAny, Description: "File video hoặc ảnh đã tạo"},
		},
		DefaultParams: map[string]any{
			"model":         "gemini-3.8-flash",
			"provider":      "googleflow",
			"system_prompt": "Bạn là chuyên gia sáng tạo nội dung AI chuyên nghiệp...",
			"prompt":        "Tạo nội dung hấp dẫn dựa trên đầu vào: {{input}}",
			"output_type":   "text",
		},
		Executor: executeCustomAIPrompt,
	})

	// 36. Universal AI node: text/image/video input, text/image/video output.
	RegisterNode(NodeDefinition{
		Type:        "custom_universal_ai",
		Title:       "AI Đa Năng Vạn Năng (Universal AI Node)",
		Category:    CategoryCustom,
		Description: "Tạo text, ảnh hoặc video qua các provider đã cấu hình (Gemini 3.8 Flash, ChatGPT GPT-6 Luna, Claude 5.5 Sonnet, Veo 2, Muse AI).",
		Icon:        "cpu",
		Inputs: []FlowPort{
			{ID: "text_in", Name: "Văn bản / Kịch bản (Text)", Type: PortTypeText, Required: false},
			{ID: "image_in", Name: "Ảnh đầu vào (Image)", Type: PortTypeImage, Required: false},
			{ID: "video_in", Name: "Video đầu vào (Video)", Type: PortTypeVideo, Required: false},
			{ID: "data_in", Name: "Dữ liệu cấu hình (Data)", Type: PortTypeAny, Required: false},
		},
		Outputs: []FlowPort{
			{ID: "result_out", Name: "Kết quả chính (Primary Output)", Type: PortTypeAny, Description: "Kết quả hoàn chỉnh (Media hoặc Text)"},
			{ID: "text_out", Name: "Văn bản xuất (Text)", Type: PortTypeText, Description: "Phản hồi văn bản từ AI"},
			{ID: "image_out", Name: "Ảnh sinh ra (Image)", Type: PortTypeImage, Description: "File ảnh AI tạo ra"},
			{ID: "video_out", Name: "Video sinh ra (Video)", Type: PortTypeVideo, Description: "File video AI tạo ra"},
		},
		DefaultParams: map[string]any{
			"model":         "gemini-3.8-flash",
			"system_prompt": "Bạn là chuyên gia AI đa năng cao cấp xử lý mọi yêu cầu sáng tạo nội dung.",
			"prompt":        "Xử lý và tạo nội dung chất lượng cao: {{text_in}}",
			"output_type":   "auto",
		},
		Executor: executeCustomUniversalAI,
	})

	// 37. Custom Media Input Node (Tải lên hoặc dán link bất kỳ media nào)
	RegisterNode(NodeDefinition{
		Type:        "custom_media_input",
		Title:       "Đầu vào Media Tùy Biến (Custom Media Input)",
		Category:    CategoryCustom,
		Description: "Nhận ảnh, video hoặc audio từ file tải lên, đường dẫn cục bộ hoặc URL media trực tiếp",
		Icon:        "folder-plus",
		Inputs:      []FlowPort{},
		Outputs: []FlowPort{
			{ID: "media_out", Name: "Media đầu ra", Type: PortTypeAny, Description: "Tệp media đa năng"},
			{ID: "image_out", Name: "Ảnh trích xuất", Type: PortTypeImage, Description: "Đầu ra dạng ảnh"},
			{ID: "video_out", Name: "Video đầu ra", Type: PortTypeVideo, Description: "Đầu ra dạng video"},
			{ID: "audio_out", Name: "Âm thanh", Type: PortTypeAudio, Description: "Đầu ra dạng âm thanh"},
			{ID: "url_out", Name: "URL nguồn", Type: PortTypeText, Description: "Đường link đã nạp"},
		},
		DefaultParams: map[string]any{
			"media_title": "Tài nguyên tùy biến",
			"media_url":   "",
			"media_path":  "",
		},
		Executor: executeCustomMediaInput,
	})

	// 38. Custom Vision & Video Analyzer Node
	RegisterNode(NodeDefinition{
		Type:        "custom_vision_analyzer",
		Title:       "Phân tích Thị giác & Video (Vision Analyzer)",
		Category:    CategoryCustom,
		Description: "Phân tích ảnh hoặc một frame video bằng Vision AI đã cấu hình; tạo prompt tái hiện dựa trên kết quả thật",
		Icon:        "eye",
		Inputs: []FlowPort{
			{ID: "image_in", Name: "Ảnh cần phân tích", Type: PortTypeImage, Required: false},
			{ID: "video_in", Name: "Video cần phân tích", Type: PortTypeVideo, Required: false},
			{ID: "prompt_in", Name: "Chỉ thị bổ sung", Type: PortTypeText, Required: false},
		},
		Outputs: []FlowPort{
			{ID: "analysis_text", Name: "Bản phân tích chi tiết", Type: PortTypeText, Description: "Nội dung phân tích trực quan sâu"},
			{ID: "prompt_out", Name: "Prompt ngược tối ưu", Type: PortTypeText, Description: "Prompt tái tạo bằng AI"},
			{ID: "detected_data", Name: "Dữ liệu cấu trúc", Type: PortTypeAny, Description: "Chi tiết đối tượng nhận diện"},
		},
		DefaultParams: map[string]any{
			"analysis_task": "comprehensive_breakdown",
		},
		Executor: executeCustomVisionAnalyzer,
	})

	// 39. Custom Media Processor Node (Biến đổi media bằng FFmpeg)
	RegisterNode(NodeDefinition{
		Type:        "custom_media_processor",
		Title:       "Xử lý & Biến đổi Media (Media Processor)",
		Category:    CategoryCustom,
		Description: "Dùng FFmpeg để đổi tỷ lệ khung hình hoặc áp dụng bộ lọc màu đã hỗ trợ trên ảnh/video",
		Icon:        "sliders",
		Inputs: []FlowPort{
			{ID: "media_in", Name: "Media đầu vào", Type: PortTypeAny, Required: true},
		},
		Outputs: []FlowPort{
			{ID: "media_out", Name: "Media thành phẩm", Type: PortTypeAny, Description: "File sau khi xử lý"},
			{ID: "video_out", Name: "Video thành phẩm", Type: PortTypeVideo, Description: "Đầu ra dạng video"},
			{ID: "image_out", Name: "Ảnh thành phẩm", Type: PortTypeImage, Description: "Đầu ra dạng ảnh"},
		},
		DefaultParams: map[string]any{
			"operation":     "resize_aspect",
			"aspect_ratio":  "9:16",
			"filter_preset": "cinematic_vibrant",
		},
		Executor: executeCustomMediaProcessor,
	})

	// 40. Text template node; does not execute arbitrary scripts.
	RegisterNode(NodeDefinition{
		Type:        "custom_script_runner",
		Title:       "Ghép template văn bản",
		Category:    CategoryCustom,
		Description: "Thay {{input_1}}, {{input_2}} và {{text_in}} trong template văn bản",
		Icon:        "code",
		Inputs: []FlowPort{
			{ID: "input_1", Name: "Đầu vào 1", Type: PortTypeAny, Required: false},
			{ID: "input_2", Name: "Đầu vào 2", Type: PortTypeAny, Required: false},
			{ID: "text_in", Name: "Văn bản (Text)", Type: PortTypeText, Required: false},
		},
		Outputs: []FlowPort{
			{ID: "output", Name: "Kết quả logic", Type: PortTypeAny, Description: "Dữ liệu sau khi xử lý"},
			{ID: "result_text", Name: "Văn bản kết quả", Type: PortTypeText, Description: "Chuỗi ký tự thành phẩm"},
			{ID: "result_data", Name: "Dữ liệu cấu trúc", Type: PortTypeConfig, Description: "Dữ liệu metadata"},
		},
		DefaultParams: map[string]any{
			"template": "{{input_1}} - {{input_2}}",
		},
		Executor: executeCustomScriptRunner,
	})

	// 41. Custom Webhook & API Node (Kết nối mọi dịch vụ trên thế giới)
	RegisterNode(NodeDefinition{
		Type:        "custom_api_webhook",
		Title:       "Kết nối API & Webhook (Webhook Node)",
		Category:    CategoryCustom,
		Description: "Gửi JSON request qua HTTP(S) và trả body/status từ endpoint cấu hình",
		Icon:        "send",
		Inputs: []FlowPort{
			{ID: "payload_in", Name: "Dữ liệu gửi (Payload)", Type: PortTypeAny, Required: false},
			{ID: "message_text", Name: "Tin nhắn gửi (Text)", Type: PortTypeText, Required: false},
		},
		Outputs: []FlowPort{
			{ID: "response_data", Name: "Dữ liệu phản hồi", Type: PortTypeAny, Description: "JSON phản hồi từ máy chủ"},
			{ID: "status_code", Name: "Mã trạng thái HTTP", Type: PortTypeNumber, Description: "Mã phản hồi HTTP (200, 201,...)"},
			{ID: "response_text", Name: "Văn bản phản hồi", Type: PortTypeText, Description: "Body dạng text"},
		},
		DefaultParams: map[string]any{
			"method":      "POST",
			"webhook_url": "",
			"auth_token":  "",
		},
		Executor: executeCustomAPIWebhook,
	})

	// 42. Custom export node. Publishing requires a separate consent flow.
	RegisterNode(NodeDefinition{
		Type:        "custom_export_publish",
		Title:       "Xuất file media",
		Category:    CategoryCustom,
		Description: "Sao chép media gốc vào thư mục output và tạo đường dẫn tải; không đăng lên nền tảng",
		Icon:        "download",
		Inputs: []FlowPort{
			{ID: "media_in", Name: "Media thành phẩm", Type: PortTypeAny, Required: true},
			{ID: "title", Name: "Tiêu đề video/ảnh", Type: PortTypeText, Required: false},
		},
		Outputs: []FlowPort{
			{ID: "final_file", Name: "Tệp thành phẩm", Type: PortTypeAny, Description: "Đường dẫn tệp trên hệ thống"},
			{ID: "file_url", Name: "Đường dẫn tải về (URL)", Type: PortTypeText, Description: "Link tải về trình duyệt"},
		},
		DefaultParams: map[string]any{
			"title":           "Thành phẩm xuất xưởng",
			"filename_prefix": "aicos_export",
		},
		Executor: executeCustomExportPublish,
	})

	RegisterNode(NodeDefinition{
		Type:        "workflow_input",
		Title:       "Đầu vào workflow",
		Category:    CategoryWorkflow,
		Description: "Khai báo input có tên để workflow khác truyền dữ liệu vào",
		Icon:        "log-in",
		Outputs:     []FlowPort{{ID: "value", Name: "Giá trị", Type: PortTypeAny}},
		DefaultParams: map[string]any{
			"key": "input", "label": "Input", "value_type": string(PortTypeText), "required": true, "value": "",
		},
		Executor: executeWorkflowInput,
	})
	RegisterNode(NodeDefinition{
		Type:        "workflow_output",
		Title:       "Đầu ra workflow",
		Category:    CategoryWorkflow,
		Description: "Khai báo output có tên để workflow khác nhận kết quả",
		Icon:        "log-out",
		Inputs:      []FlowPort{{ID: "value", Name: "Giá trị", Type: PortTypeAny, Required: true}},
		Outputs:     []FlowPort{{ID: "value", Name: "Giá trị", Type: PortTypeAny}},
		DefaultParams: map[string]any{
			"key": "output", "label": "Output", "value_type": string(PortTypeAny),
		},
		Executor: executeWorkflowOutput,
	})
	RegisterNode(NodeDefinition{
		Type:        "workflow_call",
		Title:       "Gọi workflow đã lưu",
		Category:    CategoryWorkflow,
		Description: "Gọi workflow đã lưu có ít nhất một node Đầu vào và một node Đầu ra; truyền input và nhận output theo contract",
		Icon:        "workflow",
		DefaultParams: map[string]any{
			"workflow_id": "",
		},
		Executor: executeWorkflowCall,
	})
	RegisterNode(NodeDefinition{
		Type:        "workflow_map",
		Title:       "Lặp qua danh sách (Workflow Map)",
		Category:    CategoryWorkflow,
		Description: "Chạy workflow con theo thứ tự; tùy chọn dùng chung Muse thread và frame shot trước để giữ mạch nội dung, outfit giữa các lần lặp.",
		Icon:        "repeat-2",
		Inputs: []FlowPort{
			{ID: "items", Name: "Danh sách cần xử lý", Type: PortTypeAny, Required: true},
			{ID: "context", Name: "Context dùng chung", Type: PortTypeConfig, Required: false},
			{ID: "reference_images", Name: "Ảnh tham chiếu dùng chung", Type: PortTypeImageList, Required: false},
			{ID: "reference_prefix_count", Name: "Số ảnh danh tính đứng trước frame liên tục", Type: PortTypeNumber, Required: false},
		},
		Outputs: []FlowPort{{ID: "results", Name: "Kết quả theo thứ tự", Type: PortTypeAny}},
		DefaultParams: map[string]any{
			"workflow_id":                                        "",
			"item_input_key":                                     "",
			"item_input_map":                                     map[string]any{},
			"shared_image_input_key":                             "",
			"merge_context_into_item":                            false,
			"share_muse_thread":                                  false,
			"previous_video_frame_input_key":                     "",
			"previous_video_frame_insert_at":                     -1,
			"previous_video_frame_insert_after_reference_prefix": false,
			"previous_video_frame_variables_key":                 "variables",
			"index_input_key":                                    "",
			"result_output_key":                                  "",
			"max_items":                                          20,
		},
		Executor: executeWorkflowMap,
	})
	for _, category := range []struct{ key, title string }{
		{"affiliate", "Affiliate"},
		{"viral", "TikTok Viral"},
		{"fashion", "Thời trang"},
		{"custom", "Custom"},
	} {
		registerWorkflowCallNode(category.key, category.title)
	}
}

func registerWorkflowCallNode(category, title string) {
	RegisterNode(NodeDefinition{
		Type:        "workflow_call_" + category,
		Title:       "Workflow con · " + title,
		Category:    CategoryWorkflow,
		Description: "Chạy workflow " + title + " đã lưu có node Đầu vào và Đầu ra, truyền input và nhận output theo contract",
		Icon:        "workflow",
		DefaultParams: map[string]any{
			"workflow_id":       "",
			"workflow_category": category,
		},
		Executor: executeWorkflowCall,
	})
}

func nodeParamString(params map[string]any, key, fallback string) string {
	if params == nil {
		return fallback
	}
	if val, ok := params[key]; ok {
		if s, ok := val.(string); ok && s != "" {
			return s
		}
	}
	return fallback
}

func nodeParamInt(params map[string]any, key string, fallback int) int {
	if params == nil {
		return fallback
	}
	if val, ok := params[key]; ok {
		switch v := val.(type) {
		case int:
			return v
		case float64:
			return int(v)
		case int64:
			return int(v)
		}
	}
	return fallback
}

func nodeParamFloat(params map[string]any, key string, fallback float64) float64 {
	if params == nil {
		return fallback
	}
	if val, ok := params[key]; ok {
		switch v := val.(type) {
		case float64:
			return v
		case int:
			return float64(v)
		case int64:
			return float64(v)
		}
	}
	return fallback
}

func nodeParamBool(params map[string]any, key string, fallback bool) bool {
	if params == nil {
		return fallback
	}
	if val, ok := params[key]; ok {
		if b, ok := val.(bool); ok {
			return b
		}
	}
	return fallback
}
