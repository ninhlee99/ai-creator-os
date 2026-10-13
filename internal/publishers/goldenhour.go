package publishers

// Khung giờ vàng (giờ Việt Nam) để hẹn giờ công khai video YouTube.
// Bật bằng YOUTUBE_GOLDEN_HOUR=1: video vẫn tải lên ở chế độ private,
// kèm status.publishAt → YouTube tự công khai đúng giờ vàng kế tiếp.
// Muốn duyệt trước thì vào YouTube Studio đổi/hủy lịch trước giờ đó.

import "time"

// vnZone: UTC+7 cố định (Việt Nam không đổi giờ mùa hè).
var vnZone = time.FixedZone("ICT", 7*3600)

// goldenSlots: giờ:phút cao điểm lướt mạng — nghỉ trưa và buổi tối.
var goldenSlots = [][2]int{{11, 30}, {19, 30}}

// goldenLead: lịch phải cách hiện tại ít nhất ngần này (YouTube cần
// thời gian xử lý video trước giờ công khai).
const goldenLead = 90 * time.Minute

// NextGoldenHour trả khung giờ vàng sớm nhất cách now ≥ goldenLead.
func NextGoldenHour(now time.Time) time.Time {
	n := now.In(vnZone)
	for d := 0; d < 3; d++ {
		day := n.AddDate(0, 0, d)
		for _, s := range goldenSlots {
			t := time.Date(day.Year(), day.Month(), day.Day(), s[0], s[1], 0, 0, vnZone)
			if t.Sub(now) >= goldenLead {
				return t
			}
		}
	}
	return now.Add(24 * time.Hour)
}

// goldenHourEnabled đọc YOUTUBE_GOLDEN_HOUR[_<USERNAME>].
func goldenHourEnabled(username string) bool {
	return envFor(username, "YOUTUBE_GOLDEN_HOUR") == "1"
}
