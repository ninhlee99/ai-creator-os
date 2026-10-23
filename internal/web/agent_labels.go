package web

import (
	_ "modernc.org/sqlite" // registers the "sqlite" driver (pure Go, no cgo)
	"strings"
)

// agentMeta is the display form of an agent slug in the decision feed.
type agentMeta struct {
	Icon  string
	Label string
}

// agentLabels maps raw agent slugs (ledger.decisions.agent) to Vietnamese
// display labels. Unknown slugs fall back to a humanized form so nothing
// renders with underscores (R2-11).
var agentLabels = map[string]agentMeta{
	"growth_director":  {Icon: "🌱", Label: "Đạo diễn tăng trưởng"},
	"growth_analyst":   {Icon: "📊", Label: "Phân tích tăng trưởng"},
	"growth_publisher": {Icon: "📤", Label: "Đăng bài tăng trưởng"},
	"story_publisher":  {Icon: "📖", Label: "Đăng truyện"},
	"growth":           {Icon: "🌱", Label: "Tăng trưởng"},
	"ai_orchestrator":  {Icon: "🎬", Label: "Điều phối Studio"},
	"harness":          {Icon: "🧭", Label: "Harness OODA"},
	"live_planner":     {Icon: "📅", Label: "Lập lịch live"},
	"scheduler":        {Icon: "🗓", Label: "Xếp lịch"},
	"governance":       {Icon: "🛡", Label: "Kiểm soát"},
	"governor":         {Icon: "🛡", Label: "Kiểm soát"},
	"analyst":          {Icon: "📊", Label: "Phân tích"},
	"hunter":           {Icon: "🔎", Label: "Săn sản phẩm"},
	"director":         {Icon: "🎬", Label: "Đạo diễn"},
	"producer":         {Icon: "🎥", Label: "Sản xuất"},
	"content":          {Icon: "✍️", Label: "Nội dung"},
	"onboarding":       {Icon: "🚀", Label: "Thiết lập"},
	"orchestrator":     {Icon: "🤖", Label: "Điều phối"},
	"engines":          {Icon: "⚙️", Label: "Động cơ AI"},
	"system":           {Icon: "⚙️", Label: "Hệ thống"},
	"human":            {Icon: "👤", Label: "Người dùng"},
	"test":             {Icon: "🧪", Label: "Kiểm thử"},
}

func agentLabel(slug string) agentMeta {
	if m, ok := agentLabels[slug]; ok {
		return m
	}
	if slug == "" {
		return agentMeta{Icon: "⚙️", Label: "—"}
	}
	return agentMeta{Icon: "⚙️", Label: strings.ReplaceAll(slug, "_", " ")}
}
