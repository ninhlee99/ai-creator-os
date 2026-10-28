package studio

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// DefaultTikTokTopics chứa các chủ đề mặc định thịnh hành tại Việt Nam.
var DefaultTikTokTopics = []string{
	"Xu hướng Việt Nam",
	"Nhảy trend",
	"Thời trang",
	"Làm đẹp",
	"Review ẩm thực",
}

// ListTikTokRadarTopics trả về danh sách các chủ đề TikTok đang theo dõi.
func (s *Studio) ListTikTokRadarTopics(ctx context.Context) ([]string, error) {
	if s == nil || s.db == nil {
		return DefaultTikTokTopics, nil
	}
	rows, err := s.db.QueryContext(ctx, `SELECT name FROM studio_tiktok_radar_topics ORDER BY id ASC`)
	if err != nil {
		return DefaultTikTokTopics, nil
	}
	defer rows.Close()

	var topics []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err == nil && strings.TrimSpace(name) != "" {
			topics = append(topics, strings.TrimSpace(name))
		}
	}
	if len(topics) == 0 {
		return DefaultTikTokTopics, nil
	}
	return topics, nil
}

// AddTikTokRadarTopic thêm chủ đề; web handler xếp lịch lấy và phân tích Top 8 nền.
func (s *Studio) AddTikTokRadarTopic(ctx context.Context, topic string) error {
	topic = strings.TrimSpace(topic)
	if topic == "" {
		return fmt.Errorf("chủ đề không được để trống")
	}
	if len([]rune(topic)) > 100 {
		return fmt.Errorf("tên chủ đề tối đa 100 ký tự")
	}
	if s == nil || s.db == nil {
		return fmt.Errorf("Studio chưa sẵn sàng")
	}

	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.db.ExecContext(ctx, `INSERT INTO studio_tiktok_radar_topics(name, created_at) VALUES(?, ?) ON CONFLICT(name) DO NOTHING`, topic, now)
	if err != nil {
		return fmt.Errorf("không thêm được chủ đề: %w", err)
	}

	return nil
}

