package growth

import (
	"context"
	"errors"
	"testing"
)

func TestTikTokNotConnectedWithoutUsername(t *testing.T) {
	_, err := TikTokSource{}.Fetch(context.Background(), SourceAccount{ID: 1, Username: ""})
	if !errors.Is(err, ErrNotConnected) {
		t.Fatalf("empty username must be ErrNotConnected, got %v", err)
	}
}

func TestTikTokNotConfigured(t *testing.T) {
	_, err := TikTokSource{}.Fetch(context.Background(), SourceAccount{ID: 1, Username: "nonexistent_acct_test"})
	if !errors.Is(err, ErrNotConnected) {
		t.Fatalf("unconfigured account must be ErrNotConnected, got %v", err)
	}
}
