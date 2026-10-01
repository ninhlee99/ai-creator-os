package tts

import (
	"strings"
	"sync"
	"time"
)

// Backoff ladder for rate-limited keys: 60s -> 5m -> 15m, then capped.
// A key that succeeds resets to the base level.
var keyBackoffs = []time.Duration{time.Minute, 5 * time.Minute, 15 * time.Minute}

// KeyErrorKind classifies a provider call failure for key management.
type KeyErrorKind int

const (
	// KeyErrOther is any non-key error (network, 5xx, parse...): try the
	// next key, but do not penalize the current one.
	KeyErrOther KeyErrorKind = iota
	// KeyErrQuota is 429 / quota / rate-limit / RESOURCE_EXHAUSTED: the key
	// cools down with progressive backoff.
	KeyErrQuota
	// KeyErrInvalidKey is a rejected API key (API_KEY_INVALID / 401 / 403):
	// the key is marked invalid until the configuration changes.
	KeyErrInvalidKey
)

// ClassifyKeyError inspects a provider error and decides what it means for
// the key that produced it. It works on the error text because the Gemini
// HTTP helpers surface status codes as "HTTP %d: ..." strings.
//
// NOTE: the "rate" substring is intentionally NOT matched on its own — it
// appears inside innocent words ("separate", "moderate"). We match the
// explicit rate-limit spellings instead.
func ClassifyKeyError(err error) KeyErrorKind {
	if err == nil {
		return KeyErrOther
	}
	s := strings.ToLower(err.Error())
	// Invalid key first: a 401 body can also mention quota-adjacent words.
	switch {
	case strings.Contains(s, "api_key_invalid") || strings.Contains(s, "api-key-invalid"):
		return KeyErrInvalidKey
	case (strings.Contains(s, "401") || strings.Contains(s, "403")) &&
		(strings.Contains(s, "api key") || strings.Contains(s, "apikey") || strings.Contains(s, "key")):
		return KeyErrInvalidKey
	case strings.Contains(s, "400") && strings.Contains(s, "api key"):
		return KeyErrInvalidKey
	}
	switch {
	case strings.Contains(s, "429") || strings.Contains(s, "quota") ||
		strings.Contains(s, "rate limit") || strings.Contains(s, "rate-limit") ||
		strings.Contains(s, "ratelimit") || strings.Contains(s, "resource_exhausted"):
		return KeyErrQuota
	default:
		return KeyErrOther
	}
}

// KeyStatus is the dashboard-facing state of one API key. The full key is
// NEVER exposed here or in logs — only the last 4 characters.
type KeyStatus struct {
	Index int    `json:"index"`
	Last4 string `json:"last4"`
	// State is "ok", "cooldown" or "invalid".
	State                string `json:"state"`
	CooldownRemainingSec int64  `json:"cooldown_remaining_sec"`
	RateLimitHits        int    `json:"rate_limit_hits"`
	// Requests counts how many times this key was handed out for a request.
	Requests int `json:"requests"`
	// LastRateLimitUnix is the Unix time of the most recent rate-limit hit
	// (0 = never).
	LastRateLimitUnix int64 `json:"last_rate_limit_unix"`
}

type keySlot struct {
	key           string
	cooldownUntil time.Time
	backoffIdx    int // index into keyBackoffs
	rateHits      int
	uses          int       // observability: times handed out by Next
	lastRateAt    time.Time // observability: most recent rate-limit hit
	invalid       bool
}

// KeyRing holds several API keys for one provider and rotates through them
// round-robin, skipping keys that are cooling down or marked invalid.
// It is the mechanism behind "nhiều Gemini API key để nhân quota":
// every request uses the next usable key, so N keys multiply the free tier.
type KeyRing struct {
	mu   sync.Mutex
	keys []keySlot
	pos  int // round-robin cursor: the next request starts here
}

// NewKeyRing builds a ring over keys (order = rotation order).
func NewKeyRing(keys []string) *KeyRing {
	r := &KeyRing{}
	r.SetKeys(keys)
	return r
}

