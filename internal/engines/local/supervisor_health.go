package local

import (
	"context"
	"fmt"
	"net/http"
	"time"
)

func (p *Process) HealthStatus() (err error, checkedAt time.Time, fails int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.lastHealth, p.lastHealthAt, p.healthFails
}

func (p *Process) healthLoop() {
	t := time.NewTicker(p.healthInterval())
	defer t.Stop()
	for {
		select {
		case <-p.healthStop:
			return
		case <-t.C:
			p.checkHealth()
		}
	}
}

func (p *Process) checkHealth() {
	p.mu.Lock()
	running := p.cmd != nil && !p.stopping
	url := p.HealthURL
	p.mu.Unlock()
	if !running || url == "" {
		return
	}
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(url)
	var herr error
	switch {
	case err != nil:
		herr = err
	case resp.StatusCode != http.StatusOK:
		herr = fmt.Errorf("health %s: HTTP %d", url, resp.StatusCode)
	}
	if resp != nil {
		resp.Body.Close()
	}
	p.mu.Lock()
	p.lastHealth = herr
	p.lastHealthAt = time.Now()
	if herr != nil {
		p.healthFails++
	} else {
		p.healthFails = 0
	}
	p.mu.Unlock()
}

// WaitHealthy polls HealthURL until it returns 200, ctx is cancelled, or
// timeout elapses.
func (p *Process) WaitHealthy(ctx context.Context, timeout time.Duration) error {
	if p.HealthURL == "" {
		return fmt.Errorf("%s: no health URL configured", p.Name)
	}
	client := &http.Client{Timeout: 5 * time.Second}
	deadline := time.Now().Add(timeout)
	for {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.HealthURL, nil)
		if err != nil {
			return err
		}
		resp, err := client.Do(req)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
			err = fmt.Errorf("HTTP %d", resp.StatusCode)
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("%s: not healthy after %s: %v", p.Name, timeout, err)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}
}
