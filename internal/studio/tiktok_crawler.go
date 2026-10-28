package studio

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
	"github.com/ninhlee99/ai-creator-os/internal/webai"
)

const TikTokCrawlerSource = "TikTok Web Crawler"

// TikTokCrawlerConfigured reports whether a local browser binary (Edge, Chrome, Brave, Chromium)
// is available to run the automated crawler.
func TikTokCrawlerConfigured() bool {
	_, err := webai.FindChromiumBinary()
	return err == nil
}

type tikTokRawItem struct {
	ID     string `json:"id"`
	Desc   string `json:"desc"`
	Author struct {
		Nickname string `json:"nickname"`
		UniqueID string `json:"uniqueId"`
	} `json:"author"`
	Stats struct {
		PlayCount    int64 `json:"playCount"`
		DiggCount    int64 `json:"diggCount"`
		CommentCount int64 `json:"commentCount"`
		ShareCount   int64 `json:"shareCount"`
	} `json:"stats"`
	Video struct {
		Cover       string `json:"cover"`
		OriginCover string `json:"originCover"`
	} `json:"video"`
	Music struct {
		Title string `json:"title"`
	} `json:"music"`
}

func fetchTikTokCrawlerVideos(ctx context.Context, topic string) ([]TikTokTrendVideo, error) {
	bin, err := webai.FindChromiumBinary()
	if err != nil {
		return nil, fmt.Errorf("không tìm thấy trình duyệt (Edge/Chrome) để chạy TikTok Crawler: %w", err)
	}

	searchQuery := strings.TrimSpace(topic)
	isRegionalFeed := searchQuery == "" || IsTikTokRadarProtectedDefaultTopic(searchQuery)
	searchQueries := []string{searchQuery}
	if isRegionalFeed {
		searchQueries = []string{
			"xu hướng việt nam",
			"trend nhảy tiktok việt nam",
			"điệu nhảy xu hướng việt nam",
			"dance trend tiktok vietnam",
		}
	} else {
		lower := strings.ToLower(searchQuery)
		if !strings.Contains(lower, "việt nam") && !strings.Contains(lower, "viet nam") && !strings.Contains(lower, "vn") {
			searchQuery += " việt nam"
		}
		searchQueries = []string{searchQuery}
	}

	var videos []TikTokTrendVideo
	var fetchErrors []error
	for _, query := range searchQueries {
		targetURL := "https://www.tiktok.com/search?q=" + url.QueryEscape(query)
		found, fetchErr := runTikTokCrawlerBrowser(ctx, bin, targetURL)
		if fetchErr != nil {
			fetchErrors = append(fetchErrors, fmt.Errorf("TikTok search %q: %w", query, fetchErr))
			continue
		}
		videos = mergeTikTokRadarCandidates(videos, found)
		if len(videos) >= tikTokRadarCandidateLimit {
			return videos, nil
		}
	}
	if len(videos) > 0 {
		return videos, nil
	}
	if len(fetchErrors) > 0 {
		return nil, errors.Join(fetchErrors...)
	}
	return nil, fmt.Errorf("TikTok Crawler không thu thập được video cho chủ đề %q", topic)
}