// DeleteTikTokRadarTopic xóa chủ đề tùy chỉnh; tab mặc định được bảo vệ.
func (s *Studio) DeleteTikTokRadarTopic(ctx context.Context, topic string) error {
	topic = strings.TrimSpace(topic)
	if topic == "" {
		return nil
	}
	if IsTikTokRadarProtectedDefaultTopic(topic) {
		return fmt.Errorf("không thể xoá chủ đề mặc định %q", topic)
	}
	if s == nil || s.db == nil {
		return fmt.Errorf("Studio chưa sẵn sàng")
	}
	s.mu.Lock()
	if s.radarTopicRefreshStates == nil {
		s.radarTopicRefreshStates = make(map[string]radarTopicRefreshState)
	}
	state := s.radarTopicRefreshStates[topic]
	if state.deleting {
		s.mu.Unlock()
		return fmt.Errorf("chủ đề đang được xoá")
	}
	state.deleting = true
	s.radarTopicRefreshStates[topic] = state
	cancel := state.cancel
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		state := s.radarTopicRefreshStates[topic]
		state.deleting = false
		s.radarTopicRefreshStates[topic] = state
		s.mu.Unlock()
	}()
	if cancel != nil {
		cancel()
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		s.mu.Lock()
		state := s.radarTopicRefreshStates[topic]
		running, cancel := state.running, state.cancel
		s.mu.Unlock()
		if !running {
			break
		}
		if cancel != nil {
			cancel()
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("không dừng được job Radar; thử xoá chủ đề lại sau")
		}
		time.Sleep(25 * time.Millisecond)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	rows, err := tx.QueryContext(ctx, `SELECT id FROM studio_jobs WHERE kind=? AND status IN (?,?)
		AND COALESCE(CASE WHEN json_valid(params) THEN json_extract(params,'$.topic') END,'')=?`,
		KindTikTokRadarRefresh, StatusQueued, StatusRunning, topic)
	if err != nil {
		return err
	}
	var activeJobIDs []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		activeJobIDs = append(activeJobIDs, id)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	finishedAt := time.Now().Format("2006-01-02T15:04:05")
	for _, id := range activeJobIDs {
		if _, err := tx.ExecContext(ctx, `UPDATE studio_jobs SET status=?, progress=100, finished_at=? WHERE id=?`, StatusFailed, finishedAt, id); err != nil {
			return err
		}
	}

	if _, err := tx.ExecContext(ctx, `DELETE FROM studio_tiktok_radar_topics WHERE name=?`, topic); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM studio_tiktok_radar_topic_videos WHERE topic=?`, topic); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	for _, id := range activeJobIDs {
		s.appendLog(id, "Chủ đề đã bị xoá; job Radar bị dừng.")
	}
	return nil
}

// GetTikTokRadarVideosForTopic trả danh sách đã xếp hạng trong DB; web handler
// tự xếp lịch làm mới khi danh sách trống hoặc thiếu TOP 8.
func (s *Studio) GetTikTokRadarVideosForTopic(ctx context.Context, topic string) ([]TikTokTrendVideo, error) {
	topic = strings.TrimSpace(topic)
	if topic == "" {
		topic = "Xu hướng Việt Nam"
	}
	if s == nil || s.db == nil {
		return s.CollectTikTokRadarVideosForTopic(ctx, topic)
	}

	type videoRow struct {
		payload         string
		analysisPayload string
		danceVerified   bool
		danceVerifiedAt string
	}
	var rowsData []videoRow

	rows, err := s.db.QueryContext(ctx, `
		SELECT v.payload, tv.analysis_payload, v.dance_verified, v.dance_verified_at
		FROM studio_tiktok_radar_topic_videos tv
		JOIN studio_tiktok_radar_videos v ON tv.video_id = v.video_id
		WHERE tv.topic=? AND tv.rank BETWEEN 1 AND ?
		ORDER BY tv.rank ASC
		LIMIT ?
	`, topic, tikTokRadarFeedLimit, tikTokRadarFeedLimit)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var r videoRow
		var danceVerifiedInt int
		if err := rows.Scan(&r.payload, &r.analysisPayload, &danceVerifiedInt, &r.danceVerifiedAt); err != nil {
			rows.Close()
			return nil, err
		}
		r.danceVerified = danceVerifiedInt != 0
		rowsData = append(rowsData, r)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()

	if len(rowsData) > 0 {
		var videos []TikTokTrendVideo
		for _, r := range rowsData {
			var v TikTokTrendVideo
			if json.Unmarshal([]byte(r.payload), &v) == nil {
				if strings.TrimSpace(r.analysisPayload) != "" {
					var analysis TikTokRadarContentAnalysis
					if json.Unmarshal([]byte(r.analysisPayload), &analysis) == nil {
						v.Analysis = &analysis
					}
				}
				if v.Analysis == nil || !v.Analysis.qualifies(topic, v) {
					continue
				}
				v.DanceVerified = r.danceVerified
				v.DanceVerifiedAt = r.danceVerifiedAt
				if err := s.enrichTikTokRadarMetrics(ctx, &v); err != nil {
					return nil, fmt.Errorf("không đọc được chỉ số Radar TikTok: %w", err)
				}
				v.Rank = len(videos) + 1
				videos = append(videos, v)
			}
		}
		if len(videos) > 0 {
			return videos, nil
		}
		return []TikTokTrendVideo{}, nil
	}

	// Empty topic reads stay fast; the web handler queues a background fetch.
	return []TikTokTrendVideo{}, nil
}

// CollectTikTokRadarVideosForTopic quét trực tiếp từ TikTok, lưu và xếp hạng tối đa 8 video cho chủ đề.
func (s *Studio) CollectTikTokRadarVideosForTopic(ctx context.Context, topic string) (result []TikTokTrendVideo, returnErr error) {
	topic = strings.TrimSpace(topic)
	if topic == "" {
		topic = "Xu hướng Việt Nam"
	}
	if s == nil || s.db == nil {
		return nil, fmt.Errorf("Studio chưa sẵn sàng để lưu video Radar TikTok")
	}
	s.noteTikTokRadarTopicRefresh(topic)
	defer func() { s.setTikTokRadarTopicRefreshError(topic, returnErr) }()
	candidates, err := s.SearchTikTokRadarVideos(ctx, topic)
	if err != nil {
		return nil, err
	}
	videos, analyzed, analysisErr := s.curateTikTokRadarCandidates(ctx, topic, candidates)
	if err := s.saveTopicVideos(ctx, topic, analyzed); err != nil {
		return videos, fmt.Errorf("lưu danh sách video đã phân tích cho chủ đề: %w", err)
	}
	sortTikTokRadarVideosByPopularity(videos)
	for i := range videos {
		videos[i].Rank = i + 1
	}

	if analysisErr != nil {
		return videos, analysisErr
	}
	return videos, nil
}

func (s *Studio) saveTopicVideos(ctx context.Context, topic string, videos []TikTokTrendVideo) error {
	if s == nil || s.db == nil {
		return fmt.Errorf("Studio chưa sẵn sàng để lưu chủ đề Radar TikTok")
	}
	qualified := make([]TikTokTrendVideo, 0, tikTokRadarFeedLimit)
	remaining := make([]TikTokTrendVideo, 0, len(videos))
	seen := make(map[string]struct{}, len(videos))
	for _, video := range videos {
		video.VideoID = strings.TrimSpace(video.VideoID)
		if video.VideoID == "" {
			continue
		}
		if _, duplicate := seen[video.VideoID]; duplicate {
			continue
		}
		seen[video.VideoID] = struct{}{}
		if video.Analysis.qualifies(topic, video) {
			qualified = append(qualified, video)
		} else if video.Analysis.confidentlyClassifiesNonDance(topic, video) {
			// Remove only this topic mapping. Keep the global video record and
			// mappings for other topics intact.
		} else {
			remaining = append(remaining, video)
		}
	}
	sortTikTokRadarVideosByPopularity(qualified)
	sortTikTokRadarVideosByPopularity(remaining)
	ordered := append(qualified, remaining...)
	if len(ordered) > tikTokRadarCandidateLimit {
		ordered = ordered[:tikTokRadarCandidateLimit]
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if !IsTikTokRadarProtectedDefaultTopic(topic) {
		var topicExists int
		if err := tx.QueryRowContext(ctx, `SELECT 1 FROM studio_tiktok_radar_topics WHERE name=?`, topic).Scan(&topicExists); err != nil {
			if err == sql.ErrNoRows {
				return fmt.Errorf("chủ đề %q đã bị xoá trước khi lưu kết quả Radar", topic)
			}
			return err
		}
	}

	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := tx.ExecContext(ctx, `UPDATE studio_tiktok_radar_topic_videos SET rank=0, updated_at=? WHERE topic=?`, now, topic); err != nil {
		return err
	}
	if TikTokRadarTopicRequiresDance(topic) {
		for _, video := range videos {
			if !video.Analysis.confidentlyClassifiesNonDance(topic, video) {
				continue
			}
			if _, err := tx.ExecContext(ctx, `DELETE FROM studio_tiktok_radar_topic_videos WHERE topic=? AND video_id=?`, topic, strings.TrimSpace(video.VideoID)); err != nil {
				return err
			}
		}
	}
	stmt, err := tx.PrepareContext(ctx, `INSERT INTO studio_tiktok_radar_topic_videos(topic, video_id, rank, analysis_payload, updated_at) VALUES(?, ?, ?, ?, ?)
		ON CONFLICT(topic,video_id) DO UPDATE SET rank=excluded.rank,
		analysis_payload=CASE WHEN excluded.analysis_payload<>'' THEN excluded.analysis_payload ELSE studio_tiktok_radar_topic_videos.analysis_payload END,
		updated_at=excluded.updated_at`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	nextPosition := 0
	for _, video := range ordered {
		position := 0
		if video.Analysis.qualifies(topic, video) && nextPosition < tikTokRadarFeedLimit {
			nextPosition++
			position = nextPosition
		}
		analysisPayload := ""
		if video.Analysis != nil {
			payload, err := json.Marshal(video.Analysis)
			if err != nil {
				return err
			}
			analysisPayload = string(payload)
		}
		if _, err := stmt.ExecContext(ctx, topic, video.VideoID, position, analysisPayload, now); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Studio) loadTikTokRadarTopicAnalyses(ctx context.Context, topic string) (map[string]*TikTokRadarContentAnalysis, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT video_id, analysis_payload FROM studio_tiktok_radar_topic_videos WHERE topic=?`, topic)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	analyses := make(map[string]*TikTokRadarContentAnalysis)
	for rows.Next() {
		var videoID, payload string
		if err := rows.Scan(&videoID, &payload); err != nil {
			return nil, err
		}
		var analysis TikTokRadarContentAnalysis
		if strings.TrimSpace(payload) == "" || json.Unmarshal([]byte(payload), &analysis) != nil {
			continue
		}
		analyses[videoID] = &analysis
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return analyses, nil
}

