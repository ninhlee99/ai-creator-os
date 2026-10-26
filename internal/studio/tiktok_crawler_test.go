package studio

import (
	"testing"
)

func TestTikTokCrawlerConfigured(t *testing.T) {
	// Should be configured if Edge, Chrome, Brave or Chromium is on the machine
	configured := TikTokCrawlerConfigured()
	if !configured {
		t.Log("Note: No Chromium-based browser detected in standard paths")
	}
}

func TestParseTikTokCrawlerJSONSearch(t *testing.T) {
	sampleJSON := `{
		"status_code": 0,
		"data": [
			{
				"type": 1,
				"item": {
					"id": "7700162918087724296",
					"desc": "Nhảy TikTok trend #xuhuong #dance",
					"author": {
						"nickname": "Mai Dancer",
						"uniqueId": "maidance99"
					},
					"stats": {
						"playCount": 2500000,
						"diggCount": 150000,
						"commentCount": 3500,
						"shareCount": 12000
					},
					"video": {
						"cover": "https://img.tiktokcdn.com/cover1.jpg",
						"originCover": "https://img.tiktokcdn.com/origin1.jpg"
					},
					"music": {
						"title": "Nhạc nhảy remix hot"
					}
				}
			},
			{
				"type": 1,
				"item": {
					"id": "7700162918087724297",
					"desc": "Điệu nhảy viral",
					"author": {
						"nickname": "Dancer 2",
						"uniqueId": "dancer2"
					},
					"stats": {
						"playCount": 850000,
						"diggCount": 45000,
						"commentCount": 500,
						"shareCount": 1200
					},
					"video": {
						"cover": "https://img.tiktokcdn.com/cover2.jpg"
					},
					"music": {
						"title": "Sound gốc"
					}
				}
			}
		]
	}`

	videos := parseTikTokCrawlerJSON([]byte(sampleJSON))
	if len(videos) != 2 {
		t.Fatalf("parseTikTokCrawlerJSON returned %d videos, want 2", len(videos))
	}

	v1 := videos[0]
	if v1.VideoID != "7700162918087724296" {
		t.Errorf("VideoID = %s, want 7700162918087724296", v1.VideoID)
	}
	if v1.Title != "Nhảy TikTok trend #xuhuong #dance" {
		t.Errorf("Title = %s", v1.Title)
	}
	if v1.Author != "Mai Dancer" || v1.UniqueID != "maidance99" {
		t.Errorf("Author info mismatch: %s / %s", v1.Author, v1.UniqueID)
	}
	if v1.PlayCount != 2500000 || v1.Views != "2.5M" || !v1.PlayCountKnown {
		t.Errorf("PlayCount = %d, Views = %s, Known = %v", v1.PlayCount, v1.Views, v1.PlayCountKnown)
	}
	if v1.DiggCount != 150000 || v1.Likes != "150K" || !v1.DiggCountKnown {
		t.Errorf("DiggCount = %d, Likes = %s", v1.DiggCount, v1.Likes)
	}
	if v1.Music != "Nhạc nhảy remix hot" {
		t.Errorf("Music = %s", v1.Music)
	}
	if v1.VideoURL != "https://www.tiktok.com/@maidance99/video/7700162918087724296" {
		t.Errorf("VideoURL = %s", v1.VideoURL)
	}
	if v1.CoverURL != "https://img.tiktokcdn.com/cover1.jpg" {
		t.Errorf("CoverURL = %s", v1.CoverURL)
	}
	if v1.Provider != TikTokCrawlerSource {
		t.Errorf("Provider = %s, want %s", v1.Provider, TikTokCrawlerSource)
	}
}

func TestParseTikTokCrawlerJSONExplore(t *testing.T) {
	exploreJSON := `{
		"status_code": 0,
		"itemList": [
			{
				"id": "7697248772358769938",
				"desc": "Áo dài đón tết #aodai #xuhuong",
				"author": {
					"nickname": "Shop Áo Dài",
					"uniqueId": "aodaivn"
				},
				"stats": {
					"playCount": 1200000,
					"diggCount": 98000,
					"commentCount": 1200,
					"shareCount": 5400
				},
				"video": {
					"cover": "https://img.tiktokcdn.com/aodai.jpg"
				},
				"music": {
					"title": "Nhạc tết rộn ràng"
				}
			}
		]
	}`

	videos := parseTikTokCrawlerJSON([]byte(exploreJSON))
	if len(videos) != 1 {
		t.Fatalf("parseTikTokCrawlerJSON returned %d videos, want 1", len(videos))
	}
	if videos[0].VideoID != "7697248772358769938" {
		t.Errorf("VideoID = %s, want 7697248772358769938", videos[0].VideoID)
	}
	if videos[0].Views != "1.2M" || videos[0].Likes != "98K" {
		t.Errorf("Views = %s, Likes = %s", videos[0].Views, videos[0].Likes)
	}
}

func TestParseTikTokCrawlerJSONEmpty(t *testing.T) {
	if videos := parseTikTokCrawlerJSON([]byte(`{"status_code":0,"data":[]}`)); len(videos) != 0 {
		t.Errorf("expected 0 videos, got %d", len(videos))
	}
	if videos := parseTikTokCrawlerJSON([]byte(`invalid json`)); len(videos) != 0 {
		t.Errorf("expected nil for invalid json, got %d", len(videos))
	}
}
