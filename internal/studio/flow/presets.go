package flow

// GetPresetGraphs trả về các mẫu luồng ComfyUI có sẵn trong hệ thống.
func GetPresetGraphs() []FlowGraph {
	return []FlowGraph{
		PresetFashionTryOnAngles(),
		PresetViralDance(),
		PresetAffiliateShowcase(),
		PresetMultiAIHybrid(),
	}
}

// PresetViralDance: Luồng tạo video viral tái hiện điệu nhảy TikTok theo nhân vật kênh.
func PresetViralDance() FlowGraph {
	return FlowGraph{
		ID:          "preset_viral_dance",
		Name:        "Viral TikTok Dance (Tái hiện vũ đạo nhân vật)",
		Description: "Bóc tách điệu nhảy từ video TikTok Radar gốc, ghép vũ đạo và biểu cảm vào nhân vật mẫu và render bằng Google Flow",
		Category:    "viral",
		IsPreset:    true,
		Nodes: []FlowNode{
			{
				ID:       "node_char",
				Type:     "character_input",
				Title:    "1. Nhân vật mẫu của kênh",
				Category: CategoryInput,
				X:        60,
				Y:        80,
				Params: map[string]any{
					"model_name": "Miu Nhân vật mẫu",
				},
			},
			{
				ID:       "node_video",
				Type:     "video_source",
				Title:    "2. Video TikTok Radar gốc",
				Category: CategoryInput,
				X:        60,
				Y:        490,
				Params: map[string]any{
					"video_url": "https://www.tiktok.com/@vuvu/video/trend",
				},
			},
			{
				ID:       "node_dance_extract",
				Type:     "dance_motion_extractor",
				Title:    "3. Bóc tách vũ đạo & BPM",
				Category: CategoryProcess,
				X:        430,
				Y:        490,
				Params: map[string]any{
					"extract_poses": true,
				},
			},
			{
				ID:       "node_dance_retarget",
				Type:     "dance_retarget",
				Title:    "4. Ghép vũ đạo vào nhân vật",
				Category: CategoryProcess,
				X:        790,
				Y:        80,
				Params: map[string]any{
					"expression": "expressive_smile",
				},
			},
			{
				ID:       "node_flow_video",
				Type:     "google_flow_video",
				Title:    "5. Google Flow Veo 2",
				Category: CategoryAI,
				X:        1160,
				Y:        180,
				Params: map[string]any{
					"aspect": "VIDEO_ASPECT_RATIO_PORTRAIT",
				},
			},
			{
				ID:       "node_audio_merge",
				Type:     "audio_merge",
				Title:    "6. Ghép âm thanh gốc",
				Category: CategoryProcess,
				X:        1530,
				Y:        280,
				Params: map[string]any{
					"audio_codec": "aac",
				},
			},
			{
				ID:       "node_upscale",
				Type:     "video_upscale",
				Title:    "7. Làm nét & Upscale 4K",
				Category: CategoryProcess,
				X:        1900,
				Y:        280,
				Params: map[string]any{
					"resolution": "4k",
				},
			},
			{
				ID:       "node_output",
				Type:     "video_output",
				Title:    "8. Xuất video TikTok 9:16",
				Category: CategoryOutput,
				X:        2270,
				Y:        280,
				Params: map[string]any{
					"auto_publish_draft": false,
				},
			},
		},
		Edges: []FlowEdge{
			{ID: "e1", FromNode: "node_char", FromPort: "character_image", ToNode: "node_dance_retarget", ToPort: "character_image"},
			{ID: "e2", FromNode: "node_video", FromPort: "video_path", ToNode: "node_dance_extract", ToPort: "video_in"},
			{ID: "e3", FromNode: "node_dance_extract", FromPort: "motion_data", ToNode: "node_dance_retarget", ToPort: "motion_data"},
			{ID: "e4", FromNode: "node_dance_retarget", FromPort: "prompt_choreography", ToNode: "node_flow_video", ToPort: "prompt"},
			{ID: "e5", FromNode: "node_dance_retarget", FromPort: "ingredients", ToNode: "node_flow_video", ToPort: "ingredients"},
			{ID: "e6", FromNode: "node_flow_video", FromPort: "video_out", ToNode: "node_audio_merge", ToPort: "video_in"},
			{ID: "e7", FromNode: "node_dance_extract", FromPort: "beat_audio", ToNode: "node_audio_merge", ToPort: "audio_in"},
			{ID: "e8", FromNode: "node_audio_merge", FromPort: "video_out", ToNode: "node_upscale", ToPort: "video_in"},
			{ID: "e9", FromNode: "node_upscale", FromPort: "video_out", ToNode: "node_output", ToPort: "video_in"},
		},
	}
}

