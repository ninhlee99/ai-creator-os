package products

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/ninhlee99/ai-creator-os/internal/webai"
)

var (
	tiktokCookieMu       sync.RWMutex
	tiktokCookieProvider func() (string, []webai.BrowserCookie)
)

// SetTikTokCookieProvider sets a callback to supply TikTok session cookies for product crawling.
func SetTikTokCookieProvider(provider func() (string, []webai.BrowserCookie)) {
	tiktokCookieMu.Lock()
	defer tiktokCookieMu.Unlock()
	tiktokCookieProvider = provider
}

func getTikTokCookies() (string, []webai.BrowserCookie) {
	tiktokCookieMu.RLock()
	defer tiktokCookieMu.RUnlock()
	if tiktokCookieProvider != nil {
		return tiktokCookieProvider()
	}
	return "", nil
}

// ExtractTikTokProductWithBrowser uses a headless browser to open the TikTok Shop page,
// handle redirects, scroll down to trigger the "Get the full app experience" popup,
// click "Not now" / close icon, scroll further to lazy load all gallery photos, and extract details.
func ExtractTikTokProductWithBrowser(ctx context.Context, targetURL string) (*ExtractedProduct, error) {
	bin, err := webai.FindChromiumBinary()
	if err != nil {
		return nil, fmt.Errorf("không tìm thấy trình duyệt để chạy crawler: %w", err)
	}

	timeout := 35 * time.Second
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	releaseBrowserSlot, err := webai.AcquireHeadlessBrowserSlot(ctx)
	if err != nil {
		return nil, fmt.Errorf("không còn lượt chạy trình duyệt: %w", err)
	}
	defer releaseBrowserSlot()

	profileDir, err := os.MkdirTemp("", "aicos-tt-prod-*")
	if err != nil {
		return nil, fmt.Errorf("không tạo được thư mục tạm: %w", err)
	}
	defer os.RemoveAll(profileDir)

	args := webai.OptimizedHeadlessBrowserArgs(0, profileDir, "412,915")
	// Dùng User-Agent mobile để trang hiển thị dạng mobile web (nơi popup Get the full app experience xuất hiện)
	args = append(args[:len(args)-1],
		"--user-agent=Mozilla/5.0 (iPhone; CPU iPhone OS 17_5 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.5 Mobile/15E148 Safari/604.1",
		"--lang=vi-VN,vi",
		"about:blank",
	)

	cmd := exec.CommandContext(ctx, bin, args...)
	closeBrowserOutput, err := webai.ConfigureHeadlessBrowserOutput(cmd)
	if err != nil {
		return nil, fmt.Errorf("không cấu hình được output trình duyệt: %w", err)
	}
	defer closeBrowserOutput()
	webai.ConfigureHeadlessBrowserCommand(cmd)

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("không khởi động được trình duyệt: %w", err)
	}
	// Đảm bảo trình duyệt luôn được tắt để reset RAM ngay sau khi lấy xong dữ liệu
	defer webai.TerminateBrowser(cmd)

	pageWS, err := waitProductDebuggerURL(ctx, profileDir)
	if err != nil {
		return nil, err
	}

	conn, _, err := (&websocket.Dialer{HandshakeTimeout: 5 * time.Second}).DialContext(ctx, pageWS, nil)
	if err != nil {
		return nil, fmt.Errorf("không kết nối được WebSocket: %w", err)
	}
	defer conn.Close()

	var writeMu sync.Mutex
	sendCDP := func(id int, method string, params any) error {
		writeMu.Lock()
		defer writeMu.Unlock()
		_ = conn.SetWriteDeadline(time.Now().Add(4 * time.Second))
		return conn.WriteJSON(map[string]any{"id": id, "method": method, "params": params})
	}

	_ = sendCDP(1, "Network.enable", map[string]any{})
	_ = sendCDP(2, "Page.enable", map[string]any{})
	_ = sendCDP(3, "Page.addScriptToEvaluateOnNewDocument", map[string]any{
		"source": `try{Object.defineProperty(navigator,'webdriver',{get:()=>undefined});}catch(_){}`,
	})

	// Cài đặt cookie nếu có
	cookieHeader, cookieJar := getTikTokCookies()
	extraHeaders := map[string]string{
		"Accept-Language": "vi-VN,vi;q=0.9,en;q=0.8",
	}
	if cookieHeader != "" {
		extraHeaders["Cookie"] = cookieHeader
	}
	_ = sendCDP(4, "Network.setExtraHTTPHeaders", map[string]any{
		"headers": extraHeaders,
	})
	if len(cookieJar) > 0 {
		_ = sendCDP(5, "Network.setCookies", map[string]any{
			"cookies": webai.BrowserCookieSetParams("tiktok", cookieJar),
		})
	} else if cookieHeader != "" {
		parsed := webai.ParseCookieHeaderToBrowserCookies(cookieHeader, ".tiktok.com")
		if len(parsed) > 0 {
			_ = sendCDP(5, "Network.setCookies", map[string]any{
				"cookies": webai.BrowserCookieSetParams("tiktok", parsed),
			})
		}
	}

	// Điều hướng đến URL
	_ = sendCDP(6, "Page.navigate", map[string]any{"url": targetURL})

	// Đợi trang tải sơ bộ
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-time.After(4 * time.Second):
	}

	// Script thực thi: cuộn trang, phát hiện & click "Not now", cuộn tiếp để lazy load toàn bộ ảnh
	evalScript := `(async () => {
		const logs = [];

		// Cuộn lần 1 để kích hoạt hiển thị popup "Get the full app experience"
		window.scrollBy({ top: 380, behavior: 'instant' });
		await new Promise(r => setTimeout(r, 900));

		// Phát hiện popup và click "Not now" hoặc dấu "✕"
		let dismissed = false;
		// 1. Quét tìm nút hoặc text "Not now" / "Để sau" / "Không phải bây giờ"
		const allTextEls = Array.from(document.querySelectorAll('button, div, span, a, p'));
		for (const el of allTextEls) {
			const txt = (el.textContent || '').trim().toLowerCase();
			if (txt === 'not now' || txt === 'để sau' || txt === 'không phải bây giờ' || txt === 'later') {
				el.click();
				dismissed = true;
				logs.push('Clicked: ' + txt);
				break;
			}
		}

		// 2. Nếu chưa đóng được, tìm nút icon ✕ / × / Close
		if (!dismissed) {
			const closeBtns = document.querySelectorAll('button, [role="button"], [aria-label*="close" i], [aria-label*="đóng" i]');
			for (const btn of closeBtns) {
				const txt = (btn.textContent || '').trim();
				if (txt === '✕' || txt === '×' || txt.toLowerCase() === 'x') {
					btn.click();
					dismissed = true;
					logs.push('Clicked close icon');
					break;
				}
			}
		}

		// 3. Fallback: Nếu có modal backdrop / dialog che màn hình, tìm svg close
		if (!dismissed) {
			const modalSvgs = document.querySelectorAll('div[class*="modal"] svg, div[class*="dialog"] svg, div[class*="popup"] svg');
			for (const svg of modalSvgs) {
				const p = svg.closest('button') || svg.parentElement;
				if (p) {
					p.click();
					dismissed = true;
					logs.push('Clicked modal svg close');
					break;
				}
			}
		}

		await new Promise(r => setTimeout(r, 600));

		// Cuộn lần 2 sâu hơn để kích hoạt tải ảnh mô tả và chi tiết
		window.scrollBy({ top: 800, behavior: 'instant' });
		await new Promise(r => setTimeout(r, 900));

		// Cuộn lần 3 xuống khu vực đánh giá & hình ảnh feedback của khách hàng
		window.scrollBy({ top: 1200, behavior: 'instant' });
		await new Promise(r => setTimeout(r, 1000));

		// Trích xuất toàn bộ hình ảnh sản phẩm
		const seen = new Set();
		const images = [];
		document.querySelectorAll('img').forEach(img => {
			let src = img.currentSrc || img.src || img.getAttribute('data-src') || '';
			src = src.trim();
			if (!src || src.startsWith('data:')) return;
			if (src.includes('ibyteimg.com') || src.includes('tiktokcdn.com') || src.includes('susercontent.com') || src.includes('alicdn.com')) {
				if (!src.includes('avatar') && !src.includes('icon') && !src.includes('logo') && !src.includes('1x1') && !src.includes('badge')) {
					if (!seen.has(src)) {
						seen.add(src);
						images.push(src);
					}
				}
			}
		});

		// Trích xuất tiêu đề sản phẩm
		let title = '';
		const h1 = document.querySelector('h1, [class*="product-title" i], [class*="product_title" i], [class*="title" i]');
		if (h1) title = (h1.innerText || '').trim();
		if (!title || title.length < 5) title = document.title || '';

		// Trích xuất giá bán
		let priceText = '';
		let originalPriceText = '';
		const priceEls = Array.from(document.querySelectorAll('span, div, p')).filter(el => {
			const t = el.textContent || '';
			return (t.includes('₫') || t.includes('đ') || t.includes('VNĐ')) && t.length < 30;
		});
		if (priceEls.length > 0) {
			priceText = (priceEls[0].innerText || priceEls[0].textContent || '').trim();
			if (priceEls.length > 1) {
				originalPriceText = (priceEls[1].innerText || priceEls[1].textContent || '').trim();
			}
		}

		// Trích xuất tên shop
		let shop = '';
		const shopEl = document.querySelector('[class*="shop-name" i], [class*="shop_name" i], [class*="seller" i], [class*="store" i]');
		if (shopEl) shop = (shopEl.innerText || '').trim();

		return {
			url: window.location.href,
			title: title,
			shop: shop,
			priceText: priceText,
			originalPriceText: originalPriceText,
			images: images,
			dismissed: dismissed,
			logs: logs
		};
	})()`

	_ = sendCDP(100, "Runtime.evaluate", map[string]any{
		"expression":    evalScript,
		"returnByValue": true,
		"awaitPromise":  true,
	})

	var evalRes struct {
		URL               string   `json:"url"`
		Title             string   `json:"title"`
		Shop              string   `json:"shop"`
		PriceText         string   `json:"priceText"`
		OriginalPriceText string   `json:"originalPriceText"`
		Images            []string `json:"images"`
		Dismissed         bool     `json:"dismissed"`
		Logs              []string `json:"logs"`
	}

	readerTimeout := time.After(15 * time.Second)
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-readerTimeout:
			return nil, errors.New("quá thời gian trích xuất dữ liệu từ trình duyệt")
		default:
			var msg struct {
				ID     int `json:"id"`
				Result struct {
					Result struct {
						Value json.RawMessage `json:"value"`
					} `json:"result"`
				} `json:"result"`
			}
			_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
			if err := conn.ReadJSON(&msg); err != nil {
				continue
			}
			if msg.ID == 100 && len(msg.Result.Result.Value) > 0 {
				_ = json.Unmarshal(msg.Result.Result.Value, &evalRes)
				goto extractionDone
			}
		}
	}