func runTikTokCrawlerBrowser(ctx context.Context, bin, targetURL string) ([]TikTokTrendVideo, error) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	portListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("không mở được cổng kết nối crawler: %w", err)
	}
	port := portListener.Addr().(*net.TCPAddr).Port
	_ = portListener.Close()

	profileDir, err := os.MkdirTemp("", "aicos-tt-crawler-*")
	if err != nil {
		return nil, fmt.Errorf("không tạo được thư mục tạm cho crawler: %w", err)
	}
	defer os.RemoveAll(profileDir)

	args := []string{
		"--headless=new",
		fmt.Sprintf("--remote-debugging-port=%d", port),
		"--remote-debugging-address=127.0.0.1",
		fmt.Sprintf("--user-data-dir=%s", profileDir),
		"--disable-gpu",
		"--disable-dev-shm-usage",
		"--disable-software-rasterizer",
		"--no-first-run",
		"--no-default-browser-check",
		"--disable-extensions",
		"--disable-sync",
		"--disable-background-networking",
		"--disable-background-timer-throttling",
		"--disable-backgrounding-occluded-windows",
		"--disable-renderer-backgrounding",
		"--disk-cache-size=1048576",
		"--media-cache-size=1048576",
		"--js-flags=--max-old-space-size=384",
		"--blink-settings=imagesEnabled=false",
		"--mute-audio",
		"--lang=vi-VN,vi",
		"--user-agent=Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36",
		"about:blank",
	}

	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Stdout, cmd.Stderr = io.Discard, io.Discard
	webai.ConfigureHeadlessBrowserCommand(cmd)
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("không khởi động được trình duyệt crawler: %w", err)
	}
	defer webai.TerminateBrowser(cmd)

	// Wait for WebSocket debugger URL
	pageWS, err := waitCrawlerDebuggerURL(ctx, port)
	if err != nil {
		return nil, err
	}

	conn, _, err := (&websocket.Dialer{HandshakeTimeout: 5 * time.Second}).DialContext(ctx, pageWS, nil)
	if err != nil {
		return nil, fmt.Errorf("không kết nối được WebSocket crawler: %w", err)
	}
	defer conn.Close()

	var writeMu sync.Mutex
	sendCDP := func(id int, method string, params any) error {
		writeMu.Lock()
		defer writeMu.Unlock()
		_ = conn.SetWriteDeadline(time.Now().Add(4 * time.Second))
		return conn.WriteJSON(map[string]any{"id": id, "method": method, "params": params})
	}

	if err := sendCDP(1, "Network.enable", map[string]any{}); err != nil {
		return nil, err
	}
	_ = sendCDP(2, "Network.setBlockedURLs", map[string]any{
		"urls": []string{"*.mp4", "*.webm", "*.woff*", "*.ttf", "*.otf", "*.gif"},
	})
	_ = sendCDP(3, "Network.setExtraHTTPHeaders", map[string]any{
		"headers": map[string]string{
			"Accept-Language": "vi-VN,vi;q=0.9,en;q=0.8",
		},
	})
	if err := sendCDP(4, "Page.enable", map[string]any{}); err != nil {
		return nil, err
	}
	_ = sendCDP(5, "Page.addScriptToEvaluateOnNewDocument", map[string]any{
		"source": `try{Object.defineProperty(navigator,'webdriver',{get:()=>undefined});}catch(_){}`,
	})
	if err := sendCDP(6, "Page.navigate", map[string]any{"url": targetURL}); err != nil {
		return nil, err
	}

	resultChan := make(chan []TikTokTrendVideo, 1)
	readerDone := make(chan struct{})
	var readerStop atomic.Bool
	matchedReqIDs := make(map[string]bool)
	var reqMu sync.Mutex
	var reqCounter int

	go func() {
		defer close(readerDone)
		for {
			if ctx.Err() != nil || readerStop.Load() {
				return
			}
			var msg struct {
				ID     int             `json:"id"`
				Method string          `json:"method"`
				Params json.RawMessage `json:"params"`
				Result struct {
					Body string `json:"body"`
				} `json:"result"`
			}
			if err := conn.ReadJSON(&msg); err != nil {
				return
			}
			if readerStop.Load() {
				return
			}
			if msg.ID >= 1000 && len(msg.Result.Body) > 0 {
				if videos := parseTikTokCrawlerJSON([]byte(msg.Result.Body)); len(videos) > 0 {
					select {
					case resultChan <- videos:
					default:
					}
					return
				}
			}
			if msg.Method == "Network.responseReceived" {
				var p struct {
					RequestID string `json:"requestId"`
					Response  struct {
						URL string `json:"url"`
					} `json:"response"`
				}
				if json.Unmarshal(msg.Params, &p) == nil {
					u := p.Response.URL
					if strings.Contains(u, "/api/search/general/full/") ||
						strings.Contains(u, "/api/search/item/full/") ||
						strings.Contains(u, "/api/explore/item_list/") ||
						strings.Contains(u, "/api/recommend/item_list/") {
						reqMu.Lock()
						matchedReqIDs[p.RequestID] = true
						reqMu.Unlock()
					}
				}
			} else if msg.Method == "Network.loadingFinished" {
				var p struct {
					RequestID string `json:"requestId"`
				}
				if json.Unmarshal(msg.Params, &p) == nil {
					reqMu.Lock()
					matched := matchedReqIDs[p.RequestID]
					reqMu.Unlock()
					if matched && !readerStop.Load() {
						reqCounter++
						_ = sendCDP(1000+reqCounter, "Network.getResponseBody", map[string]any{"requestId": p.RequestID})
					}
				}
			}
		}
	}()

	select {
	case videos := <-resultChan:
		readerStop.Store(true)
		<-readerDone
		if len(videos) >= tikTokRadarCandidateLimit {
			return videos[:tikTokRadarCandidateLimit], nil
		}
		// Scroll search results for additional candidates so AI filtering can
		// replace rejected videos instead of stopping at the first eight.
		domVideos, domErr := evaluateTikTokCrawlerDOM(ctx, conn, sendCDP)
		if domErr != nil {
			return videos, nil
		}
		return mergeTikTokRadarCandidates(videos, domVideos), nil
	case <-time.After(7 * time.Second):
	case <-ctx.Done():
		readerStop.Store(true)
		return nil, ctx.Err()
	}

	// DOM Evaluation Fallback: stop reader goroutine cleanly before evaluating DOM
	readerStop.Store(true)
	_ = conn.SetReadDeadline(time.Now().Add(-time.Second))
	<-readerDone
	_ = conn.SetReadDeadline(time.Time{})
	return evaluateTikTokCrawlerDOM(ctx, conn, sendCDP)
}

