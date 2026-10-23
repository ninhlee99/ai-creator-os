package webai

import (
	"encoding/json"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// cookieSnapshotJar applies the standard cookie host/path/secure rules while
// retaining cookie metadata for backup and account-store updates.
type cookieSnapshotJar struct {
	mu       sync.RWMutex
	writeMu  sync.Mutex
	platform string
	jar      http.CookieJar
	cookies  []BrowserCookie
	updater  CookieSnapshotUpdater
	lastErr  error
}

func newCookieSnapshotJar(platform, header string, cookies []BrowserCookie, updater CookieSnapshotUpdater) (*cookieSnapshotJar, error) {
	if len(cookies) == 0 {
		return nil, nil
	}
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, err
	}
	snapshot := mergeCookieHeaderIntoSnapshot(cookies, header, platform)
	tracked := &cookieSnapshotJar{platform: platform, jar: jar, cookies: snapshot, updater: updater}
	for _, cookie := range snapshot {
		tracked.seed(cookie)
	}
	return tracked, nil
}

func mergeCookieHeaderIntoSnapshot(cookies []BrowserCookie, header, platform string) []BrowserCookie {
	snapshot := normalizeCookieJar(cookies)
	defaultDomain := cookieDefaultDomain(platform)
	headerPairs := strings.Split(header, ";")
	headerCounts := make(map[string]int, len(headerPairs))
	snapshotCounts := make(map[string]int, len(snapshot))
	for _, cookie := range snapshot {
		snapshotCounts[cookie.Name]++
	}
	for _, pair := range headerPairs {
		name, _, ok := strings.Cut(strings.TrimSpace(pair), "=")
		if ok && strings.TrimSpace(name) != "" {
			headerCounts[strings.TrimSpace(name)]++
		}
	}
	for _, pair := range strings.Split(header, ";") {
		name, value, ok := strings.Cut(strings.TrimSpace(pair), "=")
		name = strings.TrimSpace(name)
		if !ok || name == "" {
			continue
		}
		if snapshotCounts[name] == 1 && headerCounts[name] == 1 {
			for i := range snapshot {
				if snapshot[i].Name == name {
					snapshot[i].Value = value
					break
				}
			}
			continue
		}
		if snapshotCounts[name] == 0 && defaultDomain != "" {
			snapshot = append(snapshot, BrowserCookie{Name: name, Value: value, Domain: defaultDomain, Path: "/", Secure: true, Session: true})
			snapshotCounts[name]++
		}
	}
	return normalizeCookieJar(snapshot)
}

func (j *cookieSnapshotJar) seed(cookie BrowserCookie) {
	if cookie.Partitioned || len(cookie.PartitionKey) > 0 || cookie.PartitionKeyOpaque {
		return
	}
	domain := strings.TrimPrefix(strings.TrimSpace(cookie.Domain), ".")
	if domain == "" {
		return
	}
	path := cookie.Path
	if path == "" {
		path = "/"
	}
	u := &url.URL{Scheme: "https", Host: domain, Path: path}
	httpCookie := &http.Cookie{
		Name: cookie.Name, Value: cookie.Value, Path: path,
		Secure: cookie.Secure, HttpOnly: cookie.HTTPOnly,
		SameSite: httpCookieSameSite(cookie.SameSite),
	}
	if strings.HasPrefix(strings.TrimSpace(cookie.Domain), ".") {
		httpCookie.Domain = domain
	}
	if !cookie.Session && cookie.Expires > 0 {
		httpCookie.Expires = time.Unix(int64(cookie.Expires), 0)
	}
	j.jar.SetCookies(u, []*http.Cookie{httpCookie})
}

func (j *cookieSnapshotJar) Cookies(u *url.URL) []*http.Cookie {
	if j == nil || u == nil {
		return nil
	}
	cookies := j.jar.Cookies(u)
	for _, cookie := range j.Snapshot() {
		if !partitionedCookieMatches(cookie, u, j.platform) {
			continue
		}
		cookies = append(cookies, &http.Cookie{Name: cookie.Name, Value: cookie.Value})
	}
	return cookies
}

func partitionedCookieMatches(cookie BrowserCookie, requestURL *url.URL, platform string) bool {
	if requestURL == nil || (!cookie.Partitioned && len(cookie.PartitionKey) == 0) || cookie.PartitionKeyOpaque || cookie.Name == "" {
		return false
	}
	var key struct {
		TopLevelSite string `json:"topLevelSite"`
	}
	if len(cookie.PartitionKey) > 0 {
		if json.Unmarshal(cookie.PartitionKey, &key) != nil {
			return false
		}
	} else {
		key.TopLevelSite = partitionTopLevelSite(platform)
	}
	if !partitionSiteMatches(key.TopLevelSite, platform) {
		return false
	}
	requestHost := strings.ToLower(strings.TrimSuffix(requestURL.Hostname(), "."))
	domain := strings.ToLower(strings.TrimPrefix(strings.TrimSpace(cookie.Domain), "."))
	hostOnly := !strings.HasPrefix(strings.TrimSpace(cookie.Domain), ".")
	if domain == "" || !cookieDomainAcceptsHost(domain, hostOnly, requestHost) || !museCookiePathMatches(cookie.Path, requestURL.Path) {
		return false
	}
	if cookie.Secure && requestURL.Scheme != "https" {
		return false
	}
	return cookie.Session || cookie.Expires <= 0 || cookie.Expires > float64(time.Now().Unix())
}

