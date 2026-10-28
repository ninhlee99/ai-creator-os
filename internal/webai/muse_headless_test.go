package webai

import "testing"

func TestMuseConversationURLRequiresThreadID(t *testing.T) {
	tests := []struct {
		name string
		url  string
		want string
	}{
		{name: "thread route", url: "https://muse.ai/thread/abc123", want: "https://muse.ai/thread/abc123"},
		{name: "thread route with query", url: "https://www.muse.ai/thread/abc123?mode=video", want: "https://www.muse.ai/thread/abc123?mode=video"},
		{name: "new chat root", url: "https://muse.ai/"},
		{name: "non-thread route", url: "https://muse.ai/settings"},
		{name: "missing thread ID", url: "https://muse.ai/thread/"},
		{name: "wrong host", url: "https://evil.example/thread/abc123"},
		{name: "insecure route", url: "http://muse.ai/thread/abc123"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := museConversationURL(tt.url); got != tt.want {
				t.Fatalf("museConversationURL(%q) = %q, want %q", tt.url, got, tt.want)
			}
		})
	}
}
