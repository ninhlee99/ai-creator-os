package webai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
)

type cdpRawMessage struct {
	ID     int64           `json:"id"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params"`
	Result json.RawMessage `json:"result"`
	Error  json.RawMessage `json:"error"`
}

type browserCDPClient struct {
	conn      *websocket.Conn
	sessionID string
	nextID    atomic.Int64
	writeMu   sync.Mutex

	readerOnce sync.Once
	pendingMu  sync.Mutex
	pending    map[int64]chan cdpRawMessage
	closed     chan struct{}
	closeErr   error

	subMu       sync.RWMutex
	nextSubID   int64
	subscribers map[string]map[int64]func(json.RawMessage)
}

func (c *browserCDPClient) ensureReader() {
	c.readerOnce.Do(func() {
		c.pending = make(map[int64]chan cdpRawMessage)
		c.closed = make(chan struct{})
		c.subscribers = make(map[string]map[int64]func(json.RawMessage))
		go c.readLoop()
	})
}

func (c *browserCDPClient) readLoop() {
	defer func() {
		c.pendingMu.Lock()
		if c.closeErr == nil {
			c.closeErr = errors.New("CDP websocket closed")
		}
		close(c.closed)
		for id, ch := range c.pending {
			close(ch)
			delete(c.pending, id)
		}
		c.pendingMu.Unlock()
	}()
	for {
		_ = c.conn.SetReadDeadline(time.Now().Add(90 * time.Second))
		_, data, err := c.conn.ReadMessage()
		if err != nil {
			c.pendingMu.Lock()
			c.closeErr = fmt.Errorf("Muse headless CDP connection failed: %w", err)
			c.pendingMu.Unlock()
			return
		}
		var msg cdpRawMessage
		if json.Unmarshal(data, &msg) != nil {
			continue
		}
		if msg.ID != 0 {
			c.pendingMu.Lock()
			ch, ok := c.pending[msg.ID]
			if ok {
				delete(c.pending, msg.ID)
			}
			c.pendingMu.Unlock()
			if ok {
				ch <- msg
				close(ch)
			}
			continue
		}
		if msg.Method != "" {
			c.dispatchEvent(msg.Method, msg.Params)
		}
	}
}

func (c *browserCDPClient) subscribe(method string, handler func(json.RawMessage)) func() {
	c.ensureReader()
	c.subMu.Lock()
	c.nextSubID++
	subID := c.nextSubID
	if c.subscribers[method] == nil {
		c.subscribers[method] = make(map[int64]func(json.RawMessage))
	}
	c.subscribers[method][subID] = handler
	c.subMu.Unlock()
	return func() {
		c.subMu.Lock()
		if m := c.subscribers[method]; m != nil {
			delete(m, subID)
		}
		c.subMu.Unlock()
	}
}

func (c *browserCDPClient) dispatchEvent(method string, params json.RawMessage) {
	c.subMu.RLock()
	handlersMap := c.subscribers[method]
	if len(handlersMap) == 0 {
		c.subMu.RUnlock()
		return
	}
	handlers := make([]func(json.RawMessage), 0, len(handlersMap))
	for _, fn := range handlersMap {
		handlers = append(handlers, fn)
	}
	c.subMu.RUnlock()
	for _, fn := range handlers {
		fn(params)
	}
}

