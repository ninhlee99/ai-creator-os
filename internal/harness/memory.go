package harness

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// MemoryType phân loại các kiểu ký ức và bài học kinh nghiệm của Agent.
type MemoryType string

const (
	MemHookPerformance   MemoryType = "hook_performance"   // Đánh giá hiệu quả giữ chân của Hook 3s
	MemConversionInsight MemoryType = "conversion_insight" // Kinh nghiệm chuyển đổi affiliate/đơn hàng
	MemFormatReflection  MemoryType = "format_reflection"  // Phản tỉnh về format nội dung (dài, ngắn, nhịp)
	MemChannelLearning   MemoryType = "channel_learning"   // Bài học đặc thù về tệp khán giả của từng kênh
)

// AgentMemory đại diện cho 1 đơn vị ký ức kinh nghiệm (Episodic/Semantic Memory).
type AgentMemory struct {
	ID              string     `json:"id"`
	MemoryType      MemoryType `json:"memory_type"`
	ChannelID       string     `json:"channel_id"`
	Niche           string     `json:"niche"`
	FormatID        string     `json:"format_id"`
	Title           string     `json:"title"`
	Insight         string     `json:"insight"`          // Bài học cốt lõi rút ra
	ConfidenceScore float64    `json:"confidence_score"` // Độ tin cậy (0.0 - 1.0)
	ImpactScore     float64    `json:"impact_score"`     // Mức độ ảnh hưởng: -1.0 (rất tệ) đến +1.0 (triệu view / chốt đơn cao)
	SampleSize      int        `json:"sample_size"`      // Số lượng mẫu dữ liệu quan sát được
	SourceRef       string     `json:"source_ref"`       // Video ID hoặc Campaign ID tham chiếu
	CreatedAt       string     `json:"created_at"`
	UpdatedAt       string     `json:"updated_at"`
}

// MemoryStore quản lý việc lưu trữ, truy xuất và tổng hợp kinh nghiệm của Agent Harness.
type MemoryStore struct {
	db *sql.DB
}

// NewMemoryStore khởi tạo kho ký ức thông minh của Agent.
func NewMemoryStore(db *sql.DB) (*MemoryStore, error) {
	ms := &MemoryStore{db: db}
	if err := ms.migrate(); err != nil {
		return nil, fmt.Errorf("harness memory migrate: %w", err)
	}
	return ms, nil
}

func (ms *MemoryStore) migrate() error {
	if ms.db == nil {
		return nil
	}
	_, err := ms.db.Exec(`
		CREATE TABLE IF NOT EXISTS harness_memories (
			id TEXT PRIMARY KEY,
			memory_type TEXT NOT NULL,
			channel_id TEXT NOT NULL DEFAULT '',
			niche TEXT NOT NULL DEFAULT '',
			format_id TEXT NOT NULL DEFAULT '',
			title TEXT NOT NULL,
			insight TEXT NOT NULL,
			confidence_score REAL NOT NULL DEFAULT 0.8,
			impact_score REAL NOT NULL DEFAULT 0.0,
			sample_size INTEGER NOT NULL DEFAULT 1,
			source_ref TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		);
		CREATE INDEX IF NOT EXISTS idx_harness_memories_niche ON harness_memories(niche, memory_type);
		CREATE INDEX IF NOT EXISTS idx_harness_memories_channel ON harness_memories(channel_id);
	`)
	return err
}

