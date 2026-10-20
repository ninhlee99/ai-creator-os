package flow

import (
	"fmt"
	"time"
)

// Standard Recipe Keys
const (
	RecipeKeyDanceTrendjack     = "recipe_dance_trendjack"
	RecipeKeyLookbookOutfit     = "recipe_lookbook_outfit"
	RecipeKeyAudioStoryCinema   = "recipe_audio_story_cinema"
	RecipeKeyChineseCinemaDrama = "recipe_chinese_cinema_drama"
)

// RecipeCatalog holds metadata for display in UI.
type RecipeCatalogItem struct {
	Key         string   `json:"key"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Category    string   `json:"category"`
	Icon        string   `json:"icon"`
	Badge       string   `json:"badge"`
	TotalNodes  int      `json:"total_nodes"`
	NodesList   []string `json:"nodes_list"`
}

// ListRecipes returns all available pre-built recipes.
func ListRecipes() []RecipeCatalogItem {
	return []RecipeCatalogItem{
		{
			Key:         RecipeKeyLookbookOutfit,
			Title:       "Affiliate Thời Trang Nữ Dễ Thương (Model Của Bạn)",
			Description: "Sản xuất video lookbook 9:16: Phối mẫu nhân vật của bạn với váy hoa nhí/đầm voan dễ thương Shopee bằng Gemini Web, sinh chuyển động qua Google Flow (Veo Free 1.000 Credits) hoặc Muse AI, không tốn tiền API.",
			Category:    "Affiliate Thời Trang",
			Icon:        "👗",
			Badge:       "Google Flow & Muse AI (0đ)",
			TotalNodes:  9,
			NodesList:   []string{"Váy Nữ Shopee", "Model Của Bạn", "Gemini Phối Đồ (0đ)", "Google Flow (1000 Credits)", "Remaster 4K", "Khóa Mặt 360°", "Cắt 9:16", "Soul Index QC", "Xuất Bản"},
		},
		{
			Key:         RecipeKeyDanceTrendjack,
			Title:       "Trending Vũ Đạo Sexy Dance (Khóa Mặt Model 360°)",
			Description: "Bắt trend nhảy Sexy Dance triệu view Douyin/TikTok, tạo chuyển động 60fps qua Muse AI Video Studio (Free) & Google Flow, khóa nét mặt 360° mẫu nhân vật, 100% không tốn phí.",
			Category:    "TikTok / Douyin Dance",
			Icon:        "💃",
			Badge:       "Muse AI 60fps (0đ)",
			TotalNodes:  10,
			NodesList:   []string{"Trend Dance Douyin", "Outfit Nữ", "Model Của Bạn", "Gemini Phối Đồ (0đ)", "Muse AI Motion (0đ)", "Khóa Mặt 360°", "Cắt Chuẩn 9:16", "CapCut Beat Mux", "Soul Index QC", "Xuất Bản"},
		},
		{
			Key:         RecipeKeyAudioStoryCinema,
			Title:       "Truyện Audio Tình Yêu (Anime Makoto Shinkai)",
			Description: "Truyện tình yêu nam nữ thanh xuân lãng mạn phong cách Anime Makoto Shinkai: Kịch bản phân đoạn sâu lắng qua ChatGPT/Claude/Gemini, hình ảnh Anime 4K lung linh, giọng đọc Google Gemini Cloud TTS (Aoede) miễn phí và hòa âm tiếng mưa piano.",
			Category:    "Truyện Audio Anime",
			Icon:        "🎙️",
			Badge:       "Gemini Cloud TTS (0đ)",
			TotalNodes:  8,
			NodesList:   []string{"Kịch Bản (ChatGPT/Claude)", "Phân Cảnh (Gemini)", "Ảnh Anime 4K (Google Labs)", "Gemini Cloud TTS (Aoede)", "Hòa Âm Mưa & Piano", "CapCut Ken Burns", "Soul Index QC", "Xuất Bản"},
		},
		{
			Key:         RecipeKeyChineseCinemaDrama,
			Title:       "Phim Ngắn Vi Kịch AI (Google Flow + Muse AI + Gemini)",
			Description: "Quy trình làm phim vi kịch công nghiệp hóa 100% Miễn Phí (Zero Cost): Phân cảnh đạo diễn ChatGPT/Claude, Ngân hàng diễn viên 360°, First/Last Frame Google Flow & Muse AI Video, Khớp khẩu hình Muse Sync, Hòa âm 4 tầng và Master 21:9.",
			Category:    "AI Cinema (Zero Cost)",
			Icon:        "🎬",
			Badge:       "100% Free Cloud",
			TotalNodes:  11,
			NodesList:   []string{"Kịch Bản Phân Cảnh (Claude/GPT)", "Actor Bank 360°", "Góc Máy Điện Ảnh", "Gemini Thoại (0đ)", "Google Flow Video (0đ)", "Muse LipSync (0đ)", "Khóa Mặt Diễn Viên", "Hòa Âm 4 Tầng", "Color LUT", "Soul Index QC", "Master 21:9"},
		},
	}
}

// BuildRecipe builds a ready-to-run Graph from a recipe key.
func BuildRecipe(recipeKey string, customParams map[string]interface{}) (*Graph, error) {
	switch recipeKey {
	case RecipeKeyChineseCinemaDrama:
		return buildChineseCinemaDramaGraph(customParams)
	case RecipeKeyDanceTrendjack:
		return buildDanceTrendjackGraph(customParams)
	case RecipeKeyLookbookOutfit:
		return buildLookbookOutfitGraph(customParams)
	case RecipeKeyAudioStoryCinema:
		return buildAudioStoryCinemaGraph(customParams)
	default:
		return nil, fmt.Errorf("không tìm thấy recipe '%s'", recipeKey)
	}
}

func buildDanceTrendjackGraph(params map[string]interface{}) (*Graph, error) {
	reg := GlobalRegistry()
	g := &Graph{
		ID:          fmt.Sprintf("flow-dance-%d", time.Now().UnixNano()),
		Title:       "Trending Vũ Đạo Sexy Dance (Khóa Mặt Model 360°)",
		Description: "Bắt trend nhảy Sexy Dance triệu view Douyin/TikTok, khóa nét mặt 360° của mẫu nhân vật bạn đã tải lên, tự động khớp nhịp beat drop CapCut.",
		RecipeKey:   RecipeKeyDanceTrendjack,
		Status:      GraphPending,
		CreatedAt:   time.Now(),
		Nodes:       make([]*Node, 0),
		Wires:       make([]Wire, 0),
		Log:         []string{"Khởi tạo đồ thị Trending Sexy Dance Douyin..."},
		Metadata:    params,
	}

	// 1. TikTok Trend Node
	nTrend, _ := reg.CreateNode("n_trend", NodeTikTokTrend, "Sexy Dance Douyin Hot Trend (130 BPM)")
	nTrend.PositionX, nTrend.PositionY = 50, 100
	if url, ok := params["trend_url"].(string); ok && url != "" {
		nTrend.Params["url"] = url
	}
	g.Nodes = append(g.Nodes, nTrend)

	// 2. Product Outfit Node
	nOutfit, _ := reg.CreateNode("n_outfit", NodeProductOutfit, "Outfit Phối Đồ Sexy Dance")
	nOutfit.PositionX, nOutfit.PositionY = 50, 260
	if purl, ok := params["product_url"].(string); ok && purl != "" {
		nOutfit.Params["product_url"] = purl
	}
	g.Nodes = append(g.Nodes, nOutfit)

	// 3. Creator Face Identity Node
	nFace, _ := reg.CreateNode("n_face", NodeCharacterFace, "Mẫu Nhân Vật Của Bạn (4 Ảnh 360°)")
	nFace.PositionX, nFace.PositionY = 50, 420
	if ch, ok := params["channel"].(string); ok && ch != "" {
		nFace.Params["channel"] = ch
	}
	g.Nodes = append(g.Nodes, nFace)

	// 4. Web Virtual Try-On Node (Gemini / Claude Web - 0đ)
	nTryOn, _ := reg.CreateNode("n_tryon", NodeWebVirtualTryOn, "Web Adapter: Phối Đồ (Gemini / Claude Web)")
	nTryOn.PositionX, nTryOn.PositionY = 300, 260
	g.Nodes = append(g.Nodes, nTryOn)

	// 5. Web Motion Transfer Node (Muse AI Free Studio)
	nMotion, _ := reg.CreateNode("n_motion", NodeWebMotionTransfer, "Web Adapter: Vũ Đạo Sexy Dance (Muse AI Studio Free)")
	nMotion.PositionX, nMotion.PositionY = 550, 180
	g.Nodes = append(g.Nodes, nMotion)

	// 6. Local InsightFace Swap Node
	nSwap, _ := reg.CreateNode("n_swap", NodeInsightFaceSwap, "Web Adapter: Khóa Mặt 360° Model Của Bạn")
	nSwap.PositionX, nSwap.PositionY = 800, 260
	g.Nodes = append(g.Nodes, nSwap)

	// 7. Local Anti-Distortion Crop 9:16
	nCrop, _ := reg.CreateNode("n_crop", NodeAntiDistortion, "Cắt Chuẩn 9:16 Chống Méo Hình")
	nCrop.PositionX, nCrop.PositionY = 1050, 260
	g.Nodes = append(g.Nodes, nCrop)

	// 8. CapCut Beat Muxer
	nMux, _ := reg.CreateNode("n_mux", NodeCapCutBeatMux, "Cắt Nhịp CapCut Pro & Mux Bass Drop")
	nMux.PositionX, nMux.PositionY = 1300, 200
	g.Nodes = append(g.Nodes, nMux)

	// 9. Soul Index QC
	nQC, _ := reg.CreateNode("n_qc", NodeSoulIndexQC, "QC Thất Pháp Video Có Hồn")
	nQC.PositionX, nQC.PositionY = 1550, 200
	g.Nodes = append(g.Nodes, nQC)

	// 10. Export Target
	nExport, _ := reg.CreateNode("n_export", NodeExportTarget, "Xuất Bản TikTok/Reels Sẵn Sàng")
	nExport.PositionX, nExport.PositionY = 1800, 200
	g.Nodes = append(g.Nodes, nExport)

	// Wires connection
	g.Wires = []Wire{
		{FromNode: "n_outfit", FromPin: "outfit_image", ToNode: "n_tryon", ToPin: "outfit_image"},
		{FromNode: "n_face", FromPin: "face_image", ToNode: "n_tryon", ToPin: "model_image"},
		{FromNode: "n_trend", FromPin: "video_url", ToNode: "n_motion", ToPin: "reference_video"},
		{FromNode: "n_tryon", FromPin: "dressed_image", ToNode: "n_motion", ToPin: "character_image"},
		{FromNode: "n_motion", FromPin: "motion_video", ToNode: "n_swap", ToPin: "target_video"},
		{FromNode: "n_face", FromPin: "face_image", ToNode: "n_swap", ToPin: "source_face"},
		{FromNode: "n_swap", FromPin: "swapped_video", ToNode: "n_crop", ToPin: "input_media"},
		{FromNode: "n_crop", FromPin: "cropped_media", ToNode: "n_mux", ToPin: "video_stream"},
		{FromNode: "n_trend", FromPin: "audio_url", ToNode: "n_mux", ToPin: "audio_stream"},
		{FromNode: "n_mux", FromPin: "final_video", ToNode: "n_qc", ToPin: "video_file"},
		{FromNode: "n_mux", FromPin: "final_video", ToNode: "n_export", ToPin: "final_media"},
	}

	return g, nil
}

func buildLookbookOutfitGraph(params map[string]interface{}) (*Graph, error) {
	reg := GlobalRegistry()
	g := &Graph{
		ID:          fmt.Sprintf("flow-lookbook-%d", time.Now().UnixNano()),
		Title:       "Affiliate Thời Trang Nữ Dễ Thương (Model Của Bạn)",
		Description: "Tạo video lookbook 9:16: Phối mẫu nhân vật của bạn với váy hoa nhí/đầm voan dễ thương Shopee/Accesstrade, chuyển động tự nhiên đi dạo phố.",
		RecipeKey:   RecipeKeyLookbookOutfit,
		Status:      GraphPending,
		CreatedAt:   time.Now(),
		Nodes:       make([]*Node, 0),
		Wires:       make([]Wire, 0),
		Log:         []string{"Khởi tạo đồ thị Affiliate Thời Trang Nữ Dễ Thương..."},
		Metadata:    params,
	}

	// Nodes
	nOutfit, _ := reg.CreateNode("n_outfit", NodeProductOutfit, "Váy Hoa Nhí Vintage Dễ Thương (Shopee)")
	nOutfit.PositionX, nOutfit.PositionY = 50, 150
	g.Nodes = append(g.Nodes, nOutfit)

	nFace, _ := reg.CreateNode("n_face", NodeCharacterFace, "Mẫu Nhân Vật Của Bạn (4 Ảnh 360°)")
	nFace.PositionX, nFace.PositionY = 50, 320
	g.Nodes = append(g.Nodes, nFace)

	nTryOn, _ := reg.CreateNode("n_tryon", NodeWebVirtualTryOn, "Web Adapter: Thử Đồ (Gemini / Claude Web)")
	nTryOn.PositionX, nTryOn.PositionY = 300, 200
	g.Nodes = append(g.Nodes, nTryOn)

	nVeo, _ := reg.CreateNode("n_veo", NodeWebVideoGen, "Web Adapter: Google Flow (Veo 1.000 Credits Free)")
	nVeo.PositionX, nVeo.PositionY = 550, 200
	nVeo.Params["motion_prompt"] = "Cute Asian girl walking gracefully in floral dress on sunlit street, 9:16 vertical, soft breeze, joyful smile"
	g.Nodes = append(g.Nodes, nVeo)

	nRemaster, _ := reg.CreateNode("n_remaster", NodeVideoRemaster, "Nâng Cấp 4K Crisp")
	nRemaster.PositionX, nRemaster.PositionY = 800, 200
	g.Nodes = append(g.Nodes, nRemaster)

	nSwap, _ := reg.CreateNode("n_swap", NodeInsightFaceSwap, "Web Adapter: Khóa Mặt 360° Model Của Bạn")
	nSwap.PositionX, nSwap.PositionY = 1050, 200
	g.Nodes = append(g.Nodes, nSwap)

	nCrop, _ := reg.CreateNode("n_crop", NodeAntiDistortion, "Cắt Chuẩn 9:16 Chống Méo")
	nCrop.PositionX, nCrop.PositionY = 1300, 200
	g.Nodes = append(g.Nodes, nCrop)

	nQC, _ := reg.CreateNode("n_qc", NodeSoulIndexQC, "Soul Index QC")
	nQC.PositionX, nQC.PositionY = 1550, 200
	g.Nodes = append(g.Nodes, nQC)

	nExport, _ := reg.CreateNode("n_export", NodeExportTarget, "Xuất Bản Lookbook")
	nExport.PositionX, nExport.PositionY = 1800, 200
	g.Nodes = append(g.Nodes, nExport)

	g.Wires = []Wire{
		{FromNode: "n_outfit", FromPin: "outfit_image", ToNode: "n_tryon", ToPin: "outfit_image"},
		{FromNode: "n_face", FromPin: "face_image", ToNode: "n_tryon", ToPin: "model_image"},
		{FromNode: "n_tryon", FromPin: "dressed_image", ToNode: "n_veo", ToPin: "keyframe_image"},
		{FromNode: "n_veo", FromPin: "video_clip", ToNode: "n_remaster", ToPin: "sd_video"},
		{FromNode: "n_remaster", FromPin: "hd_video", ToNode: "n_swap", ToPin: "target_video"},
		{FromNode: "n_face", FromPin: "face_image", ToNode: "n_swap", ToPin: "source_face"},
		{FromNode: "n_swap", FromPin: "swapped_video", ToNode: "n_crop", ToPin: "input_media"},
		{FromNode: "n_crop", FromPin: "cropped_media", ToNode: "n_qc", ToPin: "video_file"},
		{FromNode: "n_crop", FromPin: "cropped_media", ToNode: "n_export", ToPin: "final_media"},
	}

	return g, nil
}

func buildAudioStoryCinemaGraph(params map[string]interface{}) (*Graph, error) {
	reg := GlobalRegistry()
	g := &Graph{
		ID:          fmt.Sprintf("flow-story-%d", time.Now().UnixNano()),
		Title:       "Truyện Audio Tình Yêu (Anime Makoto Shinkai)",
		Description: "Truyện tình yêu nam nữ thanh xuân lãng mạn phong cách Anime Makoto Shinkai: Hình ảnh Anime 4K lung linh hoàng hôn, giọng đọc Gemini Cloud TTS (Aoede) ấm áp và hòa âm tiếng mưa piano.",
		RecipeKey:   RecipeKeyAudioStoryCinema,
		Status:      GraphPending,
		CreatedAt:   time.Now(),
		Nodes:       make([]*Node, 0),
		Wires:       make([]Wire, 0),
		Log:         []string{"Khởi tạo đồ thị Truyện Tình Yêu Anime Makoto Shinkai..."},
		Metadata:    params,
	}

	// 1. Story Script Input
	nScript, _ := reg.CreateNode("n_script", NodeStoryScript, "Kịch Bản: Hẹn Ước Dưới Mưa Hoàng Hôn")
	nScript.PositionX, nScript.PositionY = 50, 200
	if topic, ok := params["topic"].(string); ok && topic != "" {
		nScript.Params["topic"] = topic
	} else {
		nScript.Params["topic"] = "Hẹn Ước Dưới Mưa Hoàng Hôn"
	}
	if genre, ok := params["genre"].(string); ok && genre != "" {
		nScript.Params["genre"] = genre
	} else {
		nScript.Params["genre"] = "tinh_yeu"
	}
	g.Nodes = append(g.Nodes, nScript)

	// 2. Storyboard Decomposer
	nDecompose, _ := reg.CreateNode("n_decompose", NodeWebStoryDecompose, "Phân Đoạn Cảm Xúc (Makoto Shinkai)")
	nDecompose.PositionX, nDecompose.PositionY = 300, 200
	g.Nodes = append(g.Nodes, nDecompose)

	// 3. Web Image Gen (Gemini / Muse)
	nImage, _ := reg.CreateNode("n_image", NodeWebImageGen, "Vẽ Ảnh Anime Hoàng Hôn 4K (Gemini)")
	nImage.PositionX, nImage.PositionY = 580, 100
	nImage.Params["aspect"] = "16:9"
	g.Nodes = append(g.Nodes, nImage)

	// 4. Gemini Cloud TTS
	nTTS, _ := reg.CreateNode("n_tts", NodeGeminiTTS, "Gemini Cloud TTS Lồng Tiếng (Aoede)")
	nTTS.PositionX, nTTS.PositionY = 580, 300
	nTTS.Params["provider"] = "gemini"
	nTTS.Params["voice"] = "Aoede"
	nTTS.Params["style"] = "thì thầm ấm áp truyền cảm"
	g.Nodes = append(g.Nodes, nTTS)

	// 5. Acoustic Foley Bed
	nFoley, _ := reg.CreateNode("n_foley", NodeAcousticFoleyBed, "Hòa Âm Tiếng Mưa & Lofi Piano (-18dB)")
	nFoley.PositionX, nFoley.PositionY = 850, 300
	g.Nodes = append(g.Nodes, nFoley)

	// 6. Concat Beat Muxer
	nMux, _ := reg.CreateNode("n_mux", NodeCapCutBeatMux, "Dựng Cảnh Ken Burns & Ghép Nhạc")
	nMux.PositionX, nMux.PositionY = 1120, 200
	g.Nodes = append(g.Nodes, nMux)

	// 7. Soul Index QC
	nQC, _ := reg.CreateNode("n_qc", NodeSoulIndexQC, "Soul Index QC")
	nQC.PositionX, nQC.PositionY = 1380, 200
	g.Nodes = append(g.Nodes, nQC)

	// 8. Export Target
	nExport, _ := reg.CreateNode("n_export", NodeExportTarget, "Xuất Bản YouTube Video 16:9")
	nExport.PositionX, nExport.PositionY = 1640, 200
	g.Nodes = append(g.Nodes, nExport)

	g.Wires = []Wire{
		{FromNode: "n_script", FromPin: "raw_text", ToNode: "n_decompose", ToPin: "story_text"},
		{FromNode: "n_decompose", FromPin: "scene_prompts", ToNode: "n_image", ToPin: "prompt"},
		{FromNode: "n_decompose", FromPin: "voiceover_chunks", ToNode: "n_tts", ToPin: "text"},
		{FromNode: "n_tts", FromPin: "speech_audio", ToNode: "n_foley", ToPin: "voice_audio"},
		{FromNode: "n_image", FromPin: "image_url", ToNode: "n_mux", ToPin: "video_stream"},
		{FromNode: "n_foley", FromPin: "mixed_audio", ToNode: "n_mux", ToPin: "audio_stream"},
		{FromNode: "n_mux", FromPin: "final_video", ToNode: "n_qc", ToPin: "video_file"},
		{FromNode: "n_mux", FromPin: "final_video", ToNode: "n_export", ToPin: "final_media"},
	}

	return g, nil
}

func buildChineseCinemaDramaGraph(params map[string]interface{}) (*Graph, error) {
	reg := GlobalRegistry()
	g := &Graph{
		ID:          fmt.Sprintf("flow-cinema-%d", time.Now().UnixNano()),
		Title:       "Phim Ngắn Vi Kịch AI (Google Flow + Muse AI + Gemini)",
		Description: "Quy trình làm phim vi kịch công nghiệp hóa 100% Miễn Phí (Zero Cost): Phân cảnh đạo diễn ChatGPT/Claude, Ngân hàng diễn viên 360°, First/Last Frame Google Flow & Muse AI Video, Khớp khẩu hình Muse Sync, Hòa âm 4 tầng và Master 21:9.",
		RecipeKey:   RecipeKeyChineseCinemaDrama,
		Status:      GraphPending,
		CreatedAt:   time.Now(),
		Nodes:       make([]*Node, 0),
		Wires:       make([]Wire, 0),
		Log:         []string{"Khởi tạo đồ thị Phim Ngắn Vi Kịch AI (Google Flow + Muse AI + Gemini)..."},
		Metadata:    params,
	}

	// 1. Cinema Script Breakdown Node
	nScript, _ := reg.CreateNode("n_script", NodeCinemaScript, "Kịch Bản & Phân Cảnh (Claude / ChatGPT Web)")
	nScript.PositionX, nScript.PositionY = 50, 150
	if prompt, ok := params["prompt"].(string); ok && prompt != "" {
		nScript.Params["script_prompt"] = prompt
	}
	if genre, ok := params["genre"].(string); ok && genre != "" {
		nScript.Params["genre"] = genre
	}
	g.Nodes = append(g.Nodes, nScript)

	// 2. Actor Bank 360° Turnaround Node
	nActor, _ := reg.CreateNode("n_actor", NodeActorBank, "Ngân Hàng Diễn Viên 360° (Actor Bank)")
	nActor.PositionX, nActor.PositionY = 50, 350
	g.Nodes = append(g.Nodes, nActor)

	// 3. Cinematic Camera Motion Node
	nCamera, _ := reg.CreateNode("n_camera", NodeCinematicCamera, "Đạo Diễn Góc Máy Điện Ảnh")
	nCamera.PositionX, nCamera.PositionY = 300, 150
	g.Nodes = append(g.Nodes, nCamera)

	// 4. Gemini Voice Cloud TTS Node
	nTTS, _ := reg.CreateNode("n_tts", NodeGeminiTTS, "Gemini TTS Thoại Kịch Tính (Free Tier)")
	nTTS.PositionX, nTTS.PositionY = 300, 350
	nTTS.Params["provider"] = "gemini"
	nTTS.Params["voice"] = "Charon"
	nTTS.Params["style"] = "kịch tính điện ảnh"
	g.Nodes = append(g.Nodes, nTTS)

	// 5. Web Google Flow / Muse AI Video Keyframes (0đ)
	nKling, _ := reg.CreateNode("n_kling", NodeWebKlingVideo, "Web Adapter: Google Flow & Muse AI Video Keyframes")
	nKling.PositionX, nKling.PositionY = 550, 150
	g.Nodes = append(g.Nodes, nKling)

	// 6. Web Lip Sync Adapter (Muse AI Web Sync Free)
	nLipSync, _ := reg.CreateNode("n_lipsync", NodeWebLipSync, "Web Adapter: Khớp Khẩu Hình (Muse AI Web Sync Free)")
	nLipSync.PositionX, nLipSync.PositionY = 800, 250
	g.Nodes = append(g.Nodes, nLipSync)

	// 7. Local InsightFace Swap Node
	nSwap, _ := reg.CreateNode("n_swap", NodeInsightFaceSwap, "Web Adapter: Khóa Mặt Diễn Viên 360°")
	nSwap.PositionX, nSwap.PositionY = 1050, 250
	g.Nodes = append(g.Nodes, nSwap)

	// 8. 4-Layer Foley Audio Ducking Node
	nDucking, _ := reg.CreateNode("n_ducking", NodeAudioDuckingFoley, "Hòa Âm 4 Tầng & Auto-Ducking (-18dB)")
	nDucking.PositionX, nDucking.PositionY = 1050, 420
	g.Nodes = append(g.Nodes, nDucking)

	// 9. Color Grading LUT Node
	nLUT, _ := reg.CreateNode("n_lut", NodeColorGradingLUT, "Chỉnh Màu Điện Ảnh LUT (Wuxia / Hollywood)")
	nLUT.PositionX, nLUT.PositionY = 1300, 250
	g.Nodes = append(g.Nodes, nLUT)

	// 10. Soul Index QC
	nQC, _ := reg.CreateNode("n_qc", NodeSoulIndexQC, "Soul Index Cinema QC")
	nQC.PositionX, nQC.PositionY = 1550, 250
	g.Nodes = append(g.Nodes, nQC)

	// 11. Cinema Master Export Node
	nMaster, _ := reg.CreateNode("n_master", NodeCinemaMaster, "Xuất Phim Master (Cinemascope 21:9)")
	nMaster.PositionX, nMaster.PositionY = 1800, 250
	g.Nodes = append(g.Nodes, nMaster)

	g.Wires = []Wire{
		{FromNode: "n_script", FromPin: "shot_list", ToNode: "n_camera", ToPin: "shot_index"},
		{FromNode: "n_script", FromPin: "dialogue_list", ToNode: "n_tts", ToPin: "text"},
		{FromNode: "n_actor", FromPin: "turnaround_sheet", ToNode: "n_kling", ToPin: "start_frame"},
		{FromNode: "n_camera", FromPin: "camera_prompt", ToNode: "n_kling", ToPin: "motion_prompt"},
		{FromNode: "n_kling", FromPin: "rendered_clip", ToNode: "n_lipsync", ToPin: "video_clip"},
		{FromNode: "n_tts", FromPin: "speech_audio", ToNode: "n_lipsync", ToPin: "audio_dialogue"},
		{FromNode: "n_lipsync", FromPin: "synced_video", ToNode: "n_swap", ToPin: "target_video"},
		{FromNode: "n_actor", FromPin: "turnaround_sheet", ToNode: "n_swap", ToPin: "source_face"},
		{FromNode: "n_tts", FromPin: "speech_audio", ToNode: "n_ducking", ToPin: "voice_audio"},
		{FromNode: "n_swap", FromPin: "swapped_video", ToNode: "n_lut", ToPin: "input_video"},
		{FromNode: "n_lut", FromPin: "graded_video", ToNode: "n_qc", ToPin: "video_file"},
		{FromNode: "n_lut", FromPin: "graded_video", ToNode: "n_master", ToPin: "final_media"},
	}

	return g, nil
}
