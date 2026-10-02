//go:build parked

package avatar

import (
	"bytes"
	"context"
	"encoding/json"
	"time"
)

// DefaultAvatarChain wires the avatar provider chain:
//
//	local (free sidecar) -> heygen (paid, disabled) -> did (paid, disabled)
//
// cfgSrc may be nil; a non-empty saved config replaces the default.
// decide may be nil (failovers then stay silent).
func DefaultAvatarChain(dataDir string, decide DecisionLogger, heygenKeys, didKeys []string, cfgSrc func() ChainConfig) *AvatarChain {
	chain := NewAvatarChain([]AvatarVideoProvider{
		NewLocalAvatarProvider(dataDir),
		NewHeyGenProvider(heygenKeys),
		NewDIDProvider(didKeys),
	}, decide)
	cfg := DefaultAvatarConfig()
	if cfgSrc != nil {
		if saved := cfgSrc(); len(saved.Order) > 0 {
			cfg = saved
		}
	}
	chain.SetConfig(cfg)
	return chain
}

// ---------------------------------------------------------------------------
// sidecar lifecycle passthrough: the dashboard drives these through the
// web.AvatarSidecarCtl interface (see cmd/aicos adapter).

// Start launches the sidecar (see sidecarLifecycle.Start).
func (p *LocalAvatarProvider) Start(ctx context.Context) error { return p.lc.Start(ctx) }

// Stop terminates the sidecar.
func (p *LocalAvatarProvider) Stop() error { return p.lc.Stop() }

// Restart is Stop followed by Start.
func (p *LocalAvatarProvider) Restart(ctx context.Context) error { return p.lc.Restart(ctx) }

// SidecarStatus returns (state, detail) for the dashboard.
func (p *LocalAvatarProvider) SidecarStatus() (string, string) { return p.lc.Status() }

// EnsureModel downloads the weights (dashboard "tải model" button).
func (p *LocalAvatarProvider) EnsureModel(ctx context.Context, onProgress func(downloaded, total int64)) error {
	return p.lc.EnsureModel(ctx, onProgress)
}

// ModelConfigured reports whether AVATAR_MODEL_URL is set.
func (p *LocalAvatarProvider) ModelConfigured() bool { return p.lc.ModelConfigured() }

// ModelPresent reports whether weights are on disk.
func (p *LocalAvatarProvider) ModelPresent() bool { return p.lc.ModelPresent() }

// ---------------------------------------------------------------------------
// chain JSON contract: the dashboard stores the avatar chain with the same
// shape as the TTS/LLM chains: {"order":[{name,enabled,api_key,api_keys,
// timeout (seconds),retries}]}. Unknown fields are ignored; empty input
// yields an empty config.

type avatarChainJSON struct {
	Order []struct {
		Name    string   `json:"name"`
		Enabled bool     `json:"enabled"`
		APIKey  string   `json:"api_key"`
		APIKeys []string `json:"api_keys"`
		Timeout int      `json:"timeout"` // seconds
		Retries int      `json:"retries"`
	} `json:"order"`
}

// ParseAvatarChainJSON decodes a dashboard avatar chain config.
func ParseAvatarChainJSON(raw json.RawMessage) (ChainConfig, error) {
	var cj avatarChainJSON
	if len(bytes.TrimSpace(raw)) == 0 {
		return ChainConfig{}, nil
	}
	if err := json.Unmarshal(raw, &cj); err != nil {
		return ChainConfig{}, err
	}
	cfg := ChainConfig{}
	for _, e := range cj.Order {
		cfg.Order = append(cfg.Order, ProviderEntry{
			Name:    e.Name,
			Enabled: e.Enabled,
			APIKey:  e.APIKey,
			APIKeys: e.APIKeys,
			Timeout: time.Duration(e.Timeout) * time.Second,
			Retries: e.Retries,
		})
	}
	return cfg, nil
}
