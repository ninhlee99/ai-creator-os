package web

import (
	"fmt"
	"github.com/ninhlee99/ai-creator-os/internal/network"
	"github.com/ninhlee99/ai-creator-os/internal/publishers"
	_ "modernc.org/sqlite" // registers the "sqlite" driver (pure Go, no cgo)
	"path/filepath"
)

// accountView is a network account plus its rendered persona label and growth metrics.
type accountView struct {
	*network.Account
	PersonaLabel   string
	Platforms      []string
	GrowthStage    string
	Views30d       int64
	FollowersCnt   int64
	FollowersLabel string
	HasFollowers   bool
	NextTarget     string
	NextPct        float64
	HasNext        bool
	AffiliateLink  string
	ModelPhotoURL  string
	HasModel       bool
}

func personaLabel(key string) string {
	if p, ok := network.PERSONAS[key]; ok {
		return p.Label
	}
	if key == "" {
		return "—"
	}
	return key
}

func (s *Server) wrapAccount(a *network.Account) *accountView {
	ytTypes := a.YoutubeContentTypes
	if len(ytTypes) == 0 && fileExists(publishers.YouTubeTokenPath(a.Username)) {
		ytTypes = []string{"short_video", "short_film"}
		if s.Mgr != nil {
			ch := a.YoutubeChannel
			if ch == "" {
				ch = a.Username
			}
			go func(id int64, channel string) {
				_, _ = s.Mgr.SetYoutube(id, channel, []string{"short_video", "short_film"})
			}(a.ID, ch)
		}
	}
	var plats []string
	for _, p := range publishers.BuildPublishers(a.Username, a.YoutubeChannel, ytTypes) {
		plats = append(plats, p.Name())
	}
	av := &accountView{
		Account: a, PersonaLabel: personaLabel(a.Persona), Platforms: plats,
		FollowersLabel: "TikTok followers · legacy count",
	}
	if s.Growth != nil {
		if prof, err := s.Growth.EnsureProfile(a.ID, primaryPlatform(a), hasYouTube(a)); err == nil && prof != nil {
			switch prof.Stage {
			case "cold_start":
				av.GrowthStage = "Khởi động (0-1K)"
			case "monetization":
				av.GrowthStage = "Bật kiếm tiền / YPP"
			case "scaling":
				av.GrowthStage = "Mở rộng quy mô"
			default:
				av.GrowthStage = prof.Stage
			}
		}
		if snap, err := s.Growth.LatestSnapshot(a.ID); err == nil && snap != nil {
			if v, ok := metricValue(snap, "followers"); ok {
				av.FollowersCnt = int64(v)
				av.HasFollowers = true
				av.FollowersLabel = growthFollowersLabel(snap.Source)
			}
			if v, ok := metricValue(snap, "views_30d"); ok {
				av.Views30d = int64(v)
			}
		}
		if targets, err := s.Growth.ListTargets(a.ID); err == nil && len(targets) > 0 {
			for _, t := range targets {
				if t.ReachedAt == "" {
					av.NextTarget = t.Label
					av.HasNext = true
					if t.Threshold > 0 && av.FollowersCnt > 0 {
						av.NextPct = float64(av.FollowersCnt) / t.Threshold * 100
						if av.NextPct > 100 {
							av.NextPct = 100
						}
					}
					break
				}
			}
		}
	}
	if !av.HasFollowers && a.Followers > 0 {
		av.FollowersCnt = a.Followers
		av.HasFollowers = true
	}
	if av.GrowthStage == "" {
		av.GrowthStage = "Đang vận hành"
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