type radarTopicRefreshState struct {
	running     bool
	deleting    bool
	lastAttempt time.Time
	lastError   string
	jobID       string
	cancel      context.CancelFunc
}

type radarTopicRefreshJobParams struct {
	Topic string `json:"topic"`
}

const radarTopicRefreshCooldown = 15 * time.Minute

// QueueTikTokRadarTopicRefresh starts or resumes durable crawler, AI analysis,
// and ranking work without holding the page request open. Only one headless
// crawler runs at a time. Returns true while queued or running so UI can poll.
func (s *Studio) QueueTikTokRadarTopicRefresh(topic string, currentCount int) bool {
	return s.queueTikTokRadarTopicRefresh(topic, currentCount, false)
}

func (s *Studio) QueueTikTokRadarTopicRefreshNow(topic string) bool {
	return s.queueTikTokRadarTopicRefresh(topic, 0, true)
}

func (s *Studio) queueTikTokRadarTopicRefresh(topic string, currentCount int, force bool) bool {
	topic = strings.TrimSpace(topic)
	if s == nil || s.db == nil || topic == "" || !s.tikTokRadarTopicAvailable(topic) || !TikTokCrawlerConfigured() {
		return false
	}
	stale := currentCount < tikTokRadarFeedLimit
	if !stale && !force {
		var updatedAt string
		if err := s.db.QueryRow(`SELECT COALESCE(MAX(updated_at),'') FROM studio_tiktok_radar_topic_videos WHERE topic=?`, topic).Scan(&updatedAt); err != nil || updatedAt == "" {
			stale = true
		} else if parsed, err := time.Parse(time.RFC3339, updatedAt); err != nil || time.Since(parsed) >= radarTopicRefreshCooldown {
			stale = true
		}
	}
	s.mu.Lock()
	if s.radarTopicRefreshStates == nil {
		s.radarTopicRefreshStates = make(map[string]radarTopicRefreshState)
	}
	state := s.radarTopicRefreshStates[topic]
	now := time.Now()
	if state.deleting {
		s.mu.Unlock()
		return false
	}
	if state.running {
		s.mu.Unlock()
		return true
	}

	jobID, jobStatus, err := s.findActiveTikTokRadarRefreshJob(topic)
	if err != nil {
		state.lastError = truncateRadarText(err.Error(), 500)
		s.radarTopicRefreshStates[topic] = state
		s.mu.Unlock()
		return false
	}
	if jobID == "" && !force && !state.lastAttempt.IsZero() && now.Sub(state.lastAttempt) < radarTopicRefreshCooldown {
		s.mu.Unlock()
		return false
	}
	var recovered bool
	if jobID != "" {
		if jobStatus == StatusRunning {
			s.setStatus(jobID, StatusQueued, 0)
			recovered = true
		}
	} else {
		if !force && !stale {
			s.mu.Unlock()
			return false
		}
		if !force {
			lastCreated, found, err := s.latestTikTokRadarRefreshJob(topic)
			if err != nil {
				state.lastError = truncateRadarText(err.Error(), 500)
				s.radarTopicRefreshStates[topic] = state
				s.mu.Unlock()
				return false
			}
			if found && now.Sub(lastCreated) < radarTopicRefreshCooldown {
				state.lastAttempt = lastCreated
				s.radarTopicRefreshStates[topic] = state
				s.mu.Unlock()
				return false
			}
		}
		jobID, err = s.insertJob(KindTikTokRadarRefresh, "Radar TikTok · "+topic, radarTopicRefreshJobParams{Topic: topic})
		if err != nil {
			state.lastError = truncateRadarText(err.Error(), 500)
			s.radarTopicRefreshStates[topic] = state
			s.mu.Unlock()
			return false
		}
	}
	state.running = true
	state.lastAttempt = now
	state.lastError = ""
	state.jobID = jobID
	s.radarTopicRefreshStates[topic] = state
	s.mu.Unlock()
	if recovered {
		s.appendLog(jobID, "Khôi phục job Radar sau khi tiến trình trước bị ngắt.")
	}
	s.runTikTokRadarRefreshJob(topic, jobID)
	return true
}

