package web

import (
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ninhlee99/ai-creator-os/internal/studio"
)

func TestAccountCreationLeavesCharacterOptional(t *testing.T) {
	s := newTestServer(t)
	st, err := studio.New(filepath.Join(s.DataDir, "studio.db"), nil, nil, nil, s.DataDir)
	if err != nil {
		t.Fatalf("studio.New: %v", err)
	}
	defer st.Close()
	s.Studio = st

	newPage := get(t, s, "/accounts/new")
	if newPage.Code != 200 || !strings.Contains(newPage.Body.String(), `name="model_photo"`) {
		t.Fatalf("new-account page must offer optional character photo upload")
	}

	rec := postForm(t, s, "/accounts", url.Values{
		"username":   {"@channel_without_character"},
		"niche_hint": {"review đồ gia dụng"},
	})
	if rec.Code != 303 {
		t.Fatalf("POST /accounts = %d, want 303", rec.Code)
	}
	accounts, err := s.Mgr.List()
	if err != nil || len(accounts) != 1 {
		t.Fatalf("accounts = %v, err = %v", accounts, err)
	}
	library, err := st.ModelLibrary()
	if err != nil {
		t.Fatalf("ModelLibrary: %v", err)
	}
	photos, err := library.List(accounts[0].ID)
	if err != nil || len(photos) != 0 {
		t.Fatalf("unselected character photos = %v, err = %v; want none", photos, err)
	}
}
