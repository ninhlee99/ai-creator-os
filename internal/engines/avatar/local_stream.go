//go:build parked

package avatar

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"os"
	"sync"
	"time"
)

func (p *LocalAvatarProvider) OpenStream(ctx context.Context, ch Character, opts StreamOpts) (FrameStream, error) {
	if !p.Healthy(ctx) {
		return nil, fmt.Errorf("local avatar: sidecar not running at %s — start it from the dashboard", p.baseURL)
	}
	img, err := os.ReadFile(ch.ReferenceImage)
	if err != nil {
		return nil, fmt.Errorf("local avatar: read reference image: %w", err)
	}
	seed := ch.Seed
	if opts.Seed != 0 {
		seed = opts.Seed
	}
	var opened openResp
	err = p.post(ctx, "/stream/open", openReq{
		AvatarID:          fmt.Sprintf("char-%d", ch.ID),
		ReferenceImageB64: base64.StdEncoding.EncodeToString(img),
		Seed:              seed,
		IdentityLock:      ComputeIdentityLock(img, seed),
		Width:             orDefault(opts.Width, 720),
		Height:            orDefault(opts.Height, 1280),
		FPS:               orDefault(opts.FPS, 25),
	}, &opened)
	if err != nil {
		return nil, fmt.Errorf("local avatar: stream open: %w", err)
	}
	if opened.SessionID == "" {
		return nil, fmt.Errorf("local avatar: sidecar returned no session_id")
	}
	return &sidecarStream{p: p, sessionID: opened.SessionID}, nil
}

// sidecarStream implements FrameStream over the sidecar's stream endpoints.
type sidecarStream struct {
	p         *LocalAvatarProvider
	sessionID string
	closed    bool
	mu        sync.Mutex
}

func (s *sidecarStream) WriteAudio(pcm []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return fmt.Errorf("stream closed")
	}
	if len(pcm) == 0 {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	var ack map[string]any
	return s.p.post(ctx, "/stream/push", pushReq{
		SessionID: s.sessionID,
		PCMB64:    base64.StdEncoding.EncodeToString(pcm),
	}, &ack)
}

func (s *sidecarStream) ReadFrame(ctx context.Context) ([]byte, error) {
	s.mu.Lock()
	closed := s.closed
	s.mu.Unlock()
	if closed {
		return nil, fmt.Errorf("stream closed")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		s.p.baseURL+"/stream/frame?session_id="+s.sessionID, nil)
	if err != nil {
		return nil, err
	}
	resp, err := s.p.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNoContent {
		return nil, errNoFrameYet
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("stream frame: HTTP %d", resp.StatusCode)
	}
	frame, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, err
	}
	if len(frame) == 0 {
		return nil, errNoFrameYet
	}
	return frame, nil
}

func (s *sidecarStream) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}
	s.closed = true
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var ack map[string]any
	_ = s.p.post(ctx, "/stream/close", map[string]string{"session_id": s.sessionID}, &ack)
	return nil
}

// errNoFrameYet signals the renderer has not produced this frame yet; the
// caller should wait for the next tick, not treat it as fatal.
var errNoFrameYet = fmt.Errorf("no frame yet")

// ---------------------------------------------------------------------------
// HTTP plumbing
