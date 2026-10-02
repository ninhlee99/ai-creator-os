package growth

// ------------------------------------------------------------ YT quota
//
// YouTube Data API v3 quota is a hard per-PROJECT ceiling (docs §3.5):
// 10,000 units/day by default and one videos.insert costs 1,600 — roughly
// 6 uploads a day for the whole project, not per channel. The growth
// publisher therefore keeps a daily counter and defers uploads instead of
// failing against the API wall.

// UploadCostUnits is the documented cost of one videos.insert call.
const UploadCostUnits = 1600

// DailyQuotaUnits is the default Data API v3 daily quota per project.
const DailyQuotaUnits = 10000

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
