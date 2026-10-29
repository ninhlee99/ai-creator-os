package webai

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"sync"
	"time"
)

type browserCookieSnapshotReader func(context.Context, *browserCDPClient) ([]BrowserCookie, error)

type browserCookieMonitor struct {
	cancel context.CancelFunc
	done   chan struct{}
	mu     sync.RWMutex
	latest []BrowserCookie
	err    error
}

// browserCookieMonitorInterval throttles background cookie snapshots so long
// video renders do not spam CDP Network.getAllCookies and SQLite WAL writes
// twice per second; final cookies are still captured synchronously on defer.
const browserCookieMonitorInterval = 5 * time.Second

func startBrowserCookieMonitor(ctx context.Context, client *browserCDPClient, initial []BrowserCookie, read browserCookieSnapshotReader, platform string, onCookieUpdate CookieSnapshotUpdater) (context.Context, *browserCookieMonitor) {
	monitorCtx, cancel := context.WithCancel(ctx)
	monitor := &browserCookieMonitor{cancel: cancel, done: make(chan struct{}), latest: normalizeCookieJar(initial)}
	go func() {
		defer close(monitor.done)
		ticker := time.NewTicker(browserCookieMonitorInterval)
		defer ticker.Stop()
		for {
			select {
			case <-monitorCtx.Done():
				return
			case <-ticker.C:
				readCtx, readCancel := context.WithTimeout(monitorCtx, 5*time.Second)
				latest, err := read(readCtx, client)
				readCancel()
				if err != nil {
					continue
				}
				latest = normalizeCookieJar(latest)
				if reflect.DeepEqual(latest, monitor.current()) {
					continue
				}
				monitor.mu.Lock()
				monitor.latest = append([]BrowserCookie(nil), latest...)
				monitor.mu.Unlock()
				if onCookieUpdate != nil {
					if err := onCookieUpdate(browserCookieHeaderForPlatform(platform, latest), latest); err != nil {
						monitor.mu.Lock()
						monitor.err = err
						monitor.mu.Unlock()
						cancel()
						return
					}
				}
			}
		}
	}()
	return monitorCtx, monitor
}

func (m *browserCookieMonitor) current() []BrowserCookie {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return append([]BrowserCookie(nil), m.latest...)
}

func (m *browserCookieMonitor) stop() error {
	m.cancel()
	<-m.done
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.err
}

func browserCookieSnapshotFromCDP(ctx context.Context, client *browserCDPClient) ([]BrowserCookie, error) {
	response, err := client.call(ctx, "Network.getAllCookies", map[string]any{}, client.sessionID)
	if err != nil {
		return nil, err
	}
	var result struct {
		Cookies []BrowserCookie `json:"cookies"`
	}
	if err := json.Unmarshal(response, &result); err != nil {
		return nil, err
	}
	cookies := normalizeCookieJar(result.Cookies)
	sort.SliceStable(cookies, func(i, j int) bool {
		if cookies[i].Domain != cookies[j].Domain {
			return cookies[i].Domain < cookies[j].Domain
		}
		if cookies[i].Path != cookies[j].Path {
			return cookies[i].Path < cookies[j].Path
		}
		return cookies[i].Name < cookies[j].Name
	})
	return cookies, nil
}

// BrowserCookieSetParams formats browser cookies for Network.setCookies CDP call.
func BrowserCookieSetParams(platform string, cookies []BrowserCookie) []map[string]any {
	return browserCookieSetParams(platform, cookies)
}

// ParseCookieHeaderToBrowserCookies parses a raw Cookie header into BrowserCookie structs.
func ParseCookieHeaderToBrowserCookies(header, defaultDomain string) []BrowserCookie {
	var jar []BrowserCookie
	parts := strings.Split(header, ";")
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		eq := strings.Index(part, "=")
		if eq <= 0 {
			continue
		}
		name := strings.TrimSpace(part[:eq])
		val := strings.TrimSpace(part[eq+1:])
		jar = append(jar, BrowserCookie{
			Name:   name,
			Value:  val,
			Domain: defaultDomain,
			Path:   "/",
			Secure: true,
		})
	}
	return jar
}

func browserCookieSetParams(platform string, cookies []BrowserCookie) []map[string]any {
	params := make([]map[string]any, 0, len(cookies))
	for _, cookie := range normalizeCookieJar(cookies) {
		param := map[string]any{
			"name": cookie.Name, "value": cookie.Value,
			"secure": cookie.Secure, "httpOnly": cookie.HTTPOnly,
		}
		if strings.HasPrefix(cookie.Name, "__Host-") {
			host := normalizeCookieDomain(cookie.Domain)
			if host == "" || !cookie.Secure || cookie.Path != "/" || strings.ContainsAny(host, "/:@?#") {
				continue
			}
			// __Host- cookies must be host-only. Using domain here makes
			// Chromium reject the entire Network.setCookies request.
			param["url"] = fmt.Sprintf("https://%s/", host)
		} else {
			param["domain"] = cookie.Domain
			param["path"] = cookie.Path
		}
		if !cookie.Session && cookie.Expires > 0 {
			param["expires"] = cookie.Expires
		}
		if cookie.SameSite != "" {
			param["sameSite"] = cookie.SameSite
		}
		if cookie.Priority != "" {
			param["priority"] = cookie.Priority
		}
		if cookie.SourceScheme != "" {
			param["sourceScheme"] = cookie.SourceScheme
		}
		if cookie.SourcePort > 0 {
			param["sourcePort"] = cookie.SourcePort
		}
		if cookie.SameParty {
			param["sameParty"] = true
		}
		if len(cookie.PartitionKey) > 0 {
			param["partitionKey"] = cookie.PartitionKey
		} else if cookie.Partitioned {
			if topLevelSite := partitionTopLevelSite(platform); topLevelSite != "" {
				param["partitionKey"] = map[string]any{"topLevelSite": topLevelSite, "hasCrossSiteAncestor": false}
			}
		}
		if cookie.PartitionKeyOpaque {
			param["partitionKeyOpaque"] = true
		}
		params = append(params, param)
	}
	return params
}

func browserCookieHeaderForPlatform(platform string, cookies []BrowserCookie) string {
	scope := BrowserCookieScope{Platform: platform}
	switch platform {
	case PlatformMuseAI:
		scope.CookieHost = "muse.ai"
	case PlatformGoogleFlow:
		scope.CookieHost = "flow.google.com"
	}
	return browserCookieHeaderAllPaths(cookies, scope)
}
