package studio

import (
	"context"
	"path/filepath"
	"testing"
)

func TestTikTokRadarTopicsCRUD(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test_studio.db")

	st, err := New(dbPath, nil, nil, nil, t.TempDir())
	if err != nil {
		t.Fatalf("OpenStudio error: %v", err)
	}
	defer st.Close()

	ctx := context.Background()

	// 1. Initial list should contain default topics
	topics, err := st.ListTikTokRadarTopics(ctx)
	if err != nil {
		t.Fatalf("ListTikTokRadarTopics error: %v", err)
	}
	if len(topics) < 5 {
		t.Errorf("expected at least 5 default topics, got %d", len(topics))
	}

	// 2. Add custom topic
	customTopic := "Đồ gia dụng thông minh"
	// Note: AddTikTokRadarTopic calls CollectTikTokRadarVideosForTopic which might try crawling if browser is available.
	// Let's directly insert or test with DB method:
	_, err = st.db.Exec(`INSERT INTO studio_tiktok_radar_topics(name, created_at) VALUES(?, '2026-10-26T12:00:00Z')`, customTopic)
	if err != nil {
		t.Fatalf("insert topic error: %v", err)
	}

	topicsAfter, err := st.ListTikTokRadarTopics(ctx)
	if err != nil {
		t.Fatalf("ListTikTokRadarTopics after insert: %v", err)
	}
	found := false
	for _, top := range topicsAfter {
		if top == customTopic {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected to find custom topic %q in %v", customTopic, topicsAfter)
	}

	// 3. Delete custom topic
	if err := st.DeleteTikTokRadarTopic(ctx, customTopic); err != nil {
		t.Fatalf("DeleteTikTokRadarTopic error: %v", err)
	}

	topicsAfterDelete, err := st.ListTikTokRadarTopics(ctx)
	if err != nil {
		t.Fatalf("ListTikTokRadarTopics after delete: %v", err)
	}
	for _, top := range topicsAfterDelete {
		if top == customTopic {
			t.Errorf("custom topic %q should have been deleted", customTopic)
		}
	}
}

func TestTikTokRadarTop8Limit(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test_studio.db")

	st, err := New(dbPath, nil, nil, nil, t.TempDir())
	if err != nil {
		t.Fatalf("OpenStudio error: %v", err)
	}
	defer st.Close()

	ctx := context.Background()

	// Create 8 dummy videos
	topic := "Thời trang"
	var vids []TikTokTrendVideo
	for i := 1; i <= 8; i++ {
		vids = append(vids, TikTokTrendVideo{
			VideoID:        string(rune('1' + i)),
			Title:          "Thời trang VN",
			Region:         "VN",
			VideoURL:       "https://www.tiktok.com/@shop/video/" + string(rune('1'+i)),
			PlayCount:      int64(i * 100000),
			PlayCountKnown: true,
			DiggCount:      int64(i * 5000),
			DiggCountKnown: true,
		})
	}

	// Persist videos into studio DB
	if _, err := st.persistTikTokRadarResults(ctx, vids); err != nil {
		t.Fatalf("persistTikTokRadarResults: %v", err)
	}

	// Save top videos for topic
	sortTikTokRadarVideosByPopularity(vids)
	if len(vids) > tikTokRadarFeedLimit {
		vids = vids[:tikTokRadarFeedLimit]
	}
	for i := range vids {
		vids[i].Rank = i + 1
	}
	if err := st.saveTopicVideos(ctx, topic, vids); err != nil {
		t.Fatalf("saveTopicVideos: %v", err)
	}

	// Query top videos for topic
	results, err := st.GetTikTokRadarVideosForTopic(ctx, topic)
	if err != nil {
		t.Fatalf("GetTikTokRadarVideosForTopic: %v", err)
	}

	if len(results) != 8 {
		t.Fatalf("expected exactly 8 videos, got %d", len(results))
	}

	// Verify they are sorted by PlayCount descending
	if results[0].PlayCount < results[1].PlayCount || results[1].PlayCount < results[2].PlayCount {
		t.Errorf("videos are not sorted descending by play count: %v, %v, %v",
			results[0].PlayCount, results[1].PlayCount, results[2].PlayCount)
	}

	for i, r := range results {
		if r.Rank != i+1 {
			t.Errorf("expected Rank %d, got %d", i+1, r.Rank)
		}
	}
}
