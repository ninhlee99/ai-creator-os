package web

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/ninhlee99/ai-creator-os/internal/harness"
	"log"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

func (s *Server) handleHarnessRunCycle(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	if s.Harness == nil {
		http.Redirect(w, r, "/harness?err="+url.QueryEscape("Harness agent chưa khởi tạo"), http.StatusSeeOther)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 45*time.Second)
	defer cancel()

	cycle, err := s.Harness.RunCycle(ctx)
	if err != nil {
		log.Printf("web: harness run cycle error: %v", err)
		http.Redirect(w, r, "/harness?err="+url.QueryEscape("Chu kỳ nhận thức gặp lỗi: "+err.Error()), http.StatusSeeOther)
		return
	}

	msg := fmt.Sprintf("Đã ghi chu kỳ %s: %d đề xuất, %d action đã thực thi.", cycle.CycleID, len(cycle.Decisions), cycle.ActionCount)
	http.Redirect(w, r, "/harness?notice="+url.QueryEscape(msg), http.StatusSeeOther)
}

// handleHarnessAutonomy cập nhật cấp độ tự trị.
func (s *Server) handleHarnessAutonomy(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	if s.Harness == nil {
		http.Redirect(w, r, "/harness?err="+url.QueryEscape("Harness agent chưa khởi tạo"), http.StatusSeeOther)
		return
	}

	levelStr := strings.TrimSpace(r.FormValue("level"))
	var lvl harness.AutonomyLevel
	switch levelStr {
	case "1", "assisted":
		lvl = harness.AutonomyAssisted
	case "2", "semi_autonomous":
		lvl = harness.AutonomySemiAutonomous
	case "3", "fully_autonomous":
		lvl = harness.AutonomyFullyAutonomous
	default:
		lvl = harness.AutonomyAssisted
	}

	if err := s.settings().Set("harness.autonomy_level", strconv.Itoa(int(lvl))); err != nil {
		http.Redirect(w, r, "/harness?err="+url.QueryEscape("Không lưu được mức tự chủ: "+err.Error()), http.StatusSeeOther)
		return
	}
	s.Harness.SetAutonomyLevel(lvl)
	if s.Ledger != nil {
		target := "autonomy_level"
		_ = s.Ledger.Decide("harness", "harness_autonomy", &target, fmt.Sprintf("cập nhật cấp độ tự hành: %d", lvl), nil)
	}

	http.Redirect(w, r, "/harness?notice="+url.QueryEscape("Đã cập nhật cấp độ tự chủ thành công!"), http.StatusSeeOther)
}

// handleHarnessMemoryAdd cho phép người quản trị huấn luyện hoặc nạp thêm kinh nghiệm cho Agent.
func (s *Server) handleHarnessMemoryAdd(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	if s.Harness == nil || s.Harness.Memory() == nil {
		http.Redirect(w, r, "/harness?err="+url.QueryEscape("Kho ký ức chưa khả dụng"), http.StatusSeeOther)
		return
	}

	domain := strings.TrimSpace(r.FormValue("domain"))
	channel := strings.TrimSpace(r.FormValue("channel"))
	niche := strings.TrimSpace(r.FormValue("niche"))
	subject := strings.TrimSpace(r.FormValue("subject"))
	insight := strings.TrimSpace(r.FormValue("insight"))
	confidenceStr := strings.TrimSpace(r.FormValue("confidence"))
	impactStr := strings.TrimSpace(r.FormValue("impact"))
	sampleStr := strings.TrimSpace(r.FormValue("sample_size"))
	sourceRef := strings.TrimSpace(r.FormValue("source_ref"))

	var memoryType harness.MemoryType
	switch harness.MemoryType(domain) {
	case harness.MemHookPerformance, harness.MemConversionInsight, harness.MemFormatReflection, harness.MemChannelLearning:
		memoryType = harness.MemoryType(domain)
	default:
		http.Redirect(w, r, "/harness?err="+url.QueryEscape("Lĩnh vực bài học không hợp lệ"), http.StatusSeeOther)
		return
	}
	if subject == "" || insight == "" {
		http.Redirect(w, r, "/harness?err="+url.QueryEscape("Vui lòng điền chủ đề và nội dung bài học"), http.StatusSeeOther)
		return
	}

	confidence, err := strconv.ParseFloat(confidenceStr, 64)
	if err != nil || math.IsNaN(confidence) || math.IsInf(confidence, 0) || confidence < 0 || confidence > 1 {
		http.Redirect(w, r, "/harness?err="+url.QueryEscape("Độ tin cậy tự đánh giá phải từ 0 đến 1"), http.StatusSeeOther)
		return
	}
	impact, err := strconv.ParseFloat(impactStr, 64)
	if err != nil || math.IsNaN(impact) || math.IsInf(impact, 0) || impact < -1 || impact > 1 {
		http.Redirect(w, r, "/harness?err="+url.QueryEscape("Tác động tự đánh giá phải từ -1 đến 1"), http.StatusSeeOther)
		return
	}
	sampleSize := 0
	if sampleStr != "" {
		sampleSize, err = strconv.Atoi(sampleStr)
		if err != nil || sampleSize < 0 || sampleSize > 1000000 {
			http.Redirect(w, r, "/harness?err="+url.QueryEscape("Số mẫu phải từ 0 đến 1.000.000"), http.StatusSeeOther)
			return
		}
	}

	item := harness.AgentMemory{
		MemoryType:      memoryType,
		ChannelID:       channel,
		Niche:           niche,
		Title:           subject,
		Insight:         insight,
		ConfidenceScore: confidence,
		ImpactScore:     impact,
		SampleSize:      sampleSize,
		SourceRef:       sourceRef,
		CreatedAt:       time.Now().Format("2006-01-02 15:04:05"),
		UpdatedAt:       time.Now().Format("2006-01-02 15:04:05"),
	}

	if err := s.Harness.Memory().RecordInsight(r.Context(), item); err != nil {
		http.Redirect(w, r, "/harness?err="+url.QueryEscape("Lỗi lưu ký ức: "+err.Error()), http.StatusSeeOther)
		return
	}

	if s.Ledger != nil {
		target := subject
		_ = s.Ledger.Decide("harness", "harness_memory", &target, fmt.Sprintf("ghi nhận bài học: [%s] %s", domain, subject), nil)
	}

	http.Redirect(w, r, "/harness?notice="+url.QueryEscape("Đã lưu bài học. Nó chỉ được đưa vào prompt ở luồng tương thích khi khớp kênh/ngách."), http.StatusSeeOther)
}

