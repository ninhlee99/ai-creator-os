package flow

import "testing"

func TestPlanVideoSegmentsBalancesShortRemainder(t *testing.T) {
	segments, err := planVideoSegments(10.01, 10, 3, 20)
	if err != nil {
		t.Fatalf("planVideoSegments: %v", err)
	}
	if len(segments) != 2 {
		t.Fatalf("got %d segments, want 2", len(segments))
	}
	for i, segment := range segments {
		if segment.Duration < 3 || segment.Duration > 10 {
			t.Fatalf("segment %d duration %.3fs outside [3s,10s]", i+1, segment.Duration)
		}
	}
	if segments[0].Duration != 5.005 || segments[1].Duration != 5.005 || segments[1].Start != segments[0].Duration {
		t.Fatalf("segments do not evenly cover source: %+v", segments)
	}
}

func TestPlanVideoSegmentsRejectsUnsupportedRangesBeforeSplitting(t *testing.T) {
	if _, err := planVideoSegments(200.01, 10, 3, 20); err == nil {
		t.Fatal("accepted video exceeding configured 20-segment limit")
	}
	if _, err := planVideoSegments(2.99, 10, 3, 20); err == nil {
		t.Fatal("accepted segment shorter than configured provider minimum")
	}
	if segments, err := planVideoSegments(3, 10, 3, 20); err != nil || len(segments) != 1 || segments[0].Duration != 3 {
		t.Fatalf("minimum-length clip = %+v, err %v", segments, err)
	}
}