func (s *Studio) tikTokRadarTopicAvailable(topic string) bool {
	if s == nil || s.db == nil || strings.TrimSpace(topic) == "" {
		return false
	}
	if IsTikTokRadarProtectedDefaultTopic(topic) {
		return true
	}
	var exists int
	return s.db.QueryRow(`SELECT 1 FROM studio_tiktok_radar_topics WHERE name=?`, strings.TrimSpace(topic)).Scan(&exists) == nil
}

func (s *Studio) findActiveTikTokRadarRefreshJob(topic string) (jobID, status string, err error) {
	err = s.db.QueryRow(`SELECT id,status FROM studio_jobs
		WHERE kind=? AND status IN (?,?)
		AND COALESCE(CASE WHEN json_valid(params) THEN json_extract(params,'$.topic') END,'')=?
		ORDER BY created_at DESC LIMIT 1`, KindTikTokRadarRefresh, StatusQueued, StatusRunning, topic).Scan(&jobID, &status)
	if err == sql.ErrNoRows {
		return "", "", nil
	}
	return jobID, status, err
}

func (s *Studio) latestTikTokRadarRefreshJob(topic string) (time.Time, bool, error) {
	var createdAt string
	err := s.db.QueryRow(`SELECT created_at FROM studio_jobs WHERE kind=?
		AND COALESCE(CASE WHEN json_valid(params) THEN json_extract(params,'$.topic') END,'')=?
		ORDER BY created_at DESC LIMIT 1`, KindTikTokRadarRefresh, topic).Scan(&createdAt)
	if err == sql.ErrNoRows {
		return time.Time{}, false, nil
	}
	if err != nil {
		return time.Time{}, false, err
	}
	created, err := time.ParseInLocation("2006-01-02T15:04:05", createdAt, time.Local)
	if err != nil {
		return time.Time{}, false, err
	}
	return created, true, nil
}

