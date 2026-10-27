package flow

import (
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
		Description: "Chọn nhân vật mẫu từ thư viện kênh đã lưu hoặc tải ảnh nhân vật lên",
		Icon:        "user",
		Inputs:      []FlowPort{},
		Outputs: []FlowPort{
			{ID: "character_image", Name: "Ảnh nhân vật", Type: PortTypeImage, Description: "Ảnh khuôn mặt / chân dung nhân vật đại diện"},
			{ID: "character_images", Name: "Danh sách ảnh", Type: PortTypeImageList, Description: "Tập hợp các góc mặt của nhân vật mẫu"},
			{ID: "character_name", Name: "Tên nhân vật", Type: PortTypeText, Description: "Tên hoặc mô tả ngắn của nhân vật"},
		},
		DefaultParams: map[string]any{
			"account_id": 0,
			"model_path": "",
			"model_name": "Miu Character",
			"gender":     "female",
		},
		Executor: executeCharacterInput,
	})

	// 2. Product Input Node
	RegisterNode(NodeDefinition{
		Type:        "product_input",
		Title:       "Sản phẩm mẫu (Product)",
		Category:    CategoryInput,
		Description: "Chọn sản phẩm tiếp thị liên kết (Accesstrade / Shopee / TikTok Shop) hoặc tải ảnh sản phẩm",
		Icon:        "package",
		Inputs:      []FlowPort{},
		Outputs: []FlowPort{
			{ID: "product_image", Name: "Ảnh sản phẩm", Type: PortTypeImage, Description: "Ảnh gốc của sản phẩm cần tiếp thị"},
			{ID: "product_title", Name: "Tên sản phẩm", Type: PortTypeText, Description: "Tiêu đề sản phẩm"},
			{ID: "product_info", Name: "Dữ liệu sản phẩm", Type: PortTypeProduct, Description: "Chi tiết giá, link và hoa hồng sản phẩm"},
		},
		DefaultParams: map[string]any{
			"product_id":    "",
			"product_image": "",
			"product_title": "Sản phẩm xu hướng",
			"affiliate_url": "",
		},
		Executor: executeProductInput,
	})

	// 3. Video Source Node
	RegisterNode(NodeDefinition{
		Type:        "video_source",
		Title:       "Video mẫu gốc (Radar Video)",
		Category:    CategoryInput,
		Description: "Chọn video viral từ TikTok Radar xu hướng hoặc file video mp4 mẫu",
		Icon:        "video",
		Inputs:      []FlowPort{},
		Outputs: []FlowPort{
			{ID: "video_path", Name: "Video nguồn", Type: PortTypeVideo, Description: "File video mp4 đầy đủ của trend nguồn"},
			{ID: "audio_track", Name: "Âm thanh gốc", Type: PortTypeAudio, Description: "Track âm thanh / nhạc nền gốc từ video"},
			{ID: "duration", Name: "Thời lượng (s)", Type: PortTypeNumber, Description: "Độ dài video nguồn tính theo giây"},
		},
		DefaultParams: map[string]any{
			"video_url":  "",
			"video_path": "",
			"video_id":   "",
			"author":     "",
		},
		Executor: executeVideoSource,
	})

	// 4. Audio Source Node
	RegisterNode(NodeDefinition{
		Type:        "audio_source",
		Title:       "Âm thanh / Nhạc nền (Audio)",
		Category:    CategoryInput,
		Description: "Nhạc nền trending, file voiceover lồng tiếng hoặc nhạc nền tùy chọn",
		Icon:        "music",
		Inputs:      []FlowPort{},
		Outputs: []FlowPort{
			{ID: "audio_file", Name: "Track âm thanh", Type: PortTypeAudio, Description: "File âm thanh mp3/aac/wav"},
			{ID: "audio_title", Name: "Tên bài nhạc", Type: PortTypeText, Description: "Tiêu đề âm thanh"},
		},
		DefaultParams: map[string]any{
			"audio_path":  "",
			"audio_title": "Nhạc nền thịnh hành",
		},
		Executor: executeAudioSource,
	})

	// 5. Prompt Template Node
	RegisterNode(NodeDefinition{
		Type:        "prompt_template",
		Title:       "Tạo câu lệnh (Prompt Template)",
		Category:    CategoryProcess,
		Description: "Ghép các biến (nhân vật, sản phẩm, hành động, phong cách) thành prompt tối ưu cho AI",
		Icon:        "edit-3",
		Inputs: []FlowPort{
			{ID: "character_desc", Name: "Mô tả nhân vật", Type: PortTypeText, Required: false},
			{ID: "product_desc", Name: "Mô tả sản phẩm", Type: PortTypeText, Required: false},
			{ID: "motion_desc", Name: "Mô tả chuyển động", Type: PortTypeText, Required: false},
		},
		Outputs: []FlowPort{
			{ID: "prompt", Name: "Câu lệnh hoàn chỉnh", Type: PortTypeText, Description: "Prompt chuẩn hóa đã chèn các biến"},
		},
		DefaultParams: map[string]any{
			"template":    "Cinematic vertical 9:16 video of {character} {motion}, showcasing {product}. Beautiful soft lighting, 4k ultra realistic, smooth motion.",
			"visual_mood": "cinematic",
			"aspect":      "9:16",
		},
		Executor: executePromptTemplate,
	})

	// 6. Product Extractor Node (Bóc tách sản phẩm)
	RegisterNode(NodeDefinition{
		Type:        "product_extractor",
		Title:       "Bóc tách sản phẩm (Extract Product)",
		Category:    CategoryProcess,
		Description: "Bóc tách sản phẩm từ ảnh mẫu hoặc video TikTok/Reels/IG, tự động trích xuất khung hình và xóa phông nền (cutout PNG)",
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
			"video_timestamp":     "auto",
			"refine_edges":        true,
			"auto_crop":           true,
			"segmentation_engine": "auto",
		},
		Executor: executeProductExtractor,
	})

	// 7. Image Composite Node (Ghép nối sản phẩm với nhân vật mẫu)
	RegisterNode(NodeDefinition{
		Type:        "image_composite",
		Title:       "Ghép sản phẩm với nhân vật (Composite)",
		Category:    CategoryProcess,
		Description: "Ghép ảnh sản phẩm đã bóc tách vào tay hoặc bên cạnh nhân vật mẫu, điều chỉnh tỉ lệ và ánh sáng",
		Icon:        "layers",
		Inputs: []FlowPort{
			{ID: "character_image", Name: "Ảnh nhân vật", Type: PortTypeImage, Required: true},
			{ID: "product_image", Name: "Ảnh sản phẩm", Type: PortTypeImage, Required: true},
		},
		Outputs: []FlowPort{
			{ID: "composite_image", Name: "Ảnh ghép hoàn chỉnh", Type: PortTypeImage, Description: "Nhân vật mẫu đang cầm/trưng bày sản phẩm"},
		},
		DefaultParams: map[string]any{
			"placement":  "holding_hand", // holding_hand, beside, wearing, foreground
			"scale":      0.45,
			"blend_mode": "natural",
		},
		Executor: executeImageComposite,
	})

	// 8. Dance Motion Extractor Node (Bóc tách điệu nhảy từ video gốc)
	RegisterNode(NodeDefinition{
		Type:        "dance_motion_extractor",
		Title:       "Bóc tách điệu nhảy (Extract Dance)",
		Category:    CategoryProcess,
		Description: "Phân tích video TikTok gốc để bóc tách nhịp điệu (BPM), các frame vũ đạo then chốt và chuỗi chuyển động",
		Icon:        "activity",
		Inputs: []FlowPort{
			{ID: "video_in", Name: "Video gốc", Type: PortTypeVideo, Required: true},
		},
		Outputs: []FlowPort{
			{ID: "motion_data", Name: "Dữ liệu vũ đạo", Type: PortTypeMotion, Description: "Phân đoạn vũ đạo, nhịp beat và pose timing"},
			{ID: "keyframes", Name: "Keyframe vũ đạo", Type: PortTypeImageList, Description: "Các frame tư thế chính trích xuất từ video"},
			{ID: "beat_audio", Name: "Âm thanh trích xuất", Type: PortTypeAudio, Description: "Âm thanh đã đồng bộ nhịp"},
		},
		DefaultParams: map[string]any{
			"segment_duration": 6.9,
			"extract_poses":    true,
		},
		Executor: executeDanceMotionExtractor,
	})

	// 9. Dance Retarget Node (Ghép nối điệu nhảy, biểu cảm vào nhân vật)
	RegisterNode(NodeDefinition{
		Type:        "dance_retarget",
		Title:       "Ghép vũ đạo vào nhân vật (Retarget)",
		Category:    CategoryProcess,
		Description: "Gắn kết điệu nhảy, cử chỉ và biểu cảm từ video gốc vào khuôn mặt và thân hình của nhân vật mẫu",
		Icon:        "shuffle",
		Inputs: []FlowPort{
			{ID: "character_image", Name: "Ảnh nhân vật mẫu", Type: PortTypeImage, Required: true},
			{ID: "motion_data", Name: "Dữ liệu vũ đạo", Type: PortTypeMotion, Required: true},
		},
		Outputs: []FlowPort{
			{ID: "prompt_choreography", Name: "Prompt vũ đạo", Type: PortTypeText, Description: "Mô tả chuyển động nghệ thuật an toàn cho AI"},
			{ID: "ingredients", Name: "Thành phần chuẩn bị", Type: PortTypeImageList, Description: "Các ảnh nguyên liệu cho bộ tạo video"},
		},
		DefaultParams: map[string]any{
			"expression": "expressive_smile", // expressive_smile, energetic, confident, cool
			"body_type":  "preserve_model",
		},
		Executor: executeDanceRetarget,
	})

	// 10. Google Flow Video Node
	RegisterNode(NodeDefinition{
		Type:        "google_flow_video",
		Title:       "Google Flow Veo 2 (Web AI Video)",
		Category:    CategoryAI,
		Description: "Tạo video AI chất lượng cao bằng tài khoản web Google Flow đã lưu trên hệ thống",
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
			"mode":   "ingredients",
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
		},
		Outputs: []FlowPort{
			{ID: "video_out", Name: "Video từ Muse AI", Type: PortTypeVideo, Description: "Video hoàn chỉnh sinh bởi Muse AI"},
		},
		DefaultParams: map[string]any{
			"aspect": "9:16",
			"style":  "cinematic",
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
		DefaultParams: map[string]any{
			"mode": "auto_prompt", // auto_prompt, product_breakdown, style_analysis
		},
		Executor: executeGeminiVision,
	})

	// 13. Custom Web AI Node (Node AI tùy biến mở rộng)
	RegisterNode(NodeDefinition{
		Type:        "custom_web_ai",
		Title:       "Node Web AI tùy biến (Custom AI)",
		Category:    CategoryAI,
		Description: "Mở rộng và kết nối với bất kỳ dịch vụ Web AI nào (ChatGPT, Claude, Grok, Midjourney, v.v.)",
		Icon:        "cpu",
		Inputs: []FlowPort{
			{ID: "prompt_in", Name: "Prompt đầu vào", Type: PortTypeText, Required: true},
			{ID: "asset_in", Name: "File đầu vào", Type: PortTypeAny, Required: false},
		},
		Outputs: []FlowPort{
			{ID: "result_out", Name: "Kết quả xuất", Type: PortTypeAny, Description: "File video hoặc ảnh từ Custom AI"},
			{ID: "text_out", Name: "Văn bản phản hồi", Type: PortTypeText, Description: "Nội dung phản hồi dạng text"},
		},
		DefaultParams: map[string]any{
			"provider_name": "ChatGPT / Claude / Grok",
			"target_url":    "https://grok.com/",
			"output_type":   "video", // video, image, text
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
			{ID: "audio_in", Name: "Âm thanh gốc", Type: PortTypeAudio, Required: true},
		},
		Outputs: []FlowPort{
			{ID: "video_out", Name: "Video có âm thanh", Type: PortTypeVideo, Description: "Video mp4 hoàn chỉnh đồng bộ tiếng và hình"},
		},
		DefaultParams: map[string]any{
			"audio_codec": "aac",
			"keep_length": "shortest",
		},
		Executor: executeAudioMerge,
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
		},
		Outputs: []FlowPort{
			{ID: "video_out", Name: "Video nét cao (4K)", Type: PortTypeVideo, Description: "Video master sau khi upscale"},
		},
		DefaultParams: map[string]any{
			"resolution": "4k", // 1080p, 4k
			"crf":        18,
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
			{ID: "scene_1", Name: "Phân cảnh 1", Type: PortTypeVideo, Required: true},
			{ID: "scene_2", Name: "Phân cảnh 2", Type: PortTypeVideo, Required: false},
			{ID: "scene_3", Name: "Phân cảnh 3", Type: PortTypeVideo, Required: false},
		},
		Outputs: []FlowPort{
			{ID: "master_video", Name: "Video Master", Type: PortTypeVideo, Description: "Video hoàn chỉnh nối liền mạch các cảnh"},
		},
		DefaultParams: map[string]any{
			"transition": "none",
		},
		Executor: executeConcatScenes,
	})

	// 17. Video Output Node
	RegisterNode(NodeDefinition{
		Type:        "video_output",
		Title:       "Xuất video TikTok (Video Output)",
		Category:    CategoryOutput,
		Description: "Xuất file video thành phẩm chuẩn dọc 9:16, xem trực tiếp, tải về hoặc chuyển vào hàng đợi đăng TikTok",
		Icon:        "check-circle",
		Inputs: []FlowPort{
			{ID: "video_in", Name: "Video thành phẩm", Type: PortTypeVideo, Required: true},
			{ID: "title", Name: "Tiêu đề video", Type: PortTypeText, Required: false},
		},
		Outputs: []FlowPort{
			{ID: "final_video", Name: "Video cuối cùng", Type: PortTypeVideo, Description: "Đường dẫn file video master sẵn sàng xuất bản"},
		},
		DefaultParams: map[string]any{
			"auto_publish_draft": false,
		},
		Executor: executeVideoOutput,
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

	// 19. Virtual Try-On Node (Thay đổi trang phục thời trang)
	RegisterNode(NodeDefinition{
		Type:        "virtual_tryon",
		Title:       "Thay đổi trang phục AI (Virtual Try-on)",
		Category:    CategoryProcess,
		Description: "Hoán đổi trang phục thời trang nữ sang đầm/váy/outfit mới, bảo toàn khuôn mặt và form dáng nhân vật mẫu",
		Icon:        "shirt",
		Inputs: []FlowPort{
			{ID: "character_image", Name: "Ảnh nhân vật mẫu", Type: PortTypeImage, Required: true},
			{ID: "garment_image", Name: "Ảnh trang phục", Type: PortTypeImage, Required: true},
		},
		Outputs: []FlowPort{
			{ID: "fitted_image", Name: "Ảnh sau khi thay đồ", Type: PortTypeImage, Description: "Nhân vật mẫu mặc trang phục mới hoàn chỉnh"},
			{ID: "mask_image", Name: "Mặt nạ trang phục", Type: PortTypeImage, Description: "Vùng phân đoạn trang phục"},
		},
		DefaultParams: map[string]any{
			"fit_style":     "seamless_luxury",
			"preserve_face": true,
		},
		Executor: executeVirtualTryOn,
	})

	// 20. Camera Angles Director Node (Đạo diễn đa góc quay điện ảnh)
	RegisterNode(NodeDefinition{
		Type:        "camera_angles_director",
		Title:       "Đạo diễn đa góc quay (Multi-Angle Director)",
		Category:    CategoryCamera,
		Description: "Tự động thiết lập chuỗi 3 góc máy điện ảnh: toàn thân runway walk, bán thân xoay 360 dáng và cận cảnh chất vải",
		Icon:        "video",
		Inputs: []FlowPort{
			{ID: "character_image", Name: "Ảnh nhân vật mặc trang phục", Type: PortTypeImage, Required: true},
		},
		Outputs: []FlowPort{
			{ID: "angle_full", Name: "Góc 1: Toàn thân Runway", Type: PortTypeText, Description: "Prompt góc toàn thân bước đi catwalk"},
			{ID: "angle_medium", Name: "Góc 2: Bán thân xoay dáng", Type: PortTypeText, Description: "Prompt góc bán thân xoay nhẹ khoe form"},
			{ID: "angle_detail", Name: "Góc 3: Cận cảnh chất vải", Type: PortTypeText, Description: "Prompt cận cảnh sợi vải satin ngọc trai"},
			{ID: "storyboard_prompts", Name: "Chuỗi 3 phân cảnh", Type: PortTypeImageList, Description: "Bộ 3 prompt cho các phân cảnh"},
		},
		DefaultParams: map[string]any{
			"shot_style": "high_fashion_editorial",
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
		DefaultParams: map[string]any{
			"aspect": "9:16",
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

	// 33. Preview Media Node (Xem trước kết quả ComfyUI)
	RegisterNode(NodeDefinition{
		Type:        "preview_media",
		Title:       "Xem trước kết quả (Preview Image/Media)",
		Category:    CategoryOutput,
		Description: "Hiển thị trực tiếp kết quả ảnh, mặt nạ hoặc video trên Canvas ngay sau khi node chạy xong (tương tự Preview Image trong ComfyUI)",
		Icon:        "eye",
		Inputs: []FlowPort{
			{ID: "media_in", Name: "Ảnh/Video cần xem", Type: PortTypeAny, Required: true},
		},
		Outputs: []FlowPort{
			{ID: "media_out", Name: "Truyền tiếp (Passthrough)", Type: PortTypeAny, Description: "Dữ liệu media truyền tiếp sang các node khác"},
		},
		DefaultParams: map[string]any{},
		Executor:      executePreviewMedia,
	})

	// 34. Preview Image Node (Alias tương thích ComfyUI chuẩn)
	RegisterNode(NodeDefinition{
		Type:        "preview_image",
		Title:       "Xem trước ảnh (Preview Image)",
		Category:    CategoryOutput,
		Description: "Hiển thị ảnh hoặc mặt nạ kết quả trực tiếp trên Canvas (ComfyUI Preview Image)",
		Icon:        "eye",
		Inputs: []FlowPort{
			{ID: "images", Name: "Ảnh cần xem", Type: PortTypeAny, Required: true},
		},
		Outputs: []FlowPort{
			{ID: "images", Name: "Truyền tiếp", Type: PortTypeAny, Description: "Dữ liệu ảnh truyền tiếp"},
		},
		DefaultParams: map[string]any{},
		Executor:      executePreviewMedia,
	})

	// 35. Custom AI Prompt Node (n8n & ComfyUI Custom Node)
	RegisterNode(NodeDefinition{
		Type:        "custom_ai_prompt",
		Title:       "Node Custom (AI Prompt / LLM)",
		Category:    CategoryAI,
		Description: "Node AI tự do tùy biến: tự đặt tên, viết prompt, chọn model Muse AI, Gemini, ChatGPT, Claude, Grok",
		Icon:        "sparkles",
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
			"node_name":     "Custom AI Node",
			"model":         "gemini",
			"provider":      "gemini",
			"system_prompt": "Bạn là chuyên gia sáng tạo nội dung AI chuyên nghiệp...",
			"prompt":        "Tạo nội dung hấp dẫn dựa trên đầu vào: {{input}}",
			"output_type":   "text",
			"temperature":   0.7,
		},
		Executor: executeCustomAIPrompt,
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
		Description: "Chọn workflow con bất kỳ, truyền input và nhận output đã khai báo",
		Icon:        "workflow",
		DefaultParams: map[string]any{
			"workflow_id": "",
		},
		Executor: executeWorkflowCall,
	})
	for _, category := range []struct{ key, title string }{
		{"affiliate", "Affiliate"},
		{"viral", "TikTok Viral"},
		{"fashion", "Thời trang"},
		{"vfx_3d", "3D / VFX"},
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
		Description: "Chạy workflow " + title + " đã lưu, truyền input và nhận output theo contract",
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
