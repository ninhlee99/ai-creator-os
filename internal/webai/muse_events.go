package webai

import (
	"encoding/json"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"
)

var museMediaURLRegex = regexp.MustCompile(`https?://[^\s"'<>)]+\.(?:mp4|webm|mov|png|jpg|jpeg|webp)(?:\?[^\s"'<>)]*)?`)

func museEventSeq(event map[string]any) uint64 {
	switch value := event["seq"].(type) {
	case float64:
		if value > 0 {
			return uint64(value)
		}
	case json.Number:
		parsed, _ := value.Int64()
		if parsed > 0 {
			return uint64(parsed)
		}
	}
	return 0
}

func museEventText(event map[string]any) string {
	payload, _ := event["payload"].(map[string]any)
	if value, ok := payload["display_text"].(string); ok && value != "" {
		return value
	}
	if value, ok := event["display_text"].(string); ok && value != "" {
		return value
	}
	if value, ok := payload["content"].(string); ok && value != "" {
		return value
	}
	if value, ok := event["text"].(string); ok {
		return value
	}
	return ""
}

func isMuseReply(event map[string]any, baseline uint64) bool {
	name, _ := event["event_name"].(string)
	if name != "message.assistant" || museEventSeq(event) <= baseline {
		return false
	}
	payload, _ := event["payload"].(map[string]any)
	if reply, _ := event["reply_to_message_id"].(string); reply != "" {
		return false
	}
	if reply, _ := payload["reply_to_message_id"].(string); reply != "" {
		return false
	}
	if ready, ok := payload["display_text_ready"].(bool); ok && !ready {
		return false
	}
	return strings.TrimSpace(museEventText(event)) != ""
}

func extractMuseMediaURLs(event map[string]any, text string) []string {
	serialized, _ := json.Marshal(event)
	content := string(serialized) + "\n" + text
	seen := map[string]bool{}
	var links []string

	matches := museMediaURLRegex.FindAllString(content, -1)
	for _, match := range matches {
		clean := strings.TrimRight(match, ".,;!?\"')}]")
		parsed, err := url.Parse(clean)
		if err != nil || (parsed.Scheme != "https" && parsed.Scheme != "http") {
			continue
		}
		host := strings.ToLower(parsed.Hostname())
		if host != "muse.ai" && host != "www.muse.ai" && !strings.HasSuffix(host, ".muse.ai") {
			continue
		}
		ext := strings.ToLower(filepath.Ext(parsed.Path))
		if ext == ".mp4" || ext == ".webm" || ext == ".mov" || ext == ".png" || ext == ".jpg" || ext == ".jpeg" || ext == ".webp" {
			if !seen[clean] {
				seen[clean] = true
				links = append(links, clean)
			}
		}
	}
	return links
}
