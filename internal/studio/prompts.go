package studio

// Director prompts live in versioned text files (P2-2), not hardcoded in
// Go: tuning the director's craft no longer needs a code change. The files
// are embedded so the single binary still ships with zero external assets.

import (
	"bytes"
	"embed"
	"fmt"
	"text/template"
)

//go:embed prompts/*.txt
var promptFS embed.FS

var promptTemplates = map[string]*template.Template{}

func directorPrompt(name string, data any) (string, error) {
	tpl, ok := promptTemplates[name]
	if !ok {
		raw, err := promptFS.ReadFile("prompts/" + name)
		if err != nil {
			return "", fmt.Errorf("director prompt %s: %w", name, err)
		}
		tpl, err = template.New(name).Parse(string(raw))
		if err != nil {
			return "", fmt.Errorf("director prompt %s: %w", name, err)
		}
		promptTemplates[name] = tpl
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
	Recap1       string
	Recap2       string
}