func (s *Studio) runTikTokRadarRefreshJob(topic, jobID string) {
	ctx, cancel := context.WithTimeout(context.Background(), tikTokRadarTopicRefreshTimeout)
	s.mu.Lock()
	if s.shuttingDown {
		state := s.radarTopicRefreshStates[topic]
		if state.jobID == jobID {
			state.running = false
			state.cancel = nil
			s.radarTopicRefreshStates[topic] = state
		}
		s.mu.Unlock()
		cancel()
		return
	}
	if s.running == nil {
		s.running = make(map[string]context.CancelFunc)
	}
	if _, exists := s.running[jobID]; exists {
		s.mu.Unlock()
		cancel()
		return
	}
	s.running[jobID] = cancel
	if s.radarTopicRefreshSem == nil {
		s.radarTopicRefreshSem = make(chan struct{}, 1)
	}
	sem := s.radarTopicRefreshSem
	state := s.radarTopicRefreshStates[topic]
	if state.jobID == jobID {
		state.cancel = cancel
		s.radarTopicRefreshStates[topic] = state
	}
	s.mu.Unlock()
	go func() {
		defer func() {
			s.mu.Lock()
			delete(s.running, jobID)
			s.mu.Unlock()
			cancel()
		}()
		s.runTikTokRadarRefreshJobWithSemaphore(topic, jobID, sem, ctx, cancel)
	}()
}

