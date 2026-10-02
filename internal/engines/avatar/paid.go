package avatar

import (
	"context"
	"fmt"
	"sync"
)

// paidProvider is the shared skeleton for paid avatar APIs (HeyGen, D-ID).
// The wire-up is pending: every call fails honestly until the provider's
// REST mapping is implemented and integration-tested. The struct already
// carries everything the real implementation needs (key rotation, enable
// flag) so plugging it in touches no other code.
type paidProvider struct {
	mu      sync.RWMutex
	name    string // "heygen" | "did"
	apiKeys []string
	enabled bool
	// keyIdx rotates across keys like the Gemini keyring.
	keyIdx int
}

func newPaidProvider(name string, apiKeys []string) *paidProvider {
	return &paidProvider{name: name, apiKeys: apiKeys, enabled: false}
}

func (p *paidProvider) Name() string { return p.name }

// SupportsRealtime reflects the paid product's capability (both vendors
// sell a streaming avatar API). It is true by contract; calls still fail
// until the provider is wired — see RenderClip/OpenStream.
func (p *paidProvider) SupportsRealtime() bool { return true }

func (p *paidProvider) SetAPIKey(k string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if k == "" {
		p.apiKeys = nil
	} else {
		p.apiKeys = []string{k}
	}
}

func (p *paidProvider) SetAPIKeys(keys []string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.apiKeys = append([]string(nil), keys...)
}

func (p *paidProvider) SetEnabled(b bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.enabled = b
}

func (p *paidProvider) key() (string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.apiKeys) == 0 {
		return "", fmt.Errorf("%s: no API key — thêm key trong Settings để bật tier này", p.name)
	}
	k := p.apiKeys[p.keyIdx%len(p.apiKeys)]
	p.keyIdx++
	return k, nil
}

// Healthy requires the tier to be enabled AND keyed. It does not call the
// vendor (cheap check, like the other providers).
func (p *paidProvider) Healthy(_ context.Context) bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.enabled && len(p.apiKeys) > 0
}

func (p *paidProvider) RenderClip(_ context.Context, _ Character, _ []byte, _ RenderOpts) (string, error) {
	if _, err := p.key(); err != nil {
		return "", err
	}
	// Intended REST mapping (to implement at wire-up):
	//   heygen: POST https://api.heygen.com/v2/video/generate
	//           {video_inputs: [{character: {type:"talking_photo", talking_photo_id},
	//            voice: {type:"text", input_text, voice_id}}], dimension:{width,height}}
	//           → poll GET /v2/video/status/{video_id} → video_url → download mp4.
	//           talking_photo_id is created once per character (POST /v2/talking_photo).
	//   did:    POST https://api.d-id.com/talks
	//           {source_url: <reference image>, script: {type:"audio", audio_url},
	//            config: {fluent, pad_audio}}
	//           → poll GET /talks/{id} → result_url → download mp4.
	return "", fmt.Errorf("%s: provider chưa được kết nối (wiring pending) — tier local vẫn phục vụ", p.name)
}

func (p *paidProvider) OpenStream(_ context.Context, _ Character, _ StreamOpts) (FrameStream, error) {
	if _, err := p.key(); err != nil {
		return nil, err
	}
	// Intended mapping:
	//   heygen: Streaming API — POST /v1/streaming.new → session_id,
	//           WebRTC/SDP exchange, StartTask with text/audio.
	//   did:    Agents API — POST /agents/{id}/chat with stream: true
	//           (WebRTC).
	return nil, fmt.Errorf("%s: streaming chưa được kết nối (wiring pending)", p.name)
}

// HeyGenProvider is the HeyGen paid tier (video generation + streaming API).
type HeyGenProvider struct{ *paidProvider }

// NewHeyGenProvider builds the HeyGen tier. It starts DISABLED; the owner
// enables it in Settings after adding an API key.
func NewHeyGenProvider(apiKeys []string) *HeyGenProvider {
	return &HeyGenProvider{newPaidProvider("heygen", apiKeys)}
}

// DIDProvider is the D-ID paid tier (Talks + Agents streaming API).
type DIDProvider struct{ *paidProvider }

// NewDIDProvider builds the D-ID tier. It starts DISABLED; the owner
// enables it in Settings after adding an API key.
func NewDIDProvider(apiKeys []string) *DIDProvider {
	return &DIDProvider{newPaidProvider("did", apiKeys)}
}
