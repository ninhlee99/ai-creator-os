package studio

import "testing"

func TestTikTokRadarAnalysisFiltersDanceAndTopicEvidence(t *testing.T) {
	tests := []struct {
		name     string
		topic    string
		analysis TikTokRadarContentAnalysis
		want     bool
	}{
		{
			name:  "viral feed needs dance",
			topic: "Xu hướng Việt Nam",
			analysis: TikTokRadarContentAnalysis{
				Version: tikTokRadarAnalysisVersion, Status: "complete", Topic: "Xu hướng Việt Nam",
				DanceDetected: true, DanceConfidence: 0.8,
			},
			want: true,
		},
		{
			name:  "default dance confidence below threshold",
			topic: "Xu hướng Việt Nam",
			analysis: TikTokRadarContentAnalysis{
				Version: tikTokRadarAnalysisVersion, Status: "complete", Topic: "Xu hướng Việt Nam",
				DanceDetected: true, DanceConfidence: 0.5, TopicRelevance: 0.9,
			},
		},
		{
			name:  "custom category needs topic match",
			topic: "Thời trang",
			analysis: TikTokRadarContentAnalysis{
				Version: tikTokRadarAnalysisVersion, Status: "complete", Topic: "Thời trang",
				DanceDetected: false, TopicRelevance: 0.7,
			},
			want: true,
		},
		{
			name:  "custom category rejects unrelated video",
			topic: "Thời trang",
			analysis: TikTokRadarContentAnalysis{
				Version: tikTokRadarAnalysisVersion, Status: "complete", Topic: "Thời trang",
				TopicRelevance: 0.3,
			},
		},
		{
			name:  "incomplete analysis is retryable",
			topic: "Thời trang",
			analysis: TikTokRadarContentAnalysis{
				Version: tikTokRadarAnalysisVersion, Status: "error", Topic: "Thời trang",
				TopicRelevance: 1,
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			video := TikTokTrendVideo{VideoID: tc.name, Caption: "caption", Title: "title", Music: "sound"}
			tc.analysis.SourceHash = radarSourceFingerprint(video)
			if got := tc.analysis.qualifies(tc.topic, video); got != tc.want {
				t.Fatalf("qualifies(%q) = %t, want %t", tc.topic, got, tc.want)
			}
		})
	}
}

func TestTikTokRadarTrendingSortUsesGrowthBeforeTotalViews(t *testing.T) {
	videos := []TikTokTrendVideo{
		{VideoID: "old-viral", PlayCount: 10_000_000, PlayCountKnown: true},
		{VideoID: "rising", PlayCount: 100_000, PlayCountKnown: true, ViewsPerHour: 2_000, VelocityKnown: true},
		{VideoID: "engaging", PlayCount: 1_000_000, PlayCountKnown: true, EngagementRate: 0.2, EngagementKnown: true},
	}
	sortTikTokRadarVideosByPopularity(videos)
	if videos[0].VideoID != "rising" || videos[1].VideoID != "engaging" || videos[2].VideoID != "old-viral" {
		t.Fatalf("trend sort order = %q, %q, %q", videos[0].VideoID, videos[1].VideoID, videos[2].VideoID)
	}
}