func waitCrawlerDebuggerURL(ctx context.Context, port int) (string, error) {
	client := &http.Client{Timeout: 2 * time.Second}
	for {
		if err := ctx.Err(); err != nil {
			return "", errors.New("trình duyệt crawler không kịp khởi động")
		}
		resp, err := client.Get(fmt.Sprintf("http://127.0.0.1:%d/json", port))
		if err == nil {
			var targets []struct {
				Type string `json:"type"`
				WS   string `json:"webSocketDebuggerUrl"`
			}
			decodeErr := json.NewDecoder(resp.Body).Decode(&targets)
			_ = resp.Body.Close()
			if decodeErr == nil {
				for _, t := range targets {
					if t.Type == "page" && t.WS != "" {
						return t.WS, nil
					}
				}
			}
		}
		select {
		case <-ctx.Done():
			return "", errors.New("trình duyệt crawler không kịp khởi động")
		case <-time.After(200 * time.Millisecond):
		}
	}
}

func parseTikTokCrawlerJSON(body []byte) []TikTokTrendVideo {
	var payload struct {
		StatusCode int `json:"status_code"`
		Data       []struct {
			Item *tikTokRawItem `json:"item"`
		} `json:"data"`
		ItemList []tikTokRawItem `json:"itemList"`
	}

	if err := json.Unmarshal(body, &payload); err != nil {
		return nil
	}

	var rawItems []tikTokRawItem
	for _, d := range payload.Data {
		if d.Item != nil && d.Item.ID != "" {
			rawItems = append(rawItems, *d.Item)
		}
	}
	for _, item := range payload.ItemList {
		if item.ID != "" {
			rawItems = append(rawItems, item)
		}
	}

	if len(rawItems) == 0 {
		return nil
	}

	var videos []TikTokTrendVideo
	seen := make(map[string]struct{}, len(rawItems))

	for _, item := range rawItems {
		id := strings.TrimSpace(item.ID)
		if id == "" {
			continue
		}
		if _, duplicate := seen[id]; duplicate {
			continue
		}
		seen[id] = struct{}{}

		caption := strings.TrimSpace(item.Desc)
		title := caption
		if title == "" {
			if item.Author.UniqueID != "" {
				title = fmt.Sprintf("Video TikTok của @%s", item.Author.UniqueID)
			} else {
				title = fmt.Sprintf("Video TikTok %s", id)
			}
		}

		music := strings.TrimSpace(item.Music.Title)

		videoURL := fmt.Sprintf("https://www.tiktok.com/@%s/video/%s", item.Author.UniqueID, id)
		if item.Author.UniqueID == "" {
			videoURL = fmt.Sprintf("https://www.tiktok.com/video/%s", id)
		}

		cover := strings.TrimSpace(item.Video.Cover)
		if cover == "" {
			cover = strings.TrimSpace(item.Video.OriginCover)
		}

		v := TikTokTrendVideo{
			VideoID:         id,
			Title:           title,
			Caption:         caption,
			Region:          "VN",
			Author:          strings.TrimSpace(item.Author.Nickname),
			UniqueID:        strings.TrimSpace(item.Author.UniqueID),
			Music:           music,
			VideoURL:        videoURL,
			CoverURL:        cover,
			Provider:        TikTokCrawlerSource,
			CollectedAt:     time.Now().UTC().Format(time.RFC3339),
			PlayCount:       item.Stats.PlayCount,
			PlayCountKnown:  item.Stats.PlayCount > 0,
			Views:           FormatCount(item.Stats.PlayCount),
			DiggCount:       item.Stats.DiggCount,
			DiggCountKnown:  item.Stats.DiggCount > 0,
			Likes:           FormatCount(item.Stats.DiggCount),
			CommentCount:    item.Stats.CommentCount,
			CommentKnown:    item.Stats.CommentCount > 0,
			Comments:        FormatCount(item.Stats.CommentCount),
			ShareCount:      item.Stats.ShareCount,
			ShareCountKnown: item.Stats.ShareCount > 0,
			Shares:          FormatCount(item.Stats.ShareCount),
		}
		videos = append(videos, v)
		if len(videos) >= tikTokRadarCandidateLimit {
			break
		}
	}

	return videos
}