func partitionSiteMatches(raw, platform string) bool {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Scheme != "https" {
		return false
	}
	host := strings.ToLower(strings.TrimSuffix(parsed.Hostname(), "."))
	switch platform {
	case PlatformGoogleFlow:
		return host == "google.com" || strings.HasSuffix(host, ".google.com")
	case PlatformMuseAI:
		return host == "muse.ai" || strings.HasSuffix(host, ".muse.ai")
	default:
		return false
	}
}

func partitionTopLevelSite(platform string) string {
	switch platform {
	case PlatformGoogleFlow:
		return "https://google.com"
	case PlatformMuseAI:
		return "https://muse.ai"
	default:
		return ""
	}
}

func (j *cookieSnapshotJar) SetCookies(u *url.URL, updates []*http.Cookie) {
	// Keep snapshot writes in the same order as response-cookie application.
	// Otherwise a slow persistence callback from an older response can finish
	// after a newer callback and restore stale credentials in the account store.
	j.writeMu.Lock()
	defer j.writeMu.Unlock()
	j.jar.SetCookies(u, updates)
	if u == nil || len(updates) == 0 {
		return
	}

	j.mu.Lock()
	next := append([]BrowserCookie(nil), j.cookies...)
	changed := false
	now := time.Now()
	for _, update := range updates {
		if update == nil || strings.TrimSpace(update.Name) == "" {
			continue
		}
		domain, hostOnly := responseCookieDomain(update.Domain, u.Hostname())
		if domain == "" || !cookieDomainAcceptsHost(domain, hostOnly, u.Hostname()) {
			continue
		}
		path := update.Path
		if path == "" || !strings.HasPrefix(path, "/") {
			path = defaultCookiePath(u.Path)
		}
		index := cookieSnapshotIndex(next, update.Name, domain, path)
		deleted := update.MaxAge < 0 || (!update.Expires.IsZero() && !update.Expires.After(now))
		if deleted {
			if index >= 0 {
				next = append(next[:index], next[index+1:]...)
				changed = true
			}
			continue
		}
		cookie := BrowserCookie{
			Name: update.Name, Value: update.Value, Domain: domain, Path: path,
			HTTPOnly: update.HttpOnly, Secure: update.Secure,
			Session: update.Expires.IsZero(), SameSite: browserSameSiteName(update.SameSite),
			Partitioned: update.Partitioned,
		}
		if !update.Expires.IsZero() {
			cookie.Expires = float64(update.Expires.Unix())
		}
		if update.Partitioned {
			if topLevelSite := partitionTopLevelSite(j.platform); topLevelSite != "" {
				cookie.PartitionKey, _ = json.Marshal(map[string]any{"topLevelSite": topLevelSite, "hasCrossSiteAncestor": false})
			}
		}
		if u.Scheme == "https" {
			cookie.SourceScheme, cookie.SourcePort = "Secure", 443
		} else {
			cookie.SourceScheme, cookie.SourcePort = "NonSecure", 80
		}
		if port := u.Port(); port != "" {
			if parsedPort, err := strconv.Atoi(port); err == nil {
				cookie.SourcePort = parsedPort
			}
		}
		if index >= 0 {
			cookie.Priority = next[index].Priority
			cookie.SameParty = next[index].SameParty
			if len(cookie.PartitionKey) == 0 && len(next[index].PartitionKey) > 0 {
				cookie.PartitionKey = append(cookie.PartitionKey[:0], next[index].PartitionKey...)
				cookie.Partitioned = next[index].Partitioned
				cookie.PartitionKeyOpaque = next[index].PartitionKeyOpaque
			}
			if sameBrowserCookie(next[index], cookie) {
				continue
			}
			next[index] = cookie
		} else {
			next = append(next, cookie)
		}
		changed = true
	}
	if !changed {
		j.mu.Unlock()
		return
	}
	j.cookies = next
	snapshot := append([]BrowserCookie(nil), next...)
	header := cookieHeaderFromSnapshot(snapshot)
	updater := j.updater
	j.mu.Unlock()

	if updater != nil {
		if err := updater(header, snapshot); err != nil {
			j.mu.Lock()
			j.lastErr = err
			j.mu.Unlock()
		}
	}
}

