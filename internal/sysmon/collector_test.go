package sysmon

import (
	"testing"
)

func TestGetTotalRAMMB(t *testing.T) {
	total := GetTotalRAMMB()
	if total == 0 {
		t.Errorf("GetTotalRAMMB returned 0")
	}
	t.Logf("Total RAM: %d MB", total)
}

func TestReadMemoryUsage(t *testing.T) {
	total := GetTotalRAMMB()
	used, free, pct := ReadMemoryUsage(total)
	if used == 0 && free == 0 {
		t.Errorf("ReadMemoryUsage returned zeroes")
	}
	if pct < 0 || pct > 100 {
		t.Errorf("RAM percent out of bounds: %.2f", pct)
	}
	t.Logf("Memory: %d MB used / %d MB free (%.2f%%)", used, free, pct)
}

func TestReadCPULoad(t *testing.T) {
	pct, l1, l5 := ReadCPULoad(0)
	if pct < 0 || pct > 100 {
		t.Errorf("CPU load pct out of bounds: %.2f", pct)
	}
	t.Logf("CPU: %.2f%% (load1=%.2f, load5=%.2f)", pct, l1, l5)
}

func TestReadHeavyProcesses(t *testing.T) {
	procs := ReadHeavyProcesses()
	t.Logf("Found %d heavy processes", len(procs))
	for _, p := range procs {
		t.Logf("PID %d: %s (%s) - CPU: %.1f%%, RAM: %.1f MB", p.PID, p.Name, p.Role, p.CPUPercent, p.MemoryMB)
	}
}

func TestClassifyNodeResource(t *testing.T) {
	tier, label := ClassifyNodeResource("video_upscale")
	if tier != "danger" {
		t.Errorf("expected danger tier for video_upscale, got %s", tier)
	}
	if label == "" {
		t.Errorf("empty label for video_upscale")
	}

	tier, _ = ClassifyNodeResource("tts")
	if tier != "normal" {
		t.Errorf("expected normal tier for tts, got %s", tier)
	}
}

func TestCollect(t *testing.T) {
	snap := Collect([]ActiveNodeInfo{
		{
			RunID:        "test-run-1",
			WorkflowName: "TikTok Viral Dance",
			NodeID:       "node_upscale",
			NodeTitle:    "Làm nét & Nâng cấp video 2K",
			NodeType:     "video_upscale",
			ResourceTier: "danger",
			TierLabel:    "🔴 Tải Rất Cao",
			ElapsedSec:   15,
		},
	})
	if snap == nil {
		t.Fatal("Collect returned nil")
	}
	if snap.TotalRAMMB == 0 {
		t.Errorf("snap.TotalRAMMB is 0")
	}
	if len(snap.ActiveNodes) != 1 {
		t.Errorf("expected 1 active node, got %d", len(snap.ActiveNodes))
	}
	t.Logf("Snapshot overall status: %s - %s", snap.OverallStatus, snap.StatusMessage)
}