extractionDone:
	finalURL := evalRes.URL
	if finalURL == "" {
		finalURL = targetURL
	}

	// Lọc và làm sạch tiêu đề
	title := cleanProductTitle(evalRes.Title)
	if isInvalidProductTitle(title) {
		ogTitle, _ := extractOGInfo(targetURL, finalURL)
		if ogTitle != "" {
			title = ogTitle
		} else {
			title = "Sản phẩm TikTok Shop"
		}
	}

	// Lọc và làm sạch danh sách ảnh
	var cleanedImages []string
	seenImgs := make(map[string]bool)
	for _, img := range evalRes.Images {
		img = strings.TrimSpace(htmlUnescape(img))
		img = strings.ReplaceAll(img, `\/`, `/`)
		img = strings.Trim(img, `"' `)
		if strings.HasPrefix(img, "http://") || strings.HasPrefix(img, "https://") {
			if !seenImgs[img] {
				seenImgs[img] = true
				cleanedImages = append(cleanedImages, img)
			}
		}
	}

	// Nếu lấy được ảnh từ og_info mà trong mảng chưa có, chèn vào đầu danh sách làm ảnh chính
	_, ogImg := extractOGInfo(targetURL, finalURL)
	if ogImg != "" {
		ogImg = strings.TrimSpace(htmlUnescape(ogImg))
		ogImg = strings.ReplaceAll(ogImg, `\/`, `/`)
		if !seenImgs[ogImg] {
			cleanedImages = append([]string{ogImg}, cleanedImages...)
		}
	}

	// Trích xuất giá
	price := parsePriceFromText(evalRes.PriceText)
	origPrice := parsePriceFromText(evalRes.OriginalPriceText)
	if origPrice <= price {
		origPrice = 0
	}

	theme, category := inferThemeAndCategory(title, "")
	if price <= 0 {
		switch category {
		case "Thời trang":
			price = 249000
		case "Làm đẹp":
			price = 189000
		default:
			price = 199000
		}
	}

	pid := extractPlatformPID(finalURL, targetURL)
	shop := evalRes.Shop
	if shop == "" {
		shop = "TikTok Shop"
	}

	return &ExtractedProduct{
		PlatformPID:    pid,
		Title:          title,
		ImageURLs:      cleanedImages,
		Price:          price,
		OriginalPrice:  origPrice,
		Currency:       "VND",
		CommissionRate: 0.10,
		ShopName:       shop,
		Category:       category,
		Theme:          theme,
		ProductURL:     targetURL,
		ProductPageURL: finalURL,
	}, nil
}

