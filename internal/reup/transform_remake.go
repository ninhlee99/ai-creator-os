package reup

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ninhlee99/ai-creator-os/internal/studio"
)

// TransformRemake chạy transform MỨC 3 (AI Remake):
// Tái tạo lại video 100% bằng AI — giữ nguyên ý tưởng/cốt truyện viral
// nhưng thay thế toàn bộ nhân vật và hình ảnh bằng AI visuals mới để
// triệt tiêu hoàn toàn rủi ro vi phạm bản quyền / unoriginal content.
func (t *Transformer) TransformRemake(ctx context.Context, v Video, opts TransformOptions) (string, error) {
	if err := t.ensureWorkDir(); err != nil {
		return "", err
	}
	opts.Level = Level3
	tag := fmt.Sprintf("remake-%d-%d", v.ID, time.Now().UnixNano())

	style := strings.TrimSpace(opts.CharacterStyle)
	if style == "" {
		style = "cinematic_vn"
	}

	styleLabel := "Điện ảnh Việt"
	switch style {
	case "anime":
		styleLabel = "Hoạt họa Anime"
	case "3d_avatar":
		styleLabel = "Nhân vật 3D"
	case "fashion":
		styleLabel = "Thời trang hiện đại"
	}

	// 1. Phân cảnh & Kịch bản (3 cảnh: Mở đầu, Diễn biến, Cao trào).
	title := strings.TrimSpace(v.Title)
	if title == "" {
		title = "Khoảnh khắc đặc biệt"
	}

	scenes := []struct {
		SubTitle string
		Niche    string
		DurSec   float64
	}{
		{SubTitle: fmt.Sprintf("[%s] %s", styleLabel, title), Niche: "công nghệ thông minh", DurSec: 5.0},
		{SubTitle: "Khám phá diễn biến thú vị", Niche: "thời trang phong cách", DurSec: 6.0},
		{SubTitle: "Kết thúc bất ngờ — theo dõi ngay", Niche: "gia dụng thông minh", DurSec: 5.0},
	}

	type seg struct {
		path string
		dur  float64
	}
	segs := make([]seg, 0, len(scenes))

	// 2. Tạo hình ảnh nhân vật & phân cảnh mới 100% bằng pure visual synthesis
	for i, sc := range scenes {
		imgPath := filepath.Join(t.WorkDir, fmt.Sprintf("%s-sc%d.png", tag, i))
		if err := studio.GenerateProductVisual(title, sc.Niche, sc.SubTitle, reupOutW, reupOutH, imgPath); err != nil {
			for _, s := range segs {
				os.Remove(s.path)
			}
			return "", fmt.Errorf("reup remake: tạo ảnh cảnh %d: %w", i+1, err)
		}
		defer os.Remove(imgPath)

		// Render ảnh thành video chuyển động Ken Burns
		segPath := filepath.Join(t.WorkDir, fmt.Sprintf("%s-v%d.mp4", tag, i))
		frames := int(sc.DurSec*reupFPS + 0.5)
		if frames < 1 {
			frames = 1
		}

		var zpParams []string
		switch i % 3 {
		case 0: // push-in
			zpParams = []string{
				fmt.Sprintf("z='min(1+0.14*on/%d,1.14)'", frames),
				"x='iw/2-(iw/zoom/2)'", "y='ih/2-(ih/zoom/2)'",
			}
		case 1: // pan ngang
			zpParams = []string{
				"z='1.12'",
				fmt.Sprintf("x='(iw-iw/zoom)*on/%d'", frames),
				"y='ih/2-(ih/zoom/2)'",
			}
		default: // pull-out
			zpParams = []string{
				fmt.Sprintf("z='max(1.14-0.14*on/%d,1.0)'", frames),
				"x='iw/2-(iw/zoom/2)'", "y='ih/2-(ih/zoom/2)'",
			}
		}

		filter := fmt.Sprintf(
			"scale=%d:%d:force_original_aspect_ratio=increase,crop=%d:%d,setsar=1,"+
				"zoompan=d=1:%s:s=%dx%d:fps=%d,"+
				"eq=contrast=1.05:saturation=1.12",
			reupOutW*2, reupOutH*2, reupOutW*2, reupOutH*2,
			strings.Join(zpParams, ":"), reupOutW, reupOutH, reupFPS)

		args := []string{"-loop", "1", "-i", imgPath,
			"-vf", filter,
			"-t", fmt.Sprintf("%.2f", sc.DurSec),
			"-r", fmt.Sprint(reupFPS),
			"-an",
		}
		args = append(args, encoderArgs()...)
		args = append(args, "-pix_fmt", "yuv420p", segPath)

		if err := ffmpegRun(ctx, args...); err != nil {
			for _, s := range segs {
				os.Remove(s.path)
			}
			return "", fmt.Errorf("reup remake: render video cảnh %d: %w", i+1, err)
		}
		segs = append(segs, seg{path: segPath, dur: sc.DurSec})
		defer os.Remove(segPath)
	}

	// 3. Ghép các cảnh qua xfade
	xfadePath := filepath.Join(t.WorkDir, tag+"-x.mp4")
	const xd = 0.5
	xf := fmt.Sprintf(
		"[0:v][1:v]xfade=transition=fade:duration=%.1f:offset=%.3f[x1];"+
			"[x1][2:v]xfade=transition=fade:duration=%.1f:offset=%.3f[vx]",
		xd, segs[0].dur-xd, xd, segs[0].dur+segs[1].dur-2*xd)
	xargs := []string{"-i", segs[0].path, "-i", segs[1].path, "-i", segs[2].path,
		"-filter_complex", xf, "-map", "[vx]", "-r", fmt.Sprint(reupFPS), "-an"}
	xargs = append(xargs, encoderArgs()...)
	xargs = append(xargs, "-pix_fmt", "yuv420p", xfadePath)
	if err := ffmpegRun(ctx, xargs...); err != nil {
		os.Remove(xfadePath)
		return "", fmt.Errorf("reup remake: xfade: %w", err)
	}
	defer os.Remove(xfadePath)
	totalDur := segs[0].dur + segs[1].dur + segs[2].dur - 2*xd

	// 4. Voiceover & Lồng tiếng AI
	voiceText := ""
	if opts.Voiceover {
		voiceText = t.voiceoverText(ctx, []Video{v})
	}

	outPath := filepath.Join(t.WorkDir, fmt.Sprintf("reup_%s_l3.mp4", tag))
	if err := t.finalizeTransform(ctx, xfadePath, totalDur, voiceText, opts, outPath); err != nil {
		os.Remove(outPath)
		return "", err
	}
	if _, qerr := qcTransform(outPath); qerr != nil {
		os.Remove(outPath)
		return "", fmt.Errorf("reup: QC remake rớt: %w", qerr)
	}
	return outPath, nil
}
