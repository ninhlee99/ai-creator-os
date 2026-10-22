package webai

import (
	"fmt"
	"strings"
)

func evaluateQC(pkg AdScriptPackage) QCScorecard {
	hook := strings.TrimSpace(pkg.HookText)
	hookScore := 0
	if n := len([]rune(hook)); n >= 15 && n <= 120 {
		hookScore += 50
	}
	if mentionsAny(hook, productTerms(pkg.ProductName)) {
		hookScore += 50
	}

	var sceneText strings.Builder
	for _, scene := range pkg.Scenes {
		sceneText.WriteString(" ")
		sceneText.WriteString(strings.ToLower(strings.Join([]string{
			scene.SceneName, scene.VisualConcept, scene.GeminiPrompt, scene.MuseAIPrompt, scene.VoiceoverText,
		}, " ")))
	}
	nicheTerms := productTerms(pkg.Niche)
	nicheScore := 0
	if len(nicheTerms) > 0 {
		matched := 0
		for _, term := range nicheTerms {
			if strings.Contains(sceneText.String(), term) {
				matched++
			}
		}
		nicheScore = matched * 100 / len(nicheTerms)
	} else if mentionsAny(sceneText.String(), productTerms(pkg.ProductName)) {
		nicheScore = 100
	}

	persuasionScore := 0
	if strings.TrimSpace(pkg.CallToAction) != "" {
		persuasionScore += 50
	}
	spokenScenes := 0
	for _, scene := range pkg.Scenes {
		if strings.TrimSpace(scene.VoiceoverText) != "" {
			spokenScenes++
		}
	}
	if len(pkg.Scenes) > 0 {
		persuasionScore += spokenScenes * 50 / len(pkg.Scenes)
	}

	promptFields, promptFieldsPresent := 0, 0
	for _, scene := range pkg.Scenes {
		for _, field := range []string{scene.VisualConcept, scene.GeminiPrompt, scene.CameraMotion} {
			promptFields++
			if strings.TrimSpace(field) != "" {
				promptFieldsPresent++
			}
		}
	}
	avScore := 0
	if promptFields > 0 {
		avScore = promptFieldsPresent * 100 / promptFields
	}
	overall := (hookScore + nicheScore + persuasionScore + avScore) / 4
	verdict := "Còn thiếu trường kiểm tra"
	if overall >= 85 {
		verdict = "Đủ trường checklist"
	} else if overall < 50 {
		verdict = "Thiếu nhiều trường"
	}

	strengths := make([]string, 0, 4)
	if hookScore == 100 {
		strengths = append(strengths, "Hook có độ dài hợp lệ và nhắc sản phẩm")
	}
	if nicheScore >= 70 {
		strengths = append(strengths, "Các cảnh có từ khóa ngành hàng")
	}
	if strings.TrimSpace(pkg.CallToAction) != "" {
		strengths = append(strengths, "Có CTA trong kịch bản")
	}
	if avScore == 100 {
		strengths = append(strengths, "Mọi cảnh có visual concept, prompt ảnh và chuyển động camera")
	}
	recommendations := []string{
		"Checklist này chỉ kiểm tra văn bản kịch bản; chưa đánh giá video render, độ đúng sản phẩm/nhân vật, chất lượng hình/tiếng hoặc hiệu quả kênh.",
	}
	if hookScore < 100 {
		recommendations = append(recommendations, "Viết lại hook trong 15–120 ký tự và gọi đúng sản phẩm.")
	}
	if nicheScore < 70 {
		recommendations = append(recommendations, "Đưa chi tiết ngành hàng/sản phẩm vào hành động và prompt của các cảnh.")
	}
	if persuasionScore < 100 {
		recommendations = append(recommendations, "Bổ sung CTA hoặc lời thoại cho cảnh còn thiếu.")
	}
	if avScore < 100 {
		recommendations = append(recommendations, "Bổ sung visual concept, prompt ảnh và chuyển động camera cho từng cảnh.")
	}
	return QCScorecard{
		Scope:            "Script fields only",
		OverallScore:     overall,
		HookScore:        hookScore,
		NicheMatchScore:  nicheScore,
		PersuasionScore:  persuasionScore,
		AudioVisualScore: avScore,
		Verdict:          verdict,
		Summary:          fmt.Sprintf("Checklist đạt %d/100 theo 4 nhóm trường kịch bản; đây không phải điểm chất lượng video.", overall),
		Strengths:        strengths,
		Recommendations:  recommendations,
	}
}

func productTerms(text string) []string {
	seen := map[string]bool{}
	var terms []string
	for _, term := range strings.Fields(strings.ToLower(text)) {
		term = strings.Trim(term, `.,!?;:"'()[]{}-/`)
		if len([]rune(term)) < 3 || seen[term] {
			continue
		}
		seen[term] = true
		terms = append(terms, term)
	}
	return terms
}

func mentionsAny(text string, terms []string) bool {
	text = strings.ToLower(text)
	for _, term := range terms {
		if strings.Contains(text, term) {
			return true
		}
	}
	return false
}

func fillComplementaryPackageFields(pkg *AdScriptPackage, keyBenefits string) {
	cleanTag := sanitizeTag(pkg.ProductName)
	cleanNiche := sanitizeTag(pkg.Niche)
	facts := strings.TrimSpace(keyBenefits)
	parts := []string{pkg.HookText}
	if facts != "" && !strings.Contains(facts, "Chưa có lợi ích") {
		parts = append(parts, facts)
	}
	if strings.TrimSpace(pkg.CallToAction) != "" {
		parts = append(parts, pkg.CallToAction)
	}
	pkg.Caption = strings.Join(parts, "\n\n")
	pkg.Hashtags = nonEmptyTags(cleanTag, cleanNiche)

	platforms := []struct {
		key   string
		name  string
		title string
	}{
		{key: "tiktok", name: "TikTok", title: pkg.ProductName},
		{key: "youtube_shorts", name: "YouTube Shorts", title: pkg.ProductName},
		{key: "facebook_reels", name: "Facebook Reels", title: pkg.ProductName},
		{key: "threads", name: "Threads", title: pkg.ProductName},
	}
	pkg.PlatformPosts = make(map[string]PlatformPost, len(platforms))
	for _, platform := range platforms {
		pkg.PlatformPosts[platform.key] = PlatformPost{
			Platform:     platform.name,
			Title:        platform.title,
			Caption:      pkg.Caption,
			Hashtags:     append([]string(nil), pkg.Hashtags...),
			CallToAction: pkg.CallToAction,
			Tip:          "Chỉ đăng thông tin khớp ảnh listing và mô tả đã xác nhận.",
		}
	}
	pkg.PsychologicalTriggers = nil
}

func nonEmptyTags(values ...string) []string {
	var tags []string
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			tags = append(tags, "#"+value)
		}
	}
	return tags
}

func sanitizeTag(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.ReplaceAll(s, " ", "")
	s = strings.ReplaceAll(s, "-", "")
	s = strings.ReplaceAll(s, "/", "")
	s = strings.ReplaceAll(s, "&", "")
	return s
}
