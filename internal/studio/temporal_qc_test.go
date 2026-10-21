package studio

import (
	"context"
	"path/filepath"
	"testing"
)

func TestAffiliateTemporalMotionRejectsFrozenShot(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "frozen.mp4")
	if err := ffmpegRun(ctx, "-f", "lavfi", "-i", "color=c=gray:s=128x128:r=30:d=5",
		"-t", "5", "-c:v", "libx264", "-pix_fmt", "yuv420p", path); err != nil {
		t.Fatalf("create frozen fixture: %v", err)
	}
	evidence, err := measureAffiliateTemporalMotion(ctx, path, []ProShot{{Index: 1, Seconds: 5}})
	if err != nil {
		t.Fatalf("measure frozen fixture: %v", err)
	}
	if affiliateTemporalMotionPass(evidence, 1) {
		t.Fatalf("frozen clip passed temporal motion gate: %+v", evidence)
	}
}

func TestAffiliateTemporalMotionAcceptsChangingFrames(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "moving-pattern.mp4")
	graph := "testsrc2=s=128x128:r=30:d=5"
	if err := ffmpegRun(ctx, "-f", "lavfi", "-i", graph,
		"-t", "5", "-c:v", "libx264", "-pix_fmt", "yuv420p", path); err != nil {
		t.Fatalf("create moving fixture: %v", err)
	}
	evidence, err := measureAffiliateTemporalMotion(ctx, path, []ProShot{{Index: 1, Seconds: 5}})
	if err != nil {
		t.Fatalf("measure moving fixture: %v", err)
	}
	if !affiliateTemporalMotionPass(evidence, 1) {
		t.Fatalf("moving clip failed temporal motion gate: %+v", evidence)
	}
}

func TestAffiliateTemporalMotionBlocksFrozenIndividualShot(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "moving-then-frozen.mp4")
	graph := "testsrc2=s=128x128:r=30:d=5,tpad=stop_mode=clone:stop_duration=5"
	if err := ffmpegRun(ctx, "-f", "lavfi", "-i", graph,
		"-t", "10", "-c:v", "libx264", "-pix_fmt", "yuv420p", path); err != nil {
		t.Fatalf("create partly frozen fixture: %v", err)
	}
	evidence, err := measureAffiliateTemporalMotion(ctx, path, []ProShot{{Index: 1, Seconds: 5}, {Index: 2, Seconds: 5}})
	if err != nil {
		t.Fatalf("measure partly frozen fixture: %v", err)
	}
	if !affiliateTemporalMotionPass(evidence[:1], 1) {
		t.Fatalf("moving first shot failed temporal motion gate: %+v", evidence[0])
	}
	if affiliateTemporalMotionPass(evidence, 2) {
		t.Fatalf("frozen second shot passed temporal motion gate: %+v", evidence)
	}
}
