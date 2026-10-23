package webai

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// CookieSnapshotHTTPClient sends provider cookies with their saved scope and
// persists Set-Cookie changes before returning each response to its caller.
type CookieSnapshotHTTPClient struct {
	client *http.Client
	jar    *cookieSnapshotJar
}

func NewCookieSnapshotHTTPClient(platform, cookie string, cookies []BrowserCookie, timeout time.Duration, updater CookieSnapshotUpdater) (*CookieSnapshotHTTPClient, error) {
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	if len(cookies) == 0 && strings.TrimSpace(cookie) != "" {
		cookies = mergeCookieJarHeader(nil, cookie, platform)
	}
	jar, err := newCookieSnapshotJar(platform, cookie, cookies, updater)
	if err != nil {
		return nil, err
	}
	client := &http.Client{Timeout: timeout}
	if jar != nil {
		client.Jar = jar
		client.CheckRedirect = func(_ *http.Request, via []*http.Request) error {
			if len(via) >= 10 {
				return http.ErrUseLastResponse
			}
			return jar.peekError()
		}
	}
	return &CookieSnapshotHTTPClient{client: client, jar: jar}, nil
}

func (c *CookieSnapshotHTTPClient) Do(req *http.Request) (*http.Response, error) {
	if c == nil || c.client == nil {
		return nil, errors.New("provider cookie client is unavailable")
	}
	resp, err := c.client.Do(req)
	if err != nil {
		if persistErr := c.takeError(); persistErr != nil {
			return nil, fmt.Errorf("persist refreshed provider cookies: %w", persistErr)
		}
		return nil, err
	}
	if persistErr := c.takeError(); persistErr != nil {
		_ = resp.Body.Close()
		return nil, fmt.Errorf("persist refreshed provider cookies: %w", persistErr)
	}
	return resp, nil
}

func (c *CookieSnapshotHTTPClient) Snapshot() (string, []BrowserCookie) {
	if c == nil || c.jar == nil {
		return "", nil
	}
	return c.jar.CookieHeader(), c.jar.Snapshot()
}

func (c *CookieSnapshotHTTPClient) takeError() error {
	if c == nil || c.jar == nil {
		return nil
	}
	return c.jar.takeError()
}
