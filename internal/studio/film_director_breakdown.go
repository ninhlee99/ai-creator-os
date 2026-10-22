//go:build parked

package studio

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

func breakdownScreenplay(ctx context.Context, llm LLM, sp Screenplay, aspect string) (Breakdown, error) {
	var bd Breakdown
	raw, err := json.Marshal(sp)
	if err != nil {
		return bd, fmt.Errorf("breakdown: %w", err)
	}
	prompt, err := directorPrompt("breakdown.txt", promptData{
		Screenplay: string(raw),
		Orient:     orientationWord(aspect),
		N:          len(sp.Scenes),
	})
	if err != nil {
		return bd, err
	}
	text, err := llm.Complete(ctx,
		"Bạn là script supervisor + production designer điện ảnh. Chỉ trả lời JSON thuần, không giải thích.",
		prompt)
	if err != nil {
		return bd, fmt.Errorf("director pha 2: %w", err)
	}
	if err := parseDirectorJSON(text, &bd); err != nil {
		return bd, err
	}
	return bd, nil
}

// assembleScript ráp kịch bản pha 1 với breakdown pha 2 thành FilmScriptPro
// hoàn chỉnh. Quy tắc "kịch bản trước, breakdown sau":
//   - action / thoại / lời dẫn của pha 1 là CHUẨN (pha 2 không được viết lại);
//   - character bible + khóa bối cảnh + continuity + shot list lấy từ pha 2.
//
// Thiếu breakdown của bất kỳ cảnh nào, hoặc cảnh không có shot → lỗi
// (fail-closed: không quay cảnh không có shot).
func assembleScript(sp Screenplay, bd Breakdown) (FilmScriptPro, error) {
	var script FilmScriptPro
	if len(bd.Characters) == 0 {
		return script, fmt.Errorf("breakdown: thiếu character bible")
	}
	bdMap := make(map[int]BreakdownScene, len(bd.Scenes))
	for _, s := range bd.Scenes {
		bdMap[s.Index] = s
	}
	script.Title = sp.Title
	script.Logline = sp.Logline
	script.Genre = sp.Genre
	script.Characters = bd.Characters
	for _, sps := range sp.Scenes {
		bds, ok := bdMap[sps.Index]
		if !ok {
			return script, fmt.Errorf("breakdown: thiếu cảnh %d", sps.Index)
		}
		shots := sanitizeShots(bds.Shots)
		if len(shots) == 0 {
			return script, fmt.Errorf("breakdown: cảnh %d không có shot", sps.Index)
		}
		script.Scenes = append(script.Scenes, FilmScenePro{
			Index:       sps.Index,
			Act:         sps.Act,
			Location:    firstNonEmpty(bds.Location, sps.Location),
			TimeOfDay:   firstNonEmpty(bds.TimeOfDay, sps.TimeOfDay),
			Atmosphere:  firstNonEmpty(bds.Atmosphere, sps.Atmosphere),
			Seconds:     sps.Seconds,
			Action:      sps.Action,
			Shots:       shots,
			Dialogue:    sps.Dialogue,
			ImagePrompt: bds.ImagePrompt,
			Narration:   sps.Narration,
			Continuity:  bds.Continuity,
			Props:       bds.Props,
		})
	}
	return script, nil
}

// sanitizeShots loại shot rỗng prompt ảnh, kẹp thời lượng shot 2–10s.
func sanitizeShots(shots []ProShot) []ProShot {
	var out []ProShot
	for _, sh := range shots {
		if strings.TrimSpace(sh.ImagePrompt) == "" {
			continue
		}
		if sh.Seconds < 2 {
			sh.Seconds = 4
		} else if sh.Seconds > 10 {
			sh.Seconds = 8
		}
		out = append(out, sh)
	}
	return out
}

func firstNonEmpty(ss ...string) string {
	for _, s := range ss {
		if strings.TrimSpace(s) != "" {
			return s
		}
	}
	return ""
}

func countShots(script FilmScriptPro) int {
	n := 0
	for _, sc := range script.Scenes {
		n += len(sc.Shots)
	}
	return n
}

// CharacterRefs trả về ImageRef của các chân dung nhân vật đã sinh (để khóa
// identity cho mọi cảnh).
func (s FilmScriptPro) CharacterRefs() []ImageRef {
	var out []ImageRef
	for _, c := range s.Characters {
		if c.Portrait != "" {
			out = append(out, ImageRef{Path: c.Portrait})
		}
	}
	return out
}