// handleHarnessMemoryDelete xóa một bài học khỏi kho ký ức.
func (s *Server) handleHarnessMemoryDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	if s.Harness == nil || s.Harness.Memory() == nil {
		http.Redirect(w, r, "/harness?err="+url.QueryEscape("Kho ký ức chưa khả dụng"), http.StatusSeeOther)
		return
	}

	id := strings.TrimSpace(r.FormValue("id"))
	if id == "" {
		http.Redirect(w, r, "/harness?err="+url.QueryEscape("Thiếu ID bài học"), http.StatusSeeOther)
		return
	}

	if err := s.Harness.Memory().Delete(r.Context(), id); err != nil {
		http.Redirect(w, r, "/harness?err="+url.QueryEscape("Lỗi xóa bài học: "+err.Error()), http.StatusSeeOther)
		return
	}

	if s.Ledger != nil {
		target := id
		_ = s.Ledger.Decide("harness", "harness_memory_delete", &target, "xóa bài học khỏi kho ký ức", nil)
	}

	http.Redirect(w, r, "/harness?notice="+url.QueryEscape("Đã xóa bài học khỏi kho ký ức thành công!"), http.StatusSeeOther)
}

// handleHarnessCycleDetailJSON trả về dữ liệu chi tiết dạng JSON của một chu kỳ OODA.
func (s *Server) handleHarnessCycleDetailJSON(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		http.Error(w, `{"error":"missing cycle id"}`, http.StatusBadRequest)
		return
	}

	if s.Harness == nil {
		http.Error(w, `{"error":"harness not initialized"}`, http.StatusServiceUnavailable)
		return
	}

	cycle, err := s.Harness.GetCycle(id)
	if err != nil {
		http.Error(w, `{"error":"cycle not found"}`, http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(cycle)
}

// handleHarnessPromptPreview mô phỏng sinh Prompt Context cho một kênh/niche.
func (s *Server) handleHarnessPromptPreview(w http.ResponseWriter, r *http.Request) {
	niche := r.URL.Query().Get("niche")
	channel := r.URL.Query().Get("channel")

	if s.Harness == nil || s.Harness.Memory() == nil {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"prompt_context": "",
			"count":          0,
		})
		return
	}

	promptCtx := s.Harness.Memory().GeneratePromptContext(r.Context(), niche, channel)
	mems, _ := s.Harness.Memory().QueryRelevantMemories(r.Context(), niche, channel, 5)

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"niche":          niche,
		"channel":        channel,
		"prompt_context": promptCtx,
		"count":          len(mems),
	})
}
