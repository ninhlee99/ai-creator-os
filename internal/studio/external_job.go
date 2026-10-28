package studio

import (
	"context"
	"fmt"
	"strings"
)

// ExternalJobRunner performs one provider-backed operation and returns an
// output path relative to Studio's media directory plus optional caption.
type ExternalJobRunner func(context.Context, func(int, string)) (string, string, error)

// CreateExternalJob stores and runs a lightweight provider job through the
// same queue, cancellation, progress, and SQLite lifecycle as render jobs.
func (s *Studio) CreateExternalJob(kind, title string, params any, run ExternalJobRunner) (string, error) {
	if run == nil {
		return "", fmt.Errorf("external job runner is required")
	}
	id, err := s.insertJob(kind, title, params)
	if err != nil {
		return "", err
	}
	ctx, cancel := context.WithCancel(context.Background())
	if err := s.startTrackedJob(id, ctx, cancel, func(ctx context.Context) {
		defer func() {
			if recovered := recover(); recovered != nil {
				s.appendLog(id, fmt.Sprintf("LỖI: provider job panic (%v)", recovered))
				s.setStatus(id, StatusFailed, 100)
			}
		}()

		sem, slotErr := s.renderSlot(ctx)
		if slotErr != nil {
			if ctx.Err() != nil {
				s.appendLog(id, "Đã hủy trước khi bắt đầu.")
			} else {
				s.appendLog(id, "LỖI: "+slotErr.Error())
			}
			s.setStatus(id, StatusFailed, 100)
			return
		}
		defer func() { <-sem }()
		s.setStatus(id, StatusRunning, 1)
		progress := func(percent int, line string) {
			if percent < 1 {
				percent = 1
			}
			if percent > 99 {
				percent = 99
			}
			s.setStatus(id, StatusRunning, percent)
			if strings.TrimSpace(line) != "" {
				s.appendLog(id, line)
			}
		}
		output, caption, runErr := run(ctx, progress)
		if runErr != nil {
			if ctx.Err() != nil {
				s.appendLog(id, "Đã hủy theo yêu cầu.")
			} else {
				s.appendLog(id, "LỖI: "+runErr.Error())
			}
			s.setStatus(id, StatusFailed, 100)
			return
		}
		if ctx.Err() != nil {
			s.appendLog(id, "Đã hủy theo yêu cầu.")
			s.setStatus(id, StatusFailed, 100)
			return
		}
		if output != "" {
			s.setOutput(id, output)
		}
		if caption != "" {
			s.setCaption(id, caption)
		}
		s.appendLog(id, "Hoàn tất.")
		s.setStatus(id, StatusDone, 100)
	}); err != nil {
		s.appendLog(id, "LỖI: "+err.Error())
		s.setStatus(id, StatusFailed, 100)
		return "", err
	}
	return id, nil
}
