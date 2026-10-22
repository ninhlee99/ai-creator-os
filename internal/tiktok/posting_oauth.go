package tiktok

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/url"
	"time"
)

func NewPKCE() (verifier, challenge string) {
	const alphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-._~"
	b := make([]byte, 64)
	_, _ = rand.Read(b)
	for i := range b {
		b[i] = alphabet[int(b[i])%len(alphabet)]
	}
	verifier = string(b)
	sum := sha256.Sum256(b)
	return verifier, hex.EncodeToString(sum[:])
}

// AuthorizeURL builds the Login Kit authorization URL. codeChallenge is
// optional ("" = no PKCE, the Web flow).
func (c *PostingClient) AuthorizeURL(redirectURI, scope, state, codeChallenge string) string {
	q := url.Values{}
	q.Set("client_key", c.ClientKey)
	q.Set("redirect_uri", redirectURI)
	q.Set("response_type", "code")
	q.Set("scope", scope)
	if state != "" {
		q.Set("state", state)
	}
	if codeChallenge != "" {
		q.Set("code_challenge", codeChallenge)
		q.Set("code_challenge_method", "S256")
	}
	return AuthURL + "?" + q.Encode()
}

// ExchangeCode trades an authorization code for tokens and persists them.
// codeVerifier is required when the authorize URL carried a challenge.
func (c *PostingClient) ExchangeCode(code, redirectURI, codeVerifier string) (map[string]any, error) {
	form := url.Values{}
	form.Set("client_key", c.ClientKey)
	form.Set("client_secret", c.ClientSecret)
	form.Set("grant_type", "authorization_code")
	form.Set("code", code)
	form.Set("redirect_uri", redirectURI)
	if codeVerifier != "" {
		form.Set("code_verifier", codeVerifier)
	}
	_, raw, err := c.http()("POST", Base+"/v2/oauth/token/",
		map[string]string{"Content-Type": "application/x-www-form-urlencoded"},
		[]byte(form.Encode()))
	if err != nil {
		return nil, err
	}
	data := decodeJSON(raw)
	t := Tokens{}
	if v, ok := data["access_token"].(string); ok {
		t.AccessToken = v
	}
	if v, ok := data["refresh_token"].(string); ok {
		t.RefreshToken = v
	}
	if v, ok := data["expires_in"].(float64); ok {
		t.ExpiresIn = int64(v)
	}
	// TikTok answers OAuth errors with HTTP 200 + {"error":...}; never
	// overwrite a working token file with an empty one.
	if t.AccessToken == "" || t.RefreshToken == "" {
		return nil, fmt.Errorf("oauth exchange failed: %v %v", data["error"], data["error_description"])
	}
	if err := c.Store.Save(t); err != nil {
		return nil, err
	}
	return data, nil
}

// AccessToken returns a fresh access token, refreshing (and persisting the
// rotated refresh_token) when the stored one is expired or near expiry.
func (c *PostingClient) AccessToken() (string, error) {
	t, err := c.Store.Load()
	if err != nil {
		return "", err
	}
	if t.AccessToken != "" && time.Now().Unix() < t.ObtainedAt+t.ExpiresIn-60 {
		return t.AccessToken, nil
	}
	if t.RefreshToken == "" {
		return "", fmt.Errorf("no refresh_token: complete OAuth flow first")
	}
	form := url.Values{}
	form.Set("client_key", c.ClientKey)
	form.Set("client_secret", c.ClientSecret)
	form.Set("grant_type", "refresh_token")
	form.Set("refresh_token", t.RefreshToken)
	_, raw, err := c.http()("POST", Base+"/v2/oauth/token/",
		map[string]string{"Content-Type": "application/x-www-form-urlencoded"},
		[]byte(form.Encode()))
	if err != nil {
		return "", err
	}
	data := decodeJSON(raw)
	nt := Tokens{}
	if v, ok := data["access_token"].(string); ok {
		nt.AccessToken = v
	}
	if v, ok := data["refresh_token"].(string); ok {
		nt.RefreshToken = v
	}
	if v, ok := data["expires_in"].(float64); ok {
		nt.ExpiresIn = int64(v)
	}
	if nt.AccessToken == "" {
		// Keep the old file: a failed refresh must not wipe the
		// refresh_token (that would force a full re-OAuth).
		return "", fmt.Errorf("oauth refresh returned no access_token: %v %v", data["error"], data["error_description"])
	}
	if nt.RefreshToken == "" {
		nt.RefreshToken = t.RefreshToken
	}
	// Persist the ROTATED refresh_token.
	if err := c.Store.Save(nt); err != nil {
		return "", err
	}
	return nt.AccessToken, nil
}

func (c *PostingClient) auth() (map[string]string, error) {
	tok, err := c.AccessToken()
	if err != nil {
		return nil, err
	}
	return map[string]string{
		"Authorization": "Bearer " + tok,
		"Content-Type":  "application/json; charset=UTF-8",
	}, nil
}

// CreatorInfo returns max duration / allowed privacy levels — call before init.
