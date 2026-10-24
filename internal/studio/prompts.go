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

//go:embed prompts/*.txt
var promptFS embed.FS

// promptTemplates caches parsed templates. Studio jobs render in background
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

// promptData carries the fields used by affiliate and viral TikTok prompts.
type promptData struct {
	Topic            string
	Niche            string
	ChannelProfile   string
	ChannelLearning  string
	ProductCategory  string
	ProductUse       string
	ProductUseNotes  string
	ProductFocus     string
	ProductEvidence  string
	ReferenceSummary string
	ReferenceBeats   string
	Seconds          int
	Duration         string
	N                int
}