func (s *Studio) runTikTokRadarRefreshJobWithSemaphore(topic, jobID string, sem chan struct{}, ctx context.Context, cancel context.CancelFunc) {
	defer func() {
		s.mu.Lock()
		state := s.radarTopicRefreshStates[topic]
		if state.jobID == jobID {
			state.running = false
			state.jobID = ""
			state.cancel = nil
			s.radarTopicRefreshStates[topic] = state
		}
		s.mu.Unlock()
	}()
	defer func() {
		if recovered := recover(); recovered != nil {
			err := fmt.Errorf("panic trong job Radar: %v", recovered)
			s.setTikTokRadarTopicRefreshError(topic, err)
			s.appendLog(jobID, "LỖI: "+truncateRadarText(err.Error(), 500))
			s.setStatus(jobID, StatusFailed, 100)
		}
	}()
	s.mu.Lock()
	state := s.radarTopicRefreshStates[topic]
	canRun := state.jobID == jobID && !state.deleting
	if state.jobID == jobID {
		state.cancel = cancel
		s.radarTopicRefreshStates[topic] = state
	}
	s.mu.Unlock()
	if !canRun {
		cancel()
		err := context.Canceled
		s.setTikTokRadarTopicRefreshError(topic, err)
		s.appendLog(jobID, "LỖI: "+truncateRadarText(err.Error(), 500))
		s.setStatus(jobID, StatusFailed, 100)
		return
	}
	select {
	case sem <- struct{}{}:
	case <-ctx.Done():
		err := ctx.Err()
		s.setTikTokRadarTopicRefreshError(topic, err)
		s.appendLog(jobID, "LỖI: "+truncateRadarText(err.Error(), 500))
		s.setStatus(jobID, StatusFailed, 100)
		return
	}
	defer func() { <-sem }()
	if err := ctx.Err(); err != nil {
		s.setTikTokRadarTopicRefreshError(topic, err)
		s.appendLog(jobID, "LỖI: "+truncateRadarText(err.Error(), 500))
		s.setStatus(jobID, StatusFailed, 100)
		return
	}
	var status string
	if err := s.db.QueryRow(`SELECT status FROM studio_jobs WHERE id=?`, jobID).Scan(&status); err != nil || (status != StatusQueued && status != StatusRunning) {
		return
	}
	s.setStatus(jobID, StatusRunning, 5)
	s.appendLog(jobID, "Bắt đầu quét TikTok, phân tích WebAI và xếp hạng TOP 8.")
	videos, err := s.CollectTikTokRadarVideosForTopic(ctx, topic)
	if len(videos) < tikTokRadarFeedLimit {
		shortfall := fmt.Errorf("chỉ tìm được %d/%d video phù hợp sau khi phân tích", len(videos), tikTokRadarFeedLimit)
		if err == nil {
			err = shortfall
		} else {
			err = errors.Join(err, shortfall)
		}
	}
	if err != nil {
		s.setTikTokRadarTopicRefreshError(topic, err)
		s.appendLog(jobID, "LỖI: "+truncateRadarText(err.Error(), 500))
		s.setStatus(jobID, StatusFailed, 100)
		return
	}
	s.appendLog(jobID, fmt.Sprintf("Đã cập nhật đủ TOP %d video.", tikTokRadarFeedLimit))
	s.setStatus(jobID, StatusDone, 100)
}

