package web

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/ninhlee99/ai-creator-os/internal/growth"
)

// TestAccountDetailFollowersComeFromSnapshot: the detail page shows the
// follower count from the latest growth snapshot; without one it says
// "chưa kết nối"; and the old manual followers form is gone.
func TestAccountDetailFollowersComeFromSnapshot(t *testing.T) {
	s := newTestServer(t)
	if s.Growth == nil {
		t.Skip("growth store unavailable")
	}
	a, err := s.Mgr.Add("folacc", "", "")
	if err != nil {
		t.Fatal(err)
	}
	path := fmt.Sprintf("/accounts/%d", a.ID)

	// No snapshot yet -> honest "not connected", and no manual form.
	body := get(t, s, path).Body.String()
	if !strings.Contains(body, "chưa kết nối") {
		t.Error("detail without snapshot must say chưa kết nối")
	}
	if strings.Contains(body, "Cập nhật followers") || strings.Contains(body, "/followers\"") {
		t.Error("manual followers form is still rendered")
	}

	// After a synced snapshot, the real count appears.
	fol := int64(4321)
	if err := s.Growth.InsertSnapshot(growth.Snapshot{
		AccountID: a.ID, TakenAt: time.Now(), Source: "youtube_api", Followers: &fol,
	}); err != nil {
		t.Fatal(err)
	}
	body = get(t, s, path).Body.String()
	if !strings.Contains(body, "4321") {
		t.Error("detail must show followers from the latest snapshot")
	}
}
