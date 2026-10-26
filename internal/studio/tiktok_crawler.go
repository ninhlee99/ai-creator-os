package studio

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strings"
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
	ID    string `json:"id"`
	Desc  string `json:"desc"`
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
	targetURL := "https://www.tiktok.com/explore"
	if searchQuery != "" {
		targetURL = "https://www.tiktok.com/search?q=" + url.QueryEscape(searchQuery)
	}

	videos, err := runTikTokCrawlerBrowser(ctx, bin, targetURL)
	if err != nil || len(videos) == 0 {
		// If explore yielded no videos, fallback to searching trending videos in Vietnam
		if searchQuery == "" {
			fallbackURL := "https://www.tiktok.com/search?q=" + url.QueryEscape("hot trend việt nam")
			if fallbackVideos, fErr := runTikTokCrawlerBrowser(ctx, bin, fallbackURL); fErr == nil && len(fallbackVideos) > 0 {
				return fallbackVideos, nil
			}
		}
		if err != nil {
			return nil, err
		}
		return nil, fmt.Errorf("TikTok Crawler không thu thập được video cho chủ đề %q", topic)
	}

	return videos, nil
}

func runTikTokCrawlerBrowser(ctx context.Context, bin, targetURL string) ([]TikTokTrendVideo, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
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
		fmt.Sprintf("--user-data-dir=%s", profileDir),
		"--disable-gpu",
		"--disable-dev-shm-usage",
		"--no-first-run",
		"--no-default-browser-check",
		"--disable-extensions",
		"--disable-sync",
		"--user-agent=Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
		"about:blank",
	}

	cmd := exec.CommandContext(ctx, bin, args...)
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("không khởi động được trình duyệt crawler: %w", err)
	}
	defer func() {
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
		}
	}()

	// Wait for WebSocket debugger URL
	pageWS, err := waitCrawlerDebuggerURL(ctx, port)
	if err != nil {
		return nil, err
	}

	conn, _, err := (&websocket.Dialer{HandshakeTimeout: 8 * time.Second}).DialContext(ctx, pageWS, nil)
	if err != nil {
		return nil, fmt.Errorf("không kết nối được WebSocket crawler: %w", err)
	}
	defer conn.Close()

	sendCDP := func(id int, method string, params any) error {
		return conn.WriteJSON(map[string]any{"id": id, "method": method, "params": params})
	}

	if err := sendCDP(1, "Network.enable", map[string]any{}); err != nil {
		return nil, err
	}
	if err := sendCDP(2, "Page.enable", map[string]any{}); err != nil {
		return nil, err
	}
	if err := sendCDP(3, "Page.navigate", map[string]any{"url": targetURL}); err != nil {
		return nil, err
	}

	type reqDoneSignal struct {
		reqID string
		isAPI bool
	}
	signalChan := make(chan reqDoneSignal, 10)
	errChan := make(chan error, 1)

	var targetReqID string
	var reqIDToWatch string

	go func() {
		for {
			if ctx.Err() != nil {
				return
			}
			var msg struct {
				ID     int             `json:"id"`
				Method string          `json:"method"`
				Params json.RawMessage `json:"params"`
			}
			if err := conn.ReadJSON(&msg); err != nil {
				errChan <- err
				return
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
						reqIDToWatch = p.RequestID
					}
				}
			} else if msg.Method == "Network.loadingFinished" && reqIDToWatch != "" {
				var p struct {
					RequestID string `json:"requestId"`
				}
				if json.Unmarshal(msg.Params, &p) == nil && p.RequestID == reqIDToWatch {
					signalChan <- reqDoneSignal{reqID: reqIDToWatch, isAPI: true}
					return
				}
			}
		}
	}()

	select {
	case sig := <-signalChan:
		targetReqID = sig.reqID
	case <-time.After(12 * time.Second):
		// Fallback to whichever request ID was partially received if any
		targetReqID = reqIDToWatch
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	if targetReqID != "" {
		if err := sendCDP(100, "Network.getResponseBody", map[string]any{"requestId": targetReqID}); err == nil {
			var resp struct {
				ID     int `json:"id"`
				Result struct {
					Body string `json:"body"`
				} `json:"result"`
			}
			for i := 0; i < 10; i++ {
				_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
				if err := conn.ReadJSON(&resp); err == nil && resp.ID == 100 {
					if len(resp.Result.Body) > 0 {
						if videos := parseTikTokCrawlerJSON([]byte(resp.Result.Body)); len(videos) > 0 {
							return videos, nil
						}
					}
					break
				}
			}
		}
	}

	// DOM Evaluation Fallback: extract video elements directly from DOM
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
		case <-time.After(250 * time.Millisecond):
		}
	}
}

func parseTikTokCrawlerJSON(body []byte) []TikTokTrendVideo {
	var payload struct {
		StatusCode int `json:"status_code"`
		Data []struct {
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
		if music == "" {
			music = "Sound gốc video TikTok"
		}

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
		if len(videos) >= 15 {
			break
		}
	}

	return videos
}

func evaluateTikTokCrawlerDOM(ctx context.Context, conn *websocket.Conn, sendCDP func(int, string, any) error) ([]TikTokTrendVideo, error) {
	evalScript := `(() => {
		const results = [];
		const links = Array.from(document.querySelectorAll('a[href*="/video/"]'));
		const seen = new Set();
		for (const a of links) {
			const href = a.href;
			const match = href.match(/@([^/]+)\/video\/(\d+)/);
			if (!match) continue;
			const uniqueId = match[1];
			const id = match[2];
			if (seen.has(id)) continue;
			seen.add(id);

			let container = a.closest('[data-e2e="search_video-item"]') || a.closest('div');
			const img = a.querySelector('img') || (container ? container.querySelector('img') : null);
			const cover = img ? img.src : '';
			const caption = container ? (container.innerText || '').slice(0, 140) : '';

			results.push({ id, uniqueId, url: href, cover, caption });
			if (results.length >= 10) break;
		}
		return results;
	})()`

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

	_ = conn.SetReadDeadline(time.Now().Add(4 * time.Second))
	for i := 0; i < 5; i++ {
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
			}
			return videos, nil
		}
	}

	return nil, fmt.Errorf("không trích xuất được danh sách video TikTok từ trang web")
}