// PresetAffiliateShowcase: Luồng tạo video tiếp thị liên kết kết hợp nhân vật mẫu và bóc tách sản phẩm.
func PresetAffiliateShowcase() FlowGraph {
	return FlowGraph{
		ID:          "preset_affiliate_showcase",
		Name:        "Affiliate Product Showcase (Ghép sản phẩm & nhân vật)",
		Description: "Bóc tách sản phẩm từ ảnh mẫu, ghép sản phẩm vào tay nhân vật đại diện, sinh prompt tự động và tạo video với Muse AI / Google Flow",
		Category:    "affiliate",
		IsPreset:    true,
		Nodes: []FlowNode{
			{
				ID:       "node_char",
				Type:     "character_input",
				Title:    "1. Nhân vật mẫu",
				Category: CategoryInput,
				X:        60,
				Y:        80,
			},
			{
				ID:       "node_prod",
				Type:     "product_input",
				Title:    "2. Sản phẩm mẫu",
				Category: CategoryInput,
				X:        60,
				Y:        490,
				Params: map[string]any{
					"product_title": "Serum Dưỡng Trắng Da Miu Glow",
				},
			},
			{
				ID:       "node_extract",
				Type:     "product_extractor",
				Title:    "3. Bóc tách sản phẩm",
				Category: CategoryProcess,
				X:        430,
				Y:        490,
			},
			{
				ID:       "node_composite",
				Type:     "image_composite",
				Title:    "4. Ghép sản phẩm vào nhân vật",
				Category: CategoryProcess,
				X:        790,
				Y:        180,
				Params: map[string]any{
					"placement": "holding_hand",
					"scale":     0.45,
				},
			},
			{
				ID:       "node_prompt",
				Type:     "prompt_template",
				Title:    "5. Tạo câu lệnh sản phẩm",
				Category: CategoryProcess,
				X:        1160,
				Y:        180,
				Params: map[string]any{
					"template": "Cinematic vertical 9:16 video of {character} happily presenting {product}. Soft luxury studio lighting, photorealistic detail, 4k quality.",
				},
			},
			{
				ID:       "node_muse_video",
				Type:     "muse_ai_video",
				Title:    "6. Muse AI / Flow Video",
				Category: CategoryAI,
				X:        1530,
				Y:        180,
			},
			{
				ID:       "node_audio",
				Type:     "audio_source",
				Title:    "7. Nhạc nền tiếp thị",
				Category: CategoryInput,
				X:        1160,
				Y:        500,
			},
			{
				ID:       "node_audio_merge",
				Type:     "audio_merge",
				Title:    "8. Ghép âm thanh & nhạc nền",
				Category: CategoryProcess,
				X:        1900,
				Y:        280,
			},
			{
				ID:       "node_output",
				Type:     "video_output",
				Title:    "9. Xuất video Affiliate",
				Category: CategoryOutput,
				X:        2270,
				Y:        280,
			},
		},
		Edges: []FlowEdge{
			{ID: "e1", FromNode: "node_prod", FromPort: "product_image", ToNode: "node_extract", ToPort: "source_image"},
			{ID: "e2", FromNode: "node_char", FromPort: "character_image", ToNode: "node_composite", ToPort: "character_image"},
			{ID: "e3", FromNode: "node_extract", FromPort: "cutout_image", ToNode: "node_composite", ToPort: "product_image"},
			{ID: "e4", FromNode: "node_composite", FromPort: "composite_image", ToNode: "node_muse_video", ToPort: "reference_image"},
			{ID: "e5", FromNode: "node_char", FromPort: "character_name", ToNode: "node_prompt", ToPort: "character_desc"},
			{ID: "e6", FromNode: "node_prod", FromPort: "product_title", ToNode: "node_prompt", ToPort: "product_desc"},
			{ID: "e7", FromNode: "node_prompt", FromPort: "prompt", ToNode: "node_muse_video", ToPort: "prompt"},
			{ID: "e8", FromNode: "node_muse_video", FromPort: "video_out", ToNode: "node_audio_merge", ToPort: "video_in"},
			{ID: "e9", FromNode: "node_audio", FromPort: "audio_file", ToNode: "node_audio_merge", ToPort: "audio_in"},
			{ID: "e10", FromNode: "node_audio_merge", FromPort: "video_out", ToNode: "node_output", ToPort: "video_in"},
		},
	}
}

