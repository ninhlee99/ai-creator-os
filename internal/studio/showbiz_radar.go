package studio

import (
	"context"
	"encoding/xml"
	"fmt"
	"html"
	"net/http"
	"strings"
	"time"
)

func FetchSplitShowbizDrama(ctx context.Context) ([]ShowbizDrama, []ShowbizDrama, error) {
	dramaMu.Lock()
	if time.Since(lastDramaFetch) < showbizDramaTTL && (len(cachedDramasVN) > 0 || len(cachedDramasCN) > 0) {
		vn := append([]ShowbizDrama(nil), cachedDramasVN...)
		cn := append([]ShowbizDrama(nil), cachedDramasCN...)
		dramaMu.Unlock()
		return vn, cn, nil
	}
	dramaMu.Unlock()

	kItems, kErr := fetchKenh14Showbiz(ctx)
	vItems, vErr := fetchVnExpressShowbiz(ctx)
	if kErr != nil && vErr != nil {
		return nil, nil, fmt.Errorf("nguồn tin showbiz hiện không khả dụng")
	}
	var vn, cn []ShowbizDrama
	seen := make(map[string]bool)
	for _, item := range append(kItems, vItems...) {
		key := strings.ToLower(strings.TrimSpace(item.Title))
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		if item.Region == "CN" {
			cn = append(cn, item)
			if len(cn) == 10 {
				continue
			}
		} else {
			vn = append(vn, item)
			if len(vn) == 10 {
				continue
			}
		}
	}
	for i := range vn {
		vn[i].Rank = i + 1
	}
	for i := range cn {
		cn[i].Rank = i + 1
	}

	dramaMu.Lock()
	cachedDramasVN = append([]ShowbizDrama(nil), vn...)
	cachedDramasCN = append([]ShowbizDrama(nil), cn...)
	lastDramaFetch = time.Now()
	dramaMu.Unlock()
	return vn, cn, nil
}

// InvalidateTikTokRadarCache xóa cache radar để ép quét dữ liệu thời gian thực (realtime refresh).
type rssFeed struct {
	XMLName xml.Name   `xml:"rss"`
	Channel rssChannel `xml:"channel"`
}

type rssChannel struct {
	Items []rssItem `xml:"item"`
}

type rssItem struct {
	Title       string `xml:"title"`
	Link        string `xml:"link"`
	Description string `xml:"description"`
	PubDate     string `xml:"pubDate"`
}

func fetchKenh14Showbiz(ctx context.Context) ([]ShowbizDrama, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://kenh14.vn/star.rss", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36")

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var feed rssFeed
	if err := xml.NewDecoder(resp.Body).Decode(&feed); err != nil {
		return nil, err
	}

	return parseDramaItems(feed.Channel.Items), nil
}

func fetchVnExpressShowbiz(ctx context.Context) ([]ShowbizDrama, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://vnexpress.net/rss/giai-tri.rss", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36")

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var feed rssFeed
	if err := xml.NewDecoder(resp.Body).Decode(&feed); err != nil {
		return nil, err
	}

	return parseDramaItems(feed.Channel.Items), nil
}

func parseDramaItems(items []rssItem) []ShowbizDrama {
	var dramas []ShowbizDrama
	cbizKeywords := []string{
		"trung quốc", "c-biz", "cbiz", "hoa ngữ", "bắc kinh", "thượng hải",
		"từ nghệ dương", "hoàng tử thao", "tiêu chiến", "vương nhất bác",
		"dương tử", "bạch lộc", "triệu lộ tư", "triệu lệ dĩnh", "lưu diệc phi",
		"phạm băng băng", "huỳnh hiểu minh", "angelababy", "weibo", "douyin",
	}

	for _, it := range items {
		title := strings.TrimSpace(html.UnescapeString(it.Title))
		rawDesc := it.Description

		// Tách ảnh
		imageURL := ""
		if m := imgRegex.FindStringSubmatch(rawDesc); len(m) > 1 {
			imageURL = m[1]
		}

		// Làm sạch văn bản tóm tắt
		cleanSummary := strings.TrimSpace(htmlTagRegex.ReplaceAllString(rawDesc, ""))
		cleanSummary = strings.TrimSpace(html.UnescapeString(cleanSummary))
		if len(cleanSummary) > 180 {
			cleanSummary = cleanSummary[:177] + "..."
		}

		// Nhận diện Showbiz Trung (C-Biz) vs Showbiz Việt (V-Biz)
		textSearch := strings.ToLower(title + " " + cleanSummary)
		isCBiz := false
		for _, kw := range cbizKeywords {
			if strings.Contains(textSearch, kw) {
				isCBiz = true
				break
			}
		}

		category := "Showbiz Việt (V-Biz)"
		region := "VN"
		if isCBiz {
			category = "Showbiz Trung (C-Biz)"
			region = "CN"
		}

		hook := fmt.Sprintf("Biến căng showbiz: %s! Thực hư vụ việc thế nào?", title)
		if len(hook) > 85 {
			hook = hook[:82] + "..."
		}

		dramas = append(dramas, ShowbizDrama{
			Title:       title,
			Category:    category,
			Region:      region,
			Summary:     cleanSummary,
			ImageURL:    imageURL,
			SourceURL:   it.Link,
			PublishedAt: it.PubDate,
			Hook3s:      hook,
		})
	}
	return dramas
}

// AnalyzeTikTokVideoURL phân tích trực tiếp một link video TikTok do người dùng cung cấp.
