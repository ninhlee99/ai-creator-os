package growth

import (
	"time"
	_ "time/tzdata"
)

// ------------------------------------------------------------ YT quota
//
// YouTube currently gives videos.insert its own quota bucket: 100 calls/day
// by default, at one unit per call. This is project-wide, shared with any
// other clients using that Google Cloud project. Google resets it at
// midnight Pacific Time.

// UploadCostUnits is the documented cost of one videos.insert call.
const UploadCostUnits = 1

// DailyQuotaUnits is the documented default videos.insert bucket per project.
const DailyQuotaUnits = 100

// LegacyUploadCostUnits is the videos.insert cost used by the old shared
// Data API bucket. Existing successful-upload counters are migrated from it.
const LegacyUploadCostUnits = 1600

// YouTubeQuotaDay returns date key matching Google's videos.insert reset.
func YouTubeQuotaDay(now time.Time) string {
	loc, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		return now.UTC().Format("2006-01-02")
	}
	return now.In(loc).Format("2006-01-02")
}

// QuotaCanUpload reports whether one more upload fits today's budget.
func QuotaCanUpload(usedToday int) bool {
	return usedToday+UploadCostUnits <= DailyQuotaUnits
}

// QuotaUploadsLeft is how many uploads still fit today's budget.
func QuotaUploadsLeft(usedToday int) int {
	left := (DailyQuotaUnits - usedToday) / UploadCostUnits
	if left < 0 {
		return 0
	}
	return left
}
