package web

func (s *Server) storyTeam() pipeTeam {
	stages := []pipeStage{}
	j := s.latestJobByKind("story")
	if j == nil {
		for _, st := range []pipeStage{
			{Key: "writer", Label: "Writer", Icon: "✍️", Link: "/stories", RoleDesc: "Viết kịch bản cốt truyện lôi cuốn"},
			{Key: "illustrator", Label: "Illustrator", Icon: "🎨", Link: "/stories", RoleDesc: "Vẽ tranh minh họa từng cảnh bằng AI"},
			{Key: "voice", Label: "Voice", Icon: "🎙️", Link: "/stories", RoleDesc: "Đọc giọng truyền cảm xúc bằng AI Voice TTS"},
			{Key: "producer", Label: "Producer", Icon: "🎬", Link: "/stories", RoleDesc: "Ghép cảnh, hòa âm và dựng video 16:9"},
			{Key: "publisher", Label: "Publisher", Icon: "📤", Link: "/accounts", RoleDesc: "Xuất bản video lên YouTube"},
		} {
			st.Status = tmWaiting
			st.Detail = "chưa có truyện"
			stages = append(stages, st)
		}
		return pipeTeam{
			Key:         "story",
			Name:        "Kể chuyện",
			Icon:        "📖",
			Description: "Tự động sáng tác truyện dài, vẽ minh họa từng cảnh và đọc giọng truyền cảm xúc.",
			ActionLink:  "/stories",
			ActionLabel: "📖 Viết truyện mới",
			Stages:      stages,
		}
	}
	// Ánh xạ tiến độ job → chặng đang làm (ước lượng, ghi rõ trong UI).
	cur := storyStageFor(j)
	keys := []pipeStage{
		{Key: "writer", Label: "Writer", Icon: "✍️", Link: "/stories", RoleDesc: "Viết kịch bản cốt truyện lôi cuốn"},
		{Key: "illustrator", Label: "Illustrator", Icon: "🎨", Link: "/stories", RoleDesc: "Vẽ tranh minh họa từng cảnh bằng AI"},
		{Key: "voice", Label: "Voice", Icon: "🎙️", Link: "/stories", RoleDesc: "Đọc giọng truyền cảm xúc bằng AI Voice TTS"},
		{Key: "producer", Label: "Producer", Icon: "🎬", Link: "/stories", RoleDesc: "Ghép cảnh, hòa âm và dựng video 16:9"},
		{Key: "publisher", Label: "Publisher", Icon: "📤", Link: "/accounts", RoleDesc: "Xuất bản video lên YouTube"},
	}
	for i := range keys {
		switch {
		case j.Status == "failed":
			keys[i].Status = tmFail
			keys[i].Detail = "job lỗi"
		case j.Status == "done":
			keys[i].Status = tmOK
			keys[i].Detail = "xong"
		case i < cur:
			keys[i].Status = tmOK
			keys[i].Detail = "xong"
		case i == cur:
			keys[i].Status = tmWorking
			keys[i].Detail = "đang làm (ước lượng)"
		default:
			keys[i].Status = tmWaiting
			keys[i].Detail = "chờ"
		}
		stages = append(stages, keys[i])
	}
	return pipeTeam{
		Key:         "story",
		Name:        "Kể chuyện",
		Icon:        "📖",
		Description: "Tự động sáng tác truyện dài, vẽ minh họa từng cảnh và đọc giọng truyền cảm xúc.",
		ActionLink:  "/stories",
		ActionLabel: "📖 Viết truyện mới",
		Stages:      stages,
	}
}

// storyStageFor ước lượng chặng hiện tại từ tiến độ job (0-100).
func storyStageFor(j *studioJobLite) int {
	switch {
	case j.Progress < 20:
		return 0 // writer
	case j.Progress < 45:
		return 1 // illustrator
	case j.Progress < 65:
		return 2 // voice
	case j.Progress < 90:
		return 3 // producer
	default:
		return 4 // publisher
	}
}
