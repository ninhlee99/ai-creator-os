package web

import (
	"fmt"
	"github.com/ninhlee99/ai-creator-os/internal/network"
	"github.com/ninhlee99/ai-creator-os/internal/publishers"
	_ "modernc.org/sqlite" // registers the "sqlite" driver (pure Go, no cgo)
	"path/filepath"
)

// accountView is a network account plus fields needed by account templates.
type accountView struct {
	*network.Account
	TikTokConnected bool
	ModelPhotoURL   string
	HasModel        bool
}

func (s *Server) wrapAccount(a *network.Account) *accountView {
	av := &accountView{
		Account: a,
		TikTokConnected: publishers.NewTikTokPublisher(a.Username).IsConfigured(),
	}
	if s.Studio != nil {
		if ms, err := s.Studio.ModelLibrary(); err == nil && ms != nil {
			if photos, err := ms.List(a.ID); err == nil && len(photos) > 0 {
				av.HasModel = true
				av.ModelPhotoURL = fmt.Sprintf("/models/%d/%s", a.ID, filepath.Base(photos[0].Path))
			}
		}
	}
	return av
}
