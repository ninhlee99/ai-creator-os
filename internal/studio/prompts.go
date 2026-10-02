package studio

// Director prompts live in versioned text files (P2-2), not hardcoded in
// Go: tuning the director's craft no longer needs a code change. The files
// are embedded so the single binary still ships with zero external assets.

import (
	"bytes"
	"embed"
	"fmt"
	"sync"
	"text/template"
)

//go:embed prompts/*.txt prompts/parked/*.txt
var promptFS embed.FS

// promptTemplates caches parsed templates. Film jobs render in background
// goroutines (up to maxConcurrentStudioJobs), so the cache is mutex-guarded
// — two jobs must never write the map concurrently.
var (
	promptTemplates   = map[string]*template.Template{}
	promptTemplatesMu sync.Mutex
)

func directorPrompt(name string, data any) (string, error) {
	promptTemplatesMu.Lock()
	tpl, ok := promptTemplates[name]
	promptTemplatesMu.Unlock()
	if !ok {
		raw, err := promptFS.ReadFile("prompts/" + name)
		if err != nil {
			// Prompt đã park (PIVOT 2026-10-02) nằm ở prompts/parked/ —
			// thử đó trước khi báo lỗi để build parked vẫn chạy được.
			raw, err = promptFS.ReadFile("prompts/parked/" + name)
		}
		if err != nil {
			return "", fmt.Errorf("director prompt %s: %w", name, err)
		}
		tpl, err = template.New(name).Parse(string(raw))
		if err != nil {
			return "", fmt.Errorf("director prompt %s: %w", name, err)
		}
		promptTemplatesMu.Lock()
		promptTemplates[name] = tpl
		promptTemplatesMu.Unlock()
	}
	var sb bytes.Buffer
	if err := tpl.Execute(&sb, data); err != nil {
		return "", fmt.Errorf("director prompt %s: %w", name, err)
	}
	return sb.String(), nil
}

// promptData carries the fields every film/affiliate prompt template needs.
type promptData struct {
	Topic        string
	Niche        string
	Genre        string
	Seconds      int
	TotalSeconds int
	ActSeconds   int
	N            int
	Orient       string
	Characters   string
	// Cast là dàn nhân vật pha 1 đã chốt (tên — trích xác định từ thoại các
	// hồi trước), truyền cho các hồi sau để giữ tên nhất quán.
	Cast string
	// Screenplay là kịch bản pha 1 (JSON) đưa vào prompt breakdown pha 2.
	Screenplay string
	// Story là truyện pha 0 (văn xuôi) đưa vào prompt chuyển thể pha 1.
	Story string
	// StoryWords là độ dài truyện mục tiêu (từ) cho pha 0.
	StoryWords int
	Recap1     string
	Recap2     string
}