func evaluateTikTokCrawlerDOM(ctx context.Context, conn *websocket.Conn, sendCDP func(int, string, any) error) ([]TikTokTrendVideo, error) {
	evalScript := fmt.Sprintf(`(async () => {
		const results = [];
		const seen = new Set();
		const collect = () => {
			const links = Array.from(document.querySelectorAll('a[href*="/video/"]'));
			for (const a of links) {
				const href = a.href;
				const match = href.match(/@([^/]+)\/video\/(\d+)/);
				if (!match) continue;
				const uniqueId = match[1];
				const id = match[2];
				if (seen.has(id)) continue;
				seen.add(id);
				const container = a.closest('[data-e2e="search_video-item"]') || a.closest('div');
				const media = a.querySelector('img,video') || (container ? container.querySelector('img,video') : null);
				const cover = media ? (media.getAttribute('src') || media.getAttribute('data-src') || media.getAttribute('poster') || '') : '';
				const caption = container ? (container.innerText || '').slice(0, 140) : '';
				results.push({ id, uniqueId, url: href, cover, caption });
				if (results.length >= %d) break;
			}
		};
		let stagnant = 0;
		for (let page = 0; page < 5 && results.length < %d && stagnant < 2; page++) {
			collect();
			const before = results.length;
			window.scrollTo(0, document.body.scrollHeight);
			await new Promise(resolve => setTimeout(resolve, 900));
			collect();
			stagnant = results.length === before ? stagnant + 1 : 0;
		}
		collect();
		return results;
	})()`, tikTokRadarCandidateLimit, tikTokRadarCandidateLimit)

	if err := sendCDP(200, "Runtime.evaluate", map[string]any{
		"expression":    evalScript,
		"returnByValue": true,
		"awaitPromise":  true,
	}); err != nil {
		return nil, err
	}

	var evalResp struct {
		ID     int `json:"id"`
		Result struct {
			Result struct {
				Value []struct {
					ID       string `json:"id"`
					UniqueID string `json:"uniqueId"`
					URL      string `json:"url"`
					Cover    string `json:"cover"`
					Caption  string `json:"caption"`
				} `json:"value"`
			} `json:"result"`
		} `json:"result"`
	}

	_ = conn.SetReadDeadline(time.Now().Add(7 * time.Second))
	for i := 0; i < 8; i++ {
		if err := conn.ReadJSON(&evalResp); err == nil && evalResp.ID == 200 {
			var videos []TikTokTrendVideo
			for _, item := range evalResp.Result.Result.Value {
				if item.ID == "" {
					continue
				}
				title := strings.TrimSpace(item.Caption)
				if title == "" {
					title = fmt.Sprintf("Video TikTok của @%s", item.UniqueID)
				}
				videos = append(videos, TikTokTrendVideo{
					VideoID:     item.ID,
					Title:       title,
					Caption:     title,
					Region:      "VN",
					UniqueID:    item.UniqueID,
					Author:      item.UniqueID,
					VideoURL:    item.URL,
					CoverURL:    item.Cover,
					Provider:    TikTokCrawlerSource,
					CollectedAt: time.Now().UTC().Format(time.RFC3339),
				})
				if len(videos) >= tikTokRadarCandidateLimit {
					break
				}
			}
			return videos, nil
		}
	}

	return nil, fmt.Errorf("không trích xuất được danh sách video TikTok từ trang web")
}

func mergeTikTokRadarCandidates(primary, additional []TikTokTrendVideo) []TikTokTrendVideo {
	merged := make([]TikTokTrendVideo, 0, tikTokRadarCandidateLimit)
	seen := make(map[string]struct{}, tikTokRadarCandidateLimit)
	for _, group := range [][]TikTokTrendVideo{primary, additional} {
		for _, video := range group {
			if strings.TrimSpace(video.VideoID) == "" {
				continue
			}
			if _, duplicate := seen[video.VideoID]; duplicate {
				continue
			}
			seen[video.VideoID] = struct{}{}
			merged = append(merged, video)
			if len(merged) >= tikTokRadarCandidateLimit {
				return merged
			}
		}
	}
	return merged
}