func waitProductDebuggerURL(ctx context.Context, profileDir string) (string, error) {
	client := &http.Client{Timeout: 2 * time.Second}
	for {
		if err := ctx.Err(); err != nil {
			return "", errors.New("trình duyệt không kịp khởi động")
		}
		if activePort, err := os.ReadFile(filepath.Join(profileDir, "DevToolsActivePort")); err == nil {
			var port int
			if _, err := fmt.Sscanf(string(activePort), "%d", &port); err == nil && port > 0 && port <= 65535 {
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
			}
		}
		select {
		case <-ctx.Done():
			return "", errors.New("trình duyệt không kịp khởi động")
		case <-time.After(120 * time.Millisecond):
		}
	}
}

var (
	priceWithCurrencyRegex = regexp.MustCompile(`(?:₫|đ|VNĐ|VND)\s*([\d\.,]+)|([\d\.,]+)\s*(?:₫|đ|VNĐ|VND)`)
	priceNumberRegex       = regexp.MustCompile(`[\d\.,]+`)
	percentRegex           = regexp.MustCompile(`-?\d+%\s*`)
)

func parsePriceFromText(raw string) float64 {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0
	}
	// 1. Tìm số gắn liền với ký hiệu tiền tệ ₫, đ, VNĐ
	if m := priceWithCurrencyRegex.FindStringSubmatch(raw); len(m) > 0 {
		numStr := m[1]
		if numStr == "" {
			numStr = m[2]
		}
		if p := cleanAndParseFloat(numStr); p > 0 {
			return p
		}
	}
	// 2. Loại bỏ các chuỗi phần trăm giảm giá (ví dụ: -39%)
	noPercent := percentRegex.ReplaceAllString(raw, "")
	// 3. Quét các số còn lại
	for _, numStr := range priceNumberRegex.FindAllString(noPercent, -1) {
		if p := cleanAndParseFloat(numStr); p >= 1000 {
			return p
		}
	}
	return 0
}

func cleanAndParseFloat(s string) float64 {
	clean := strings.ReplaceAll(s, ".", "")
	clean = strings.ReplaceAll(clean, ",", "")
	val, err := strconv.ParseFloat(clean, 64)
	if err == nil && val > 0 {
		if val > 100000000 {
			val = val / 100000
		}
		return val
	}
	return 0
}

func htmlUnescape(s string) string {
	return strings.ReplaceAll(s, "&amp;", "&")
}
