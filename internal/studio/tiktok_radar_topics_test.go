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
	// Topic creation stays independent from the background crawl.
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
			Analysis: &TikTokRadarContentAnalysis{
				Version:         tikTokRadarAnalysisVersion,
				Status:          "complete",
				Topic:           topic,
				DanceDetected:   true,
				DanceConfidence: 0.95,
				TopicRelevance:  0.95,
			},
		})
	}
	for i := range vids {
		vids[i].Analysis.SourceHash = radarSourceFingerprint(vids[i])
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

func TestTikTokRadarDuplicateVideoIDUpdatesExistingRecord(t *testing.T) {
	st, err := New(filepath.Join(t.TempDir(), "radar.db"), nil, nil, nil, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ctx := context.Background()
	first := TikTokTrendVideo{
		VideoID: "1234567890", Title: "old caption", VideoURL: "https://www.tiktok.com/@creator/video/1234567890",
		PlayCount: 100, PlayCountKnown: true,
	}
	if _, err := st.persistTikTokRadarResults(ctx, []TikTokTrendVideo{first}); err != nil {
		t.Fatal(err)
	}
	updated := first
	updated.Title = "updated caption"
	updated.PlayCount = 250
	if _, err := st.persistTikTokRadarResults(ctx, []TikTokTrendVideo{updated}); err != nil {
		t.Fatal(err)
	}
	stored, err := st.loadTikTokRadarVideo(ctx, first.VideoID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Title != updated.Title || stored.PlayCount != updated.PlayCount {
		t.Fatalf("duplicate ID did not refresh metadata: %+v", stored)
	}
	var count int
	if err := st.db.QueryRow(`SELECT COUNT(*) FROM studio_tiktok_radar_videos WHERE video_id=?`, first.VideoID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("stored %d records for duplicate video ID, want 1", count)
	}
}