// ResumeTikTokRadarRefreshJobs redispatches durable refresh jobs after WebAI
// and crawler dependencies have been initialized at app startup.
func (s *Studio) ResumeTikTokRadarRefreshJobs() int {
	if s == nil || s.db == nil || !TikTokCrawlerConfigured() {
		return 0
	}
	rows, err := s.db.Query(`SELECT id,params,status FROM studio_jobs
		WHERE kind=? AND status IN (?,?) ORDER BY created_at ASC`, KindTikTokRadarRefresh, StatusQueued, StatusRunning)
	if err != nil {
		return 0
	}
	type pendingJob struct{ id, params, status string }
	var pending []pendingJob
	for rows.Next() {
		var job pendingJob
		if rows.Scan(&job.id, &job.params, &job.status) == nil {
			pending = append(pending, job)
		}
	}
	rows.Close()
	resumed := 0
	for _, job := range pending {
		var params radarTopicRefreshJobParams
		if json.Unmarshal([]byte(job.params), &params) != nil || strings.TrimSpace(params.Topic) == "" {
			s.appendLog(job.id, "LỖI: không đọc được topic để khôi phục job Radar.")
			s.setStatus(job.id, StatusFailed, 100)
			continue
		}
		if job.status == StatusRunning {
			s.setStatus(job.id, StatusQueued, 0)
			s.appendLog(job.id, "Khôi phục job Radar sau khi tiến trình trước bị ngắt.")
		}
		if s.queueTikTokRadarTopicRefreshJob(params.Topic, job.id) {
			resumed++
		}
	}
	return resumed
}

func (s *Studio) queueTikTokRadarTopicRefreshJob(topic, jobID string) bool {
	topic = strings.TrimSpace(topic)
	if s == nil || s.db == nil || topic == "" || jobID == "" || !s.tikTokRadarTopicAvailable(topic) || !TikTokCrawlerConfigured() {
		return false
	}
	s.mu.Lock()
	if s.radarTopicRefreshStates == nil {
		s.radarTopicRefreshStates = make(map[string]radarTopicRefreshState)
	}
	state := s.radarTopicRefreshStates[topic]
	if state.deleting || state.running {
		s.mu.Unlock()
		return false
	}
	state.running = true
	state.lastAttempt = time.Now()
	state.lastError = ""
	state.jobID = jobID
	s.radarTopicRefreshStates[topic] = state
	s.mu.Unlock()
	s.runTikTokRadarRefreshJob(topic, jobID)
	return true
}

func (s *Studio) noteTikTokRadarTopicRefresh(topic string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.radarTopicRefreshStates == nil {
		s.radarTopicRefreshStates = make(map[string]radarTopicRefreshState)
	}
	state := s.radarTopicRefreshStates[topic]
	state.lastAttempt = time.Now()
	s.radarTopicRefreshStates[topic] = state
}

func (s *Studio) setTikTokRadarTopicRefreshError(topic string, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.radarTopicRefreshStates == nil {
		s.radarTopicRefreshStates = make(map[string]radarTopicRefreshState)
	}
	state := s.radarTopicRefreshStates[topic]
	state.lastError = ""
	if err != nil {
		state.lastError = truncateRadarText(err.Error(), 500)
	}
	s.radarTopicRefreshStates[topic] = state
}

func (s *Studio) TikTokRadarTopicRefreshError(topic string) string {
	if s == nil {
		return ""
	}
	s.mu.Lock()
	message := s.radarTopicRefreshStates[strings.TrimSpace(topic)].lastError
	s.mu.Unlock()
	if message != "" || s.db == nil {
		return message
	}
	var jobLog string
	err := s.db.QueryRow(`SELECT log FROM studio_jobs WHERE kind=? AND status=?
		AND COALESCE(CASE WHEN json_valid(params) THEN json_extract(params,'$.topic') END,'')=?
		ORDER BY created_at DESC LIMIT 1`, KindTikTokRadarRefresh, StatusFailed, strings.TrimSpace(topic)).Scan(&jobLog)
	if err != nil {
		return ""
	}
	lines := strings.Split(strings.TrimSpace(jobLog), "\n")
	if len(lines) == 0 {
		return ""
	}
	return truncateRadarText(lines[len(lines)-1], 500)
}
