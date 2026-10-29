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

	args := webai.OptimizedHeadlessBrowserArgs(0, profileDir, "1280,800")
	args = append(args[:len(args)-1],
		"--user-agent=Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36",
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
		"source": `try{
			Object.defineProperty(navigator, 'webdriver', { get: () => undefined });
			window.chrome = {
				app: { isInstalled: false },
				runtime: {},
				loadTimes: () => {},
				csi: () => {}
			};
			Object.defineProperty(navigator, 'plugins', { get: () => [1, 2, 3, 4, 5] });
			Object.defineProperty(navigator, 'languages', { get: () => ['vi-VN', 'vi', 'en-US', 'en'] });
		}catch(_){}`,
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

	// Script thực thi: phát hiện & click popup, mở rộng thẻ "About this product" / "Product description",
	// cuộn trang từ từ để tải toàn bộ ảnh mô tả sắc nét, trích xuất ảnh và video từ DOM và JSON State
	evalScript := `(async () => {
		const logs = [];

		// 1. Cuộn nhẹ để kích hoạt popup "Get the full app experience" nếu có
		window.scrollBy({ top: 350, behavior: 'instant' });
		await new Promise(r => setTimeout(r, 700));

		// 2. Phát hiện popup và click "Not now" hoặc dấu "✕"
		let dismissed = false;
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

		await new Promise(r => setTimeout(r, 500));

		// 3. Tự động mở rộng (Expand) thẻ "About this product" / "Product description" / "Mô tả sản phẩm"
		const expandDescription = () => {
			const keywords = ['about this product', 'product description', 'mô tả sản phẩm', 'chi tiết sản phẩm', 'xem thêm', 'view more', 'see more', 'read more'];
			const candidates = Array.from(document.querySelectorAll('h2, h3, h4, h5, button, div, span, p, a'));
			for (const el of candidates) {
				const txt = (el.innerText || el.textContent || '').trim().toLowerCase();
				if (keywords.some(kw => txt === kw || (txt.includes(kw) && txt.length < 50))) {
					const clickTarget = el.closest('button') || el.querySelector('button') || el.parentElement?.querySelector('button, [class*="arrow" i], [class*="chevron" i], [class*="expand" i], [class*="icon" i]') || el;
					if (clickTarget && typeof clickTarget.click === 'function') {
						try {
							clickTarget.click();
							logs.push('Expanded: ' + txt);
						} catch(_) {}
					}
				}
			}
		};
		expandDescription();

		// 4. Cuộn từ từ toàn bộ trang để kích hoạt toàn bộ lazy-load ảnh trong thẻ mô tả sản phẩm
		const scrollMax = Math.min(document.body.scrollHeight || 6000, 8000);
		for (let top = 400; top <= scrollMax; top += 600) {
			window.scrollTo({ top: top, behavior: 'instant' });
			await new Promise(r => setTimeout(r, 160));
		}

		// Gọi lại expand một lần nữa sau khi đã cuộn (nút "Xem thêm" có thể xuất hiện sau khi cuộn)
		expandDescription();
		await new Promise(r => setTimeout(r, 600));

		// 5. Trích xuất văn bản mô tả (Product description)
		let descriptionText = '';
		const descEls = document.querySelectorAll('[class*="product-description" i], [class*="product_description" i], [class*="detail-desc" i], [class*="desc-detail" i], [data-testid*="desc" i], [class*="desc" i]');
		for (const el of descEls) {
			const t = (el.innerText || '').trim();
			if (t.length > descriptionText.length && !t.includes('©') && !t.includes('TikTok Shop')) {
				descriptionText = t;
			}
		}

		// 6. Trích xuất toàn bộ hình ảnh sản phẩm (cả trong mô tả "About this product", carousel và biến thể)
		const seen = new Set();
		const images = [];
		const videos = [];

		const cleanImgUrl = (u) => {
			if (!u || typeof u !== 'string') return '';
			u = u.trim();
			if (u.startsWith('//')) u = 'https:' + u;
			if (!u.startsWith('http')) return '';
			// Đối với ByteDance / TikTok CDN: không gỡ bỏ toàn bộ template tplv vì CDN sẽ từ chối trả ảnh (lỗi 2004/405).
			// Thay vào đó, nâng cấp resize thành 1080:1080 chất lượng cao
			if (u.includes('ibyteimg.com') || u.includes('tiktokcdn.com')) {
				if (u.includes('resize-jpeg:')) {
					u = u.replace(/resize-jpeg:\d+:\d+/g, 'resize-jpeg:1080:1080');
				} else if (!u.includes('~tplv-') && !u.match(/\.(jpeg|jpg|png|webp)($|\?)/i)) {
					u = u + '~tplv-aphluv4xwc-resize-jpeg:1080:1080.jpeg';
				}
			}
			return u;
		};

		const isGoodImage = (u) => {
			if (!u || u.startsWith('data:')) return false;
			const lower = u.toLowerCase();
			if (lower.includes('avatar') || lower.includes('icon') || lower.includes('logo') || lower.includes('1x1') || lower.includes('badge') || lower.includes('pixel') || lower.includes('favicon') || lower.includes('captcha')) {
				return false;
			}
			return lower.includes('ibyteimg.com') || lower.includes('tiktokcdn.com') || lower.includes('susercontent.com') || lower.includes('alicdn.com') || lower.includes('tikicdn.com') || lower.includes('scdn.vn') || lower.match(/\.(jpeg|jpg|png|webp)($|\?)/i);
		};

		const addImg = (u) => {
			const cleaned = cleanImgUrl(u);
			if (cleaned && isGoodImage(cleaned) && !seen.has(cleaned)) {
				seen.add(cleaned);
				images.push(cleaned);
			}
		};

		// 6A. Trích xuất từ tất cả thẻ img
		document.querySelectorAll('img').forEach(img => {
			addImg(img.currentSrc);
			addImg(img.src);
			addImg(img.getAttribute('data-src'));
			addImg(img.getAttribute('data-original-src'));
			addImg(img.getAttribute('data-lazy-src'));
			const srcset = img.getAttribute('srcset');
			if (srcset) {
				srcset.split(',').forEach(s => addImg(s.trim().split(' ')[0]));
			}
		});

		// 6B. Trích xuất từ background-image của các container mô tả
		document.querySelectorAll('[class*="desc" i] *, [class*="detail" i] *, [class*="about" i] *').forEach(el => {
			const bg = window.getComputedStyle(el).backgroundImage;
			if (bg && bg.startsWith('url(')) {
				const m = bg.match(/url\(['"]?(https?:\/\/[^'"]+)['"]?\)/);
				if (m && m[1]) addImg(m[1]);
			}
		});

		// 6C. Trích xuất video sản phẩm (nếu có)
		document.querySelectorAll('video, [class*="video" i] video').forEach(v => {
			const vSrc = v.currentSrc || v.src || v.querySelector('source')?.src || '';
			if (vSrc && vSrc.startsWith('http') && !videos.includes(vSrc)) {
				videos.push(vSrc);
			}
		});

		// 6D. Quét sâu trong dữ liệu JSON hydrate của TikTok Shop (window.__UNIVERSAL_DATA_FOR_REHYDRATION__, SIGI_STATE, RENDER_DATA)
		const deepExtractFromJSON = (obj) => {
			if (!obj) return;
			if (typeof obj === 'string') {
				if (isGoodImage(obj)) addImg(obj);
				if (obj.startsWith('http') && (obj.includes('.mp4') || obj.includes('tiktokcdn.com/video') || obj.includes('bytevcloud.com'))) {
					if (!videos.includes(obj)) videos.push(obj);
				}
				return;
			}
			if (Array.isArray(obj)) {
				for (const item of obj) deepExtractFromJSON(item);
				return;
			}
			if (typeof obj === 'object') {
				if (Array.isArray(obj.urlList)) {
					for (const u of obj.urlList) addImg(u);
				}
				if (Array.isArray(obj.url_list)) {
					for (const u of obj.url_list) addImg(u);
				}
				if (Array.isArray(obj.images)) {
					for (const im of obj.images) deepExtractFromJSON(im);
				}
				for (const k of Object.keys(obj)) {
					const lk = k.toLowerCase();
					if (lk.includes('image') || lk.includes('img') || lk.includes('photo') || lk.includes('picture') || lk.includes('url') || lk.includes('desc') || lk.includes('detail') || lk.includes('video') || lk.includes('play_addr') || lk.includes('sku') || lk.includes('productinfo')) {
						deepExtractFromJSON(obj[k]);
					}
				}
			}
		};

		try {
			if (window.__UNIVERSAL_DATA_FOR_REHYDRATION__) deepExtractFromJSON(window.__UNIVERSAL_DATA_FOR_REHYDRATION__);
			if (window.SIGI_STATE) deepExtractFromJSON(window.SIGI_STATE);
			if (window.RENDER_DATA) deepExtractFromJSON(window.RENDER_DATA);
			const sEl = document.getElementById('__UNIVERSAL_DATA_FOR_REHYDRATION__') || document.querySelector('script#__UNIVERSAL_DATA_FOR_REHYDRATION__') || document.getElementById('SIGI_STATE') || document.getElementById('RENDER_DATA');
			if (sEl && sEl.textContent) {
				try {
					deepExtractFromJSON(JSON.parse(sEl.textContent));
				} catch(_) {}
			}
			document.querySelectorAll('script').forEach(s => {
				const txt = s.textContent || '';
				if (txt.includes('ibyteimg.com') || txt.includes('tos-alisg') || txt.includes('tos-maliva')) {
					const matches = txt.match(/https?:\\?\/\\?\/[^\s"'<>\\]+ibyteimg\.com\\?\/[^\s"'<>\\]+/g);
					if (matches) {
						matches.forEach(m => {
							const clean = m.replace(/\\u002F/g, '/').replace(/\\/g, '');
							addImg(clean);
						});
					}
				}
				if (s.type === 'application/json' || s.id.includes('DATA') || s.id.includes('STATE')) {
					try {
						deepExtractFromJSON(JSON.parse(txt));
					} catch(_) {}
				}
			});
		} catch(e) {
			logs.push('JSON parse: ' + e.message);
		}

		// 7. Trích xuất tiêu đề sản phẩm
		let title = '';
		const h1 = document.querySelector('h1, [class*="product-title" i], [class*="product_title" i], [class*="title" i]');
		if (h1) title = (h1.innerText || '').trim();
		if (!title || title.length < 5) title = document.title || '';

		// 8. Trích xuất giá bán
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

		// 9. Trích xuất tên shop
		let shop = '';
		const shopEl = document.querySelector('[class*="shop-name" i], [class*="shop_name" i], [class*="seller" i], [class*="store" i]');
		if (shopEl) shop = (shopEl.innerText || '').trim();

		return {
			url: window.location.href,
			title: title,
			description: descriptionText,
			shop: shop,
			priceText: priceText,
			originalPriceText: originalPriceText,
			images: images,
			videos: videos,
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
		Description       string   `json:"description"`
		Shop              string   `json:"shop"`
		PriceText         string   `json:"priceText"`
		OriginalPriceText string   `json:"originalPriceText"`
		Images            []string `json:"images"`
		Videos            []string `json:"videos"`
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
		img = cleanCDNImageURL(img)
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
		ogImg = cleanCDNImageURL(ogImg)
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
		Description:    evalRes.Description,
		ImageURLs:      cleanedImages,
		VideoURLs:      evalRes.Videos,
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