// RecordInsight ghi nhận một bài học kinh nghiệm mới vào bộ nhớ.
func (ms *MemoryStore) RecordInsight(ctx context.Context, mem AgentMemory) error {
	if ms.db == nil {
		return nil
	}
	now := time.Now().Format("2006-01-02 15:04:05")
	if mem.ID == "" {
		mem.ID = fmt.Sprintf("mem-%d", time.Now().UnixNano())
	}
	if mem.CreatedAt == "" {
		mem.CreatedAt = now
	}
	mem.UpdatedAt = now

	_, err := ms.db.ExecContext(ctx, `
		INSERT INTO harness_memories (
			id, memory_type, channel_id, niche, format_id, title,
			insight, confidence_score, impact_score, sample_size, source_ref,
			created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			insight = excluded.insight,
			confidence_score = excluded.confidence_score,
			impact_score = excluded.impact_score,
			sample_size = excluded.sample_size,
			updated_at = excluded.updated_at
	`, mem.ID, string(mem.MemoryType), mem.ChannelID, mem.Niche, mem.FormatID, mem.Title,
		mem.Insight, mem.ConfidenceScore, mem.ImpactScore, mem.SampleSize, mem.SourceRef,
		mem.CreatedAt, mem.UpdatedAt)

	return err
}

// QueryRelevantMemories tìm kiếm các ký ức liên quan nhất đến chủ đề hoặc kênh hiện tại.
func (ms *MemoryStore) QueryRelevantMemories(ctx context.Context, niche string, channelID string, limit int) ([]AgentMemory, error) {
	if ms.db == nil {
		return nil, nil
	}
	if limit <= 0 {
		limit = 10
	}

	query := `
		SELECT id, memory_type, channel_id, niche, format_id, title,
		       insight, confidence_score, impact_score, sample_size, source_ref,
		       created_at, updated_at
		FROM harness_memories
		WHERE (niche = ? OR niche = 'chung' OR ? = '' OR channel_id = ?)
		ORDER BY ABS(impact_score) DESC, updated_at DESC
		LIMIT ?
	`
	rows, err := ms.db.QueryContext(ctx, query, niche, niche, channelID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []AgentMemory
	for rows.Next() {
		var m AgentMemory
		var mType string
		if err := rows.Scan(
			&m.ID, &mType, &m.ChannelID, &m.Niche, &m.FormatID, &m.Title,
			&m.Insight, &m.ConfidenceScore, &m.ImpactScore, &m.SampleSize, &m.SourceRef,
			&m.CreatedAt, &m.UpdatedAt,
		); err != nil {
			continue
		}
		m.MemoryType = MemoryType(mType)
		out = append(out, m)
	}
	return out, nil
}

// ListAllMemories trả về danh sách tất cả ký ức thực chiến đã tích lũy.
func (ms *MemoryStore) ListAllMemories(ctx context.Context, limit int) ([]AgentMemory, error) {
	return ms.QueryRelevantMemories(ctx, "", "", limit)
}

// GeneratePromptContext tổng hợp các bài học thành ngữ cảnh ngắn gọn để tiêm vào prompt của LLM.
func (ms *MemoryStore) GeneratePromptContext(ctx context.Context, niche, channelID string) string {
	mems, err := ms.QueryRelevantMemories(ctx, niche, channelID, 5)
	if err != nil || len(mems) == 0 {
		return ""
	}

	var dos []string
	var donts []string

	for _, m := range mems {
		if m.ImpactScore > 0.2 {
			dos = append(dos, fmt.Sprintf("- [%s] %s (Bài học: %s)", m.Title, m.Insight, m.FormatID))
		} else if m.ImpactScore < -0.2 {
			donts = append(donts, fmt.Sprintf("- [TRÁNH] %s (Lý do: %s)", m.Title, m.Insight))
		}
	}

	var sb strings.Builder
	sb.WriteString("\n### BÀI HỌC THỰC CHIẾN TỪ CÁC VIDEO TRƯỚC (AGENT EPISODIC MEMORY):\n")
	if len(dos) > 0 {
		sb.WriteString("Phát huy các yếu tố đã chứng minh thành công:\n")
		sb.WriteString(strings.Join(dos, "\n"))
		sb.WriteString("\n")
	}
	if len(donts) > 0 {
		sb.WriteString("Tuyệt đối tránh các lỗi đã gây giảm tương tác:\n")
		sb.WriteString(strings.Join(donts, "\n"))
		sb.WriteString("\n")
	}
	return sb.String()
}