func sameBrowserCookie(left, right BrowserCookie) bool {
	return left.Name == right.Name && left.Value == right.Value &&
		strings.EqualFold(left.Domain, right.Domain) && left.Path == right.Path &&
		left.Expires == right.Expires && left.HTTPOnly == right.HTTPOnly &&
		left.Secure == right.Secure && left.Session == right.Session &&
		strings.EqualFold(left.SameSite, right.SameSite) && left.Priority == right.Priority &&
		left.SourceScheme == right.SourceScheme && left.SourcePort == right.SourcePort &&
		left.SameParty == right.SameParty && left.Partitioned == right.Partitioned && string(left.PartitionKey) == string(right.PartitionKey) &&
		left.PartitionKeyOpaque == right.PartitionKeyOpaque
}

func responseCookieDomain(rawDomain, requestHost string) (string, bool) {
	rawDomain = strings.ToLower(strings.TrimSpace(rawDomain))
	if rawDomain == "" {
		return strings.ToLower(strings.TrimSuffix(requestHost, ".")), true
	}
	domain := strings.TrimPrefix(rawDomain, ".")
	if domain == "" {
		return "", false
	}
	return "." + domain, false
}

func cookieDomainAcceptsHost(domain string, hostOnly bool, requestHost string) bool {
	domain = strings.ToLower(strings.TrimPrefix(strings.TrimSpace(domain), "."))
	host := strings.ToLower(strings.TrimSuffix(strings.TrimSpace(requestHost), "."))
	if hostOnly {
		return host == domain
	}
	return host == domain || strings.HasSuffix(host, "."+domain)
}

func cookieSnapshotIndex(cookies []BrowserCookie, name, domain, path string) int {
	for i := range cookies {
		if cookies[i].Name == name && strings.EqualFold(cookies[i].Domain, domain) && cookies[i].Path == path {
			return i
		}
	}
	return -1
}

func defaultCookiePath(requestPath string) string {
	if requestPath == "" || !strings.HasPrefix(requestPath, "/") {
		return "/"
	}
	lastSlash := strings.LastIndex(requestPath, "/")
	if lastSlash <= 0 {
		return "/"
	}
	return requestPath[:lastSlash]
}

func httpCookieSameSite(value string) http.SameSite {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "strict":
		return http.SameSiteStrictMode
	case "lax":
		return http.SameSiteLaxMode
	case "none":
		return http.SameSiteNoneMode
	default:
		return http.SameSiteDefaultMode
	}
}

func browserSameSiteName(value http.SameSite) string {
	switch value {
	case http.SameSiteStrictMode:
		return "Strict"
	case http.SameSiteLaxMode:
		return "Lax"
	case http.SameSiteNoneMode:
		return "None"
	default:
		return ""
	}
}

func cookieHeaderFromSnapshot(cookies []BrowserCookie) string {
	type orderedCookie struct {
		cookie BrowserCookie
		order  int
	}
	applicable := make([]orderedCookie, 0, len(cookies))
	now := float64(time.Now().Unix())
	for i, cookie := range cookies {
		if cookie.Name == "" || (!cookie.Session && cookie.Expires > 0 && cookie.Expires <= now) {
			continue
		}
		applicable = append(applicable, orderedCookie{cookie: cookie, order: i})
	}
	sort.SliceStable(applicable, func(i, j int) bool {
		left, right := applicable[i].cookie.Path, applicable[j].cookie.Path
		if len(left) != len(right) {
			return len(left) > len(right)
		}
		return applicable[i].order < applicable[j].order
	})
	parts := make([]string, 0, len(applicable))
	for _, item := range applicable {
		parts = append(parts, item.cookie.Name+"="+item.cookie.Value)
	}
	return strings.Join(parts, "; ")
}

func cookieHeaderFromHTTP(cookies []*http.Cookie) string {
	parts := make([]string, 0, len(cookies))
	for _, cookie := range cookies {
		if cookie != nil && cookie.Name != "" {
			parts = append(parts, cookie.Name+"="+cookie.Value)
		}
	}
	return strings.Join(parts, "; ")
}

func (j *cookieSnapshotJar) Snapshot() []BrowserCookie {
	if j == nil {
		return nil
	}
	j.mu.RLock()
	defer j.mu.RUnlock()
	return normalizeCookieJar(j.cookies)
}

func (j *cookieSnapshotJar) CookieHeader() string {
	if j == nil {
		return ""
	}
	return cookieHeaderFromSnapshot(j.Snapshot())
}

func (j *cookieSnapshotJar) takeError() error {
	if j == nil {
		return nil
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	err := j.lastErr
	j.lastErr = nil
	return err
}

func (j *cookieSnapshotJar) peekError() error {
	if j == nil {
		return nil
	}
	j.mu.RLock()
	defer j.mu.RUnlock()
	return j.lastErr
}