func (c *browserCDPClient) call(ctx context.Context, method string, params any, sessionID string) (json.RawMessage, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	c.ensureReader()
	id := c.nextID.Add(1)
	ch := make(chan cdpRawMessage, 1)

	c.pendingMu.Lock()
	select {
	case <-c.closed:
		err := c.closeErr
		c.pendingMu.Unlock()
		return nil, err
	default:
	}
	c.pending[id] = ch
	c.pendingMu.Unlock()

	defer func() {
		c.pendingMu.Lock()
		delete(c.pending, id)
		c.pendingMu.Unlock()
	}()

	request := map[string]any{"id": id, "method": method, "params": params}
	if sessionID != "" {
		request["sessionId"] = sessionID
	}

	c.writeMu.Lock()
	err := c.conn.SetWriteDeadline(time.Now().Add(8 * time.Second))
	if err == nil {
		err = c.conn.WriteJSON(request)
	}
	c.writeMu.Unlock()
	if err != nil {
		return nil, err
	}

	callTimer := time.NewTimer(25 * time.Second)
	defer callTimer.Stop()

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-callTimer.C:
		return nil, fmt.Errorf("Muse headless CDP %s timed out", method)
	case response, ok := <-ch:
		if !ok {
			c.pendingMu.Lock()
			err := c.closeErr
			c.pendingMu.Unlock()
			if err != nil {
				return nil, err
			}
			return nil, fmt.Errorf("Muse headless CDP connection closed during %s", method)
		}
		if len(response.Error) > 0 {
			return nil, fmt.Errorf("Muse headless CDP %s failed", method)
		}
		return response.Result, nil
	}
}

const stealthNewDocumentScript = `(() => {
	try {
		Object.defineProperty(navigator, 'webdriver', { get: () => undefined });
		if (!navigator.languages || navigator.languages.length === 0) {
			Object.defineProperty(navigator, 'languages', { get: () => ['vi-VN', 'vi', 'en-US', 'en'] });
		}
		window.chrome = window.chrome || { runtime: {} };
	} catch (_) {}
})();`

func (c *browserCDPClient) enableStealth(ctx context.Context) {
	_, _ = c.call(ctx, "Page.addScriptToEvaluateOnNewDocument", map[string]any{
		"source": stealthNewDocumentScript,
	}, c.sessionID)
}

// attachOrReusePageTarget finds and attaches to the initial page target created on
// browser launch instead of creating a second blank tab, saving an entire renderer
// process and 80-120MB of RAM per operation.
func (c *browserCDPClient) attachOrReusePageTarget(ctx context.Context) (string, string, error) {
	targetID := ""
	if targetsRaw, getErr := c.call(ctx, "Target.getTargets", map[string]any{}, ""); getErr == nil {
		var resp struct {
			TargetInfos []struct {
				TargetID string `json:"targetId"`
				Type     string `json:"type"`
			} `json:"targetInfos"`
		}
		if json.Unmarshal(targetsRaw, &resp) == nil {
			for _, t := range resp.TargetInfos {
				if t.Type == "page" && t.TargetID != "" {
					targetID = t.TargetID
					break
				}
			}
		}
	}

	if targetID == "" {
		created, createErr := c.call(ctx, "Target.createTarget", map[string]any{"url": "about:blank"}, "")
		if createErr != nil {
			return "", "", createErr
		}
		var target struct {
			TargetID string `json:"targetId"`
		}
		if json.Unmarshal(created, &target) != nil || target.TargetID == "" {
			return "", "", errors.New("headless browser did not create a page target")
		}
		targetID = target.TargetID
	}

	attached, attachErr := c.call(ctx, "Target.attachToTarget", map[string]any{"targetId": targetID, "flatten": true}, "")
	if attachErr != nil {
		return "", "", attachErr
	}
	var session struct {
		SessionID string `json:"sessionId"`
	}
	if json.Unmarshal(attached, &session) != nil || session.SessionID == "" {
		return "", "", errors.New("headless browser did not attach to page")
	}
	c.sessionID = session.SessionID
	return targetID, session.SessionID, nil
}

// setupResourceThrottling blocks bloated advertising/telemetry trackers and
// throttles runaway CSS animation render loops, cutting 30-50% CPU and 40MB+ RAM.
func (c *browserCDPClient) setupResourceThrottling(ctx context.Context) {
	blockedPatterns := []string{
		"*google-analytics.com*",
		"*googletagmanager.com*",
		"*facebook.net*",
		"*clarity.ms*",
		"*doubleclick.net*",
		"*segment.io*",
		"*datadoghq-browser-agent.com*",
		"*sentry.io*",
		"*hotjar.com*",
		"*stats.wp.com*",
	}
	_, _ = c.call(ctx, "Network.setBlockedURLs", map[string]any{"urls": blockedPatterns}, c.sessionID)

	const reduceCPUAnimationScript = `(() => {
		try {
			const style = document.createElement('style');
			style.textContent = '*, *::before, *::after { animation-duration: 0.001s !important; animation-iteration-count: 1 !important; transition-duration: 0.001s !important; }';
			document.documentElement.appendChild(style);
		} catch (_) {}
	})();`
	_, _ = c.call(ctx, "Page.addScriptToEvaluateOnNewDocument", map[string]any{
		"source": reduceCPUAnimationScript,
	}, c.sessionID)
}

