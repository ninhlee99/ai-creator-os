package studio

import (
	"context"
	"errors"
)

// ErrMuseVideoAPIUnavailable marks Muse video generation as a manual handoff
// until an official, supported video-generation API is available.
var ErrMuseVideoAPIUnavailable = errors.New("muse ai: no verified video-generation API; use manual export/import")

// MuseAIClient remains as a compatibility boundary for callers. It does not
// retain credentials or send requests to undocumented endpoints.
type MuseAIClient struct{}

func NewMuseAIClient(_, _ string) *MuseAIClient { return &MuseAIClient{} }

// SetCookieHeader is retained for source compatibility and intentionally does nothing.
func (*MuseAIClient) SetCookieHeader(string) {}

// Healthy reports false because no supported video-generation API is wired.
func (*MuseAIClient) Healthy() bool { return false }

// RenderMuseAIVideo fails explicitly instead of sending credentials or media
// to an undocumented endpoint.
func (*MuseAIClient) RenderMuseAIVideo(context.Context, string, string, int, string, string, func(string)) error {
	return ErrMuseVideoAPIUnavailable
}