// PresetMultiAIHybrid: Luồng AI đa tác tử kết hợp Gemini Vision + Google Flow + Custom AI.
func PresetMultiAIHybrid() FlowGraph {
	return FlowGraph{
		ID:          "preset_multi_ai",
		Name:        "Multi-Engine AI Studio (Gemini + Flow + Custom AI)",
		Description: "Dùng Gemini Vision phân tích ảnh, sinh prompt tối ưu, kết hợp nhiều Web AI và ghép cảnh hoàn thiện",
		Category:    "custom",
		IsPreset:    true,
		Nodes: []FlowNode{
			{
				ID:       "node_char",
				Type:     "character_input",
				Title:    "1. Nhân vật mẫu",
				Category: CategoryInput,
				X:        60,
				Y:        180,
			},
			{
				ID:       "node_gemini",
				Type:     "gemini_vision",
				Title:    "2. Phân tích Gemini Vision",
				Category: CategoryAI,
				X:        380,
				Y:        180,
			},
			{
				ID:       "node_flow",
				Type:     "google_flow_video",
				Title:    "3. Cảnh 1: Google Flow Veo",
				Category: CategoryAI,
				X:        720,
				Y:        80,
			},
			{
				ID:       "node_custom_ai",
				Type:     "custom_web_ai",
				Title:    "4. Cảnh 2: Custom Web AI",
				Category: CategoryAI,
				X:        720,
				Y:        320,
				Params: map[string]any{
					"provider_name": "Grok Video",
					"output_type":   "video",
				},
			},
			{
				ID:       "node_concat",
				Type:     "concat_scenes",
				Title:    "5. Ghép nối 2 phân cảnh",
				Category: CategoryProcess,
				X:        1060,
				Y:        200,
			},
			{
				ID:       "node_output",
				Type:     "video_output",
				Title:    "6. Xuất video Master",
				Category: CategoryOutput,
				X:        1380,
				Y:        200,
			},
		},
		Edges: []FlowEdge{
			{ID: "e1", FromNode: "node_char", FromPort: "character_image", ToNode: "node_gemini", ToPort: "image_in"},
			{ID: "e2", FromNode: "node_gemini", FromPort: "optimized_prompt", ToNode: "node_flow", ToPort: "prompt"},
			{ID: "e3", FromNode: "node_char", FromPort: "character_image", ToNode: "node_flow", ToPort: "character_image"},
			{ID: "e4", FromNode: "node_gemini", FromPort: "optimized_prompt", ToNode: "node_custom_ai", ToPort: "prompt_in"},
			{ID: "e5", FromNode: "node_flow", FromPort: "video_out", ToNode: "node_concat", ToPort: "scene_1"},
			{ID: "e6", FromNode: "node_custom_ai", FromPort: "result_out", ToNode: "node_concat", ToPort: "scene_2"},
			{ID: "e7", FromNode: "node_concat", FromPort: "master_video", ToNode: "node_output", ToPort: "video_in"},
		},
	}
}

