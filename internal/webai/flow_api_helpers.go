package webai

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

func mergeFlowCookieHeader(header string, updates []*http.Cookie) string {
	type cookieUpdate struct {
		name    string
		value   string
		deleted bool
	}
	updateOrder := make([]string, 0, len(updates))
	latestUpdates := make(map[string]cookieUpdate, len(updates))
	for _, cookie := range updates {
		if cookie == nil || cookie.Name == "" {
			continue
		}
		if _, exists := latestUpdates[cookie.Name]; !exists {
			updateOrder = append(updateOrder, cookie.Name)
		}
		latestUpdates[cookie.Name] = cookieUpdate{
			name: cookie.Name, value: cookie.Value,
			deleted: cookie.Value == "" || cookie.MaxAge < 0 || (!cookie.Expires.IsZero() && cookie.Expires.Before(time.Now())),
		}
	}
	if len(latestUpdates) == 0 {
		return header
	}
	merged := make([]string, 0, strings.Count(header, ";")+len(latestUpdates)+1)
	for _, pair := range strings.Split(header, ";") {
		pair = strings.TrimSpace(pair)
		name, _, ok := strings.Cut(pair, "=")
		if !ok || name == "" {
			continue
		}
		if _, replaced := latestUpdates[name]; !replaced {
			merged = append(merged, pair)
		}
	}
	for _, name := range updateOrder {
		update := latestUpdates[name]
		if !update.deleted {
			merged = append(merged, update.name+"="+update.value)
		}
	}
	return strings.Join(merged, "; ")
}

func flowMediaID(workflow map[string]any) string {
	if value, ok := workflow["name"].(string); ok {
		return value
	}
	return findJSONKey(workflow, "mediaGenerationId")
}

func flowProjectID(workflow map[string]any) string { return findJSONKey(workflow, "projectId") }

func findJSONKey(value any, wanted string) string {
	switch item := value.(type) {
	case map[string]any:
		for key, child := range item {
			if key == wanted {
				if text, ok := child.(string); ok {
					return text
				}
			}
			if found := findJSONKey(child, wanted); found != "" {
				return found
			}
		}
	case []any:
		for _, child := range item {
			if found := findJSONKey(child, wanted); found != "" {
				return found
			}
		}
	}
	return ""
}

func findJSONArray(value any, wanted string) []any {
	switch item := value.(type) {
	case map[string]any:
		for key, child := range item {
			if key == wanted {
				if rows, ok := child.([]any); ok {
					return rows
				}
			}
			if rows := findJSONArray(child, wanted); rows != nil {
				return rows
			}
		}
	case []any:
		for _, child := range item {
			if rows := findJSONArray(child, wanted); rows != nil {
				return rows
			}
		}
	}
	return nil
}

func isGoogleMediaURL(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.User != nil {
		return false
	}
	host := strings.ToLower(u.Hostname())
	for _, suffix := range []string{".googleusercontent.com", ".googleapis.com", ".google.com", ".usercontent.goog", ".gvt1.com"} {
		if strings.HasSuffix(host, suffix) {
			return true
		}
	}
	return host == "googleusercontent.com" || host == "googleapis.com" || host == "flow-content.google"
}

func flowUUID() string {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return fmt.Sprintf("flow-%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(raw[:])
}