func (c *browserCDPClient) purgeMemory(ctx context.Context) {
	_, _ = c.call(ctx, "HeapProfiler.collectGarbage", map[string]any{}, c.sessionID)
	_, _ = c.call(ctx, "Memory.forciblyPurgeJavaScriptMemory", map[string]any{}, c.sessionID)
}

func (c *browserCDPClient) closePage(ctx context.Context, targetID string) {
	if targetID != "" {
		c.purgeMemory(ctx)
		_, _ = c.call(ctx, "Target.closeTarget", map[string]any{"targetId": targetID}, "")
	}
}

func (c *browserCDPClient) evaluate(ctx context.Context, expression string) (any, error) {
	response, err := c.call(ctx, "Runtime.evaluate", map[string]any{"expression": expression, "returnByValue": true, "awaitPromise": true}, c.sessionID)
	if err != nil {
		return nil, err
	}
	var result struct {
		ExceptionDetails *struct {
			Text      string `json:"text"`
			Exception struct {
				Description string `json:"description"`
			} `json:"exception"`
		} `json:"exceptionDetails"`
		Result struct {
			Value any `json:"value"`
		} `json:"result"`
	}
	if err := json.Unmarshal(response, &result); err != nil {
		return nil, err
	}
	if result.ExceptionDetails != nil {
		detail := result.ExceptionDetails.Text
		if description := result.ExceptionDetails.Exception.Description; description != "" {
			detail = description
		}
		if detail == "" {
			detail = "JavaScript evaluation failed"
		}
		return nil, errors.New(detail)
	}
	return result.Result.Value, nil
}

func readDevToolsActivePortWS(profileDir string, fallbackPort int) string {
	if profileDir == "" {
		return ""
	}
	raw, err := os.ReadFile(filepath.Join(profileDir, "DevToolsActivePort"))
	if err != nil {
		return ""
	}
	lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
	if len(lines) < 2 {
		return ""
	}
	port, err := strconv.Atoi(strings.TrimSpace(lines[0]))
	if err != nil || port <= 0 {
		port = fallbackPort
	}
	path := strings.TrimSpace(lines[1])
	if port <= 0 || !strings.HasPrefix(path, "/") {
		return ""
	}
	return fmt.Sprintf("ws://127.0.0.1:%d%s", port, path)
}

func waitDebuggerURL(ctx context.Context, port int, profileDir ...string) (string, error) {
	var dir string
	if len(profileDir) > 0 {
		dir = profileDir[0]
	}
	client := &http.Client{Timeout: 2 * time.Second}
	startupTimer := time.NewTimer(30 * time.Second)
	defer startupTimer.Stop()
	for {
		if err := ctx.Err(); err != nil {
			return "", fmt.Errorf("headless browser did not start: %w", err)
		}
		if ws := readDevToolsActivePortWS(dir, port); ws != "" {
			return ws, nil
		}
		if port > 0 {
			resp, err := client.Get(fmt.Sprintf("http://127.0.0.1:%d/json/version", port))
			if err == nil {
				var meta struct {
					WS string `json:"webSocketDebuggerUrl"`
				}
				decodeErr := json.NewDecoder(resp.Body).Decode(&meta)
				_ = resp.Body.Close()
				if decodeErr == nil && meta.WS != "" {
					return meta.WS, nil
				}
			}
		}
		select {
		case <-ctx.Done():
			return "", fmt.Errorf("headless browser did not start: %w", ctx.Err())
		case <-startupTimer.C:
			return "", errors.New("headless browser did not publish its DevTools endpoint within 30 seconds")
		case <-time.After(100 * time.Millisecond):
		}
	}
}