// PresetFashionTryOnAngles: Luồng thời trang thay đổi trang phục nữ và dựng video đa góc quay với sản phẩm.
func PresetFashionTryOnAngles() FlowGraph {
	return FlowGraph{
		ID:          "preset_fashion_tryon",
		Name:        "Fashion Try-On & Đa Góc Quay (Đầm Lụa Pháp)",
		Description: "Thay đổi trang phục cho nhân vật mẫu sang Đầm Lụa Satin Pháp, tự động điều hướng 3 góc quay (Runway walk, Bán thân 360, Cận cảnh chất vải) và render video lookbook chất lượng cao",
		Category:    "fashion",
		IsPreset:    true,
		Nodes: []FlowNode{
			{
				ID:       "node_char",
				Type:     "character_input",
				Title:    "1. Nhân vật mẫu (Miu Street Style)",
				Category: CategoryInput,
				X:        60,
				Y:        80,
				Params: map[string]any{
					"model_name":  "Miu Urban Model",
					"model_path":  "/static/presets/pintest-01.jpg",
					"preview_url": "/static/presets/pintest-01.jpg",
				},
			},
			{
				ID:       "node_garment",
				Type:     "product_input",
				Title:    "2. Đầm Lụa Satin Pháp Nữ",
				Category: CategoryInput,
				X:        60,
				Y:        490,
				Params: map[string]any{
					"product_title": "Đầm Lụa Satin Pháp Dáng Dài (French Silk Slip Dress)",
					"product_image": "/static/presets/char_02.jpg",
					"preview_url":   "/static/presets/char_02.jpg",
					"affiliate_url": "https://shopee.vn/french-silk-slip-dress",
				},
			},
			{
				ID:       "node_extract",
				Type:     "product_extractor",
				Title:    "3. Bóc tách phom váy lụa",
				Category: CategoryProcess,
				X:        430,
				Y:        490,
				Params: map[string]any{
					"refine_edges": true,
				},
			},
			{
				ID:       "node_tryon",
				Type:     "virtual_tryon",
				Title:    "4. Thay đổi trang phục AI (Virtual Try-on)",
				Category: CategoryProcess,
				X:        790,
				Y:        180,
				Params: map[string]any{
					"fit_style":     "seamless_luxury",
					"preserve_face": true,
					"preview_url":   "/static/presets/pintest-01.jpg",
				},
			},
			{
				ID:       "node_director",
				Type:     "camera_angles_director",
				Title:    "5. Đạo diễn 3 góc quay điện ảnh",
				Category: CategoryProcess,
				X:        1160,
				Y:        180,
				Params: map[string]any{
					"shot_style": "high_fashion_editorial",
				},
			},
			{
				ID:       "node_flow_video",
				Type:     "google_flow_video",
				Title:    "6. Google Flow Veo 2 (Render đa góc quay)",
				Category: CategoryAI,
				X:        1530,
				Y:        180,
				Params: map[string]any{
					"aspect": "VIDEO_ASPECT_RATIO_PORTRAIT",
					"mode":   "ingredients",
				},
			},
			{
				ID:       "node_audio",
				Type:     "audio_source",
				Title:    "7. Nhạc Runway Lounge Beat",
				Category: CategoryInput,
				X:        1160,
				Y:        500,
				Params: map[string]any{
					"audio_title": "Paris Fashion Runway Lounge Beat (115 BPM)",
					"audio_path":  "/static/preview.mp3",
				},
			},
			{
				ID:       "node_concat",
				Type:     "concat_scenes",
				Title:    "8. Ghép 3 góc quay (Toàn thân + Bán thân + Cận cảnh)",
				Category: CategoryProcess,
				X:        1900,
				Y:        180,
			},
			{
				ID:       "node_audio_merge",
				Type:     "audio_merge",
				Title:    "9. Lồng nhạc & Âm thanh Runway",
				Category: CategoryProcess,
				X:        2270,
				Y:        280,
				Params: map[string]any{
					"audio_codec": "aac",
				},
			},
			{
				ID:       "node_upscale",
				Type:     "video_upscale",
				Title:    "10. Làm nét & Nâng chuẩn 4K UHD",
				Category: CategoryProcess,
				X:        2640,
				Y:        280,
				Params: map[string]any{
					"resolution": "4k",
				},
			},
			{
				ID:       "node_output",
				Type:     "video_output",
				Title:    "11. Xuất video Lookbook 9:16",
				Category: CategoryOutput,
				X:        3010,
				Y:        280,
				Params: map[string]any{
					"title": "Lookbook Đầm Lụa Pháp - Xu hướng thời trang nữ cao cấp",
				},
			},
		},
		Edges: []FlowEdge{
			{ID: "e1", FromNode: "node_char", FromPort: "character_image", ToNode: "node_tryon", ToPort: "character_image"},
			{ID: "e2", FromNode: "node_garment", FromPort: "product_image", ToNode: "node_extract", ToPort: "source_image"},
			{ID: "e3", FromNode: "node_extract", FromPort: "cutout_image", ToNode: "node_tryon", ToPort: "garment_image"},
			{ID: "e4", FromNode: "node_tryon", FromPort: "fitted_image", ToNode: "node_director", ToPort: "character_image"},
			{ID: "e5", FromNode: "node_director", FromPort: "angle_full", ToNode: "node_flow_video", ToPort: "prompt"},
			{ID: "e6", FromNode: "node_tryon", FromPort: "fitted_image", ToNode: "node_flow_video", ToPort: "character_image"},
			{ID: "e7", FromNode: "node_flow_video", FromPort: "video_out", ToNode: "node_concat", ToPort: "scene_1"},
			{ID: "e8", FromNode: "node_concat", FromPort: "master_video", ToNode: "node_audio_merge", ToPort: "video_in"},
			{ID: "e9", FromNode: "node_audio", FromPort: "audio_file", ToNode: "node_audio_merge", ToPort: "audio_in"},
			{ID: "e10", FromNode: "node_audio_merge", FromPort: "video_out", ToNode: "node_upscale", ToPort: "video_in"},
			{ID: "e11", FromNode: "node_upscale", FromPort: "video_out", ToNode: "node_output", ToPort: "video_in"},
		},
	}
}