// SetKeys replaces the key set at runtime (dashboard save / env reload).
// Per-key state (cooldown, backoff level, invalid flag) is preserved for
// keys that survive the change, matched by value; brand-new keys start
// fresh. The rotation cursor resets.
func (r *KeyRing) SetKeys(keys []string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	keep := make(map[string]keySlot, len(r.keys))
	for _, s := range r.keys {
		if _, dup := keep[s.key]; !dup {
			keep[s.key] = s
		}
	}
	r.keys = r.keys[:0]
	seen := map[string]bool{}
	for _, k := range keys {
		k = strings.TrimSpace(k)
		if k == "" || seen[k] {
			continue
		}
		seen[k] = true
		if s, ok := keep[k]; ok {
			r.keys = append(r.keys, s)
		} else {
			r.keys = append(r.keys, keySlot{key: k})
		}
	}
	r.pos = 0
}

// Len returns the number of configured keys.
func (r *KeyRing) Len() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.keys)
}

// Key returns the raw key at idx (for ValidateKey-style probes).
func (r *KeyRing) Key(idx int) (string, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if idx < 0 || idx >= len(r.keys) {
		return "", false
	}
	return r.keys[idx].key, true
}

// HasUsable reports whether at least one key is not marked invalid.
// (Cooling-down keys are transient — they do not count as unusable here;
// Next simply skips them.)
func (r *KeyRing) HasUsable() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, s := range r.keys {
		if !s.invalid {
			return true
		}
	}
	return false
}

// AllCoolingDown reports whether every usable key is currently cooling
// down. When true the provider must fail over to the next tier instead of
// retrying — hammering would only extend the cooldowns.
func (r *KeyRing) AllCoolingDown() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.keys) == 0 {
		return false
	}
	now := time.Now()
	for _, s := range r.keys {
		if s.invalid {
			continue
		}
		if !now.Before(s.cooldownUntil) {
			return false
		}
	}
	return true
}

// Next returns the next usable key in round-robin order, skipping keys in
// tried, marked invalid, or cooling down. ok=false means no key can serve
// right now.
func (r *KeyRing) Next(tried map[int]bool) (key string, idx int, ok bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	n := len(r.keys)
	if n == 0 {
		return "", 0, false
	}
	now := time.Now()
	for i := 0; i < n; i++ {
		j := (r.pos + i) % n
		if tried != nil && tried[j] {
			continue
		}
		s := &r.keys[j]
		if s.invalid || now.Before(s.cooldownUntil) {
			continue
		}
		r.pos = (j + 1) % n
		s.uses++ // observability only; rotation order is unchanged
		return s.key, j, true
	}
	return "", 0, false
}

// ReportSuccess resets the key's backoff ladder (a healthy key starts over
// at 60s if it ever gets rate-limited again).
func (r *KeyRing) ReportSuccess(idx int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if idx < 0 || idx >= len(r.keys) {
		return
	}
	r.keys[idx].backoffIdx = 0
}

// ReportRateLimit puts the key in cooldown with progressive backoff:
// 60s, then 5m, then 15m (capped).
func (r *KeyRing) ReportRateLimit(idx int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if idx < 0 || idx >= len(r.keys) {
		return
	}
	s := &r.keys[idx]
	s.cooldownUntil = time.Now().Add(keyBackoffs[s.backoffIdx])
	if s.backoffIdx < len(keyBackoffs)-1 {
		s.backoffIdx++
	}
	s.rateHits++
	s.lastRateAt = time.Now()
}

// ReportInvalid marks the key as permanently broken until the key set
// changes (user edits the config).
func (r *KeyRing) ReportInvalid(idx int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if idx < 0 || idx >= len(r.keys) {
		return
	}
	r.keys[idx].invalid = true
}

// Status returns the dashboard view of every key. Full keys are never
// exposed — only the last 4 characters.
func (r *KeyRing) Status() []KeyStatus {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := time.Now()
	out := make([]KeyStatus, 0, len(r.keys))
	for i, s := range r.keys {
		st := KeyStatus{
			Index:         i,
			Last4:         last4(s.key),
			State:         "ok",
			RateLimitHits: s.rateHits,
			Requests:      s.uses,
		}
		if !s.lastRateAt.IsZero() {
			st.LastRateLimitUnix = s.lastRateAt.Unix()
		}
		switch {
		case s.invalid:
			st.State = "invalid"
		case now.Before(s.cooldownUntil):
			st.State = "cooldown"
			st.CooldownRemainingSec = int64(time.Until(s.cooldownUntil).Seconds())
		}
		out = append(out, st)
	}
	return out
}

// last4 returns the last 4 characters of a key for safe display/logging.
func last4(key string) string {
	if len(key) <= 4 {
		return key
	}
	return key[len(key)-4:]
}
