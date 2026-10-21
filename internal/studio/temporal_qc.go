package studio

import (
	"bytes"
	"context"
	"fmt"
	"math"
	"os/exec"
)

const (
	temporalMotionFPS        = 2
	temporalMotionFrameSize  = 96
	temporalMotionPixelCount = temporalMotionFrameSize * temporalMotionFrameSize
	minMotionActiveRatio     = 0.25
	minMotionFrameDifference = 0.25
	activeMotionDifference   = 0.35
)

// TemporalMotionEvidence records grayscale frame changes. It detects frozen
// or nearly frozen shots; it does not prove that a person used the product.
type TemporalMotionEvidence struct {
	Shot               int     `json:"shot"`
	SampledFrames      int     `json:"sampled_frames"`
	Transitions        int     `json:"transitions"`
	ActiveTransitions  int     `json:"active_transitions"`
	ActiveRatio        float64 `json:"active_ratio"`
	MeanGrayDifference float64 `json:"mean_gray_difference"`
}

func measureAffiliateTemporalMotion(ctx context.Context, videoPath string, shots []ProShot) ([]TemporalMotionEvidence, error) {
	if len(shots) == 0 {
		return nil, fmt.Errorf("temporal QC cần shot list")
	}
	var totalSeconds float64
	for _, shot := range shots {
		if shot.Seconds <= 0 {
			return nil, fmt.Errorf("temporal QC gặp shot duration không hợp lệ")
		}
		totalSeconds += float64(shot.Seconds)
	}
	if got := probeDuration(ctx, videoPath); got <= 0 || math.Abs(got-totalSeconds) > 0.35 {
		return nil, fmt.Errorf("temporal QC không khớp thời lượng video và shot list")
	}

	filter := fmt.Sprintf("fps=%d,scale=%d:%d:force_original_aspect_ratio=increase,crop=%d:%d,format=gray",
		temporalMotionFPS, temporalMotionFrameSize, temporalMotionFrameSize, temporalMotionFrameSize, temporalMotionFrameSize)
	cmd := exec.CommandContext(ctx, "ffmpeg", "-nostdin", "-v", "error", "-i", videoPath,
		"-an", "-vf", filter, "-f", "rawvideo", "-pix_fmt", "gray", "pipe:1")
	var raw, stderr bytes.Buffer
	cmd.Stdout = &raw
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("đọc chuỗi frame để đo chuyển động: %w: %s", err, stderr.String())
	}
	if raw.Len() == 0 || raw.Len()%temporalMotionPixelCount != 0 {
		return nil, fmt.Errorf("chuỗi frame temporal QC rỗng hoặc không đầy đủ")
	}
	frameCount := raw.Len() / temporalMotionPixelCount
	if frameCount < len(shots)*3 {
		return nil, fmt.Errorf("temporal QC chỉ đọc được %d frame cho %d shot", frameCount, len(shots))
	}

	out := make([]TemporalMotionEvidence, len(shots))
	for i := range shots {
		out[i].Shot = i + 1
	}
	shotStarts := make([]float64, len(shots))
	shotEnds := make([]float64, len(shots))
	cursor := 0.0
	for i, shot := range shots {
		shotStarts[i] = cursor
		cursor += float64(shot.Seconds)
		shotEnds[i] = cursor
	}

	for i := 0; i < frameCount; i++ {
		at := float64(i) / temporalMotionFPS
		for shot := range shots {
			if at >= shotStarts[shot] && at < shotEnds[shot] {
				out[shot].SampledFrames++
				break
			}
		}
		if i == frameCount-1 {
			break
		}
		from := float64(i) / temporalMotionFPS
		to := float64(i+1) / temporalMotionFPS
		shot := shotForTemporalTransition(from, to, shotStarts, shotEnds)
		if shot < 0 {
			continue // Do not count a transition that crosses a cut between shots.
		}
		left := raw.Bytes()[i*temporalMotionPixelCount : (i+1)*temporalMotionPixelCount]
		right := raw.Bytes()[(i+1)*temporalMotionPixelCount : (i+2)*temporalMotionPixelCount]
		difference := 0.0
		for pixel := range left {
			difference += math.Abs(float64(left[pixel]) - float64(right[pixel]))
		}
		difference /= temporalMotionPixelCount
		out[shot].Transitions++
		out[shot].MeanGrayDifference += difference
		if difference >= activeMotionDifference {
			out[shot].ActiveTransitions++
		}
	}

	for i := range out {
		if out[i].Transitions > 0 {
			out[i].MeanGrayDifference /= float64(out[i].Transitions)
			out[i].ActiveRatio = float64(out[i].ActiveTransitions) / float64(out[i].Transitions)
		}
	}
	return out, nil
}

func shotForTemporalTransition(from, to float64, starts, ends []float64) int {
	for i := range starts {
		if from >= starts[i] && to < ends[i] {
			return i
		}
	}
	return -1
}

func affiliateTemporalMotionPass(evidence []TemporalMotionEvidence, expectedShots int) bool {
	if expectedShots <= 0 || len(evidence) != expectedShots {
		return false
	}
	for i, shot := range evidence {
		if shot.Shot != i+1 || shot.SampledFrames < 3 || shot.Transitions < 2 ||
			shot.ActiveRatio < minMotionActiveRatio || shot.MeanGrayDifference < minMotionFrameDifference {
			return false
		}
	}
	return true
}
