package publishers

import (
	"testing"
	"time"
)

func TestNextGoldenHour(t *testing.T) {
	at := func(h, m int) time.Time { return time.Date(2026, 10, 12, h, m, 0, 0, vnZone) }
	cases := []struct {
		now  time.Time
		want time.Time
	}{
		{at(8, 0), at(11, 30)},
		{at(10, 30), at(19, 30)}, // còn < 90 phút tới 11:30
		{at(15, 0), at(19, 30)},
		{at(19, 0), time.Date(2026, 10, 13, 11, 30, 0, 0, vnZone)},
	}
	for _, c := range cases {
		if got := NextGoldenHour(c.now); !got.Equal(c.want) {
			t.Errorf("now %v → %v, muốn %v", c.now, got, c.want)
		}
	}
}
