package webai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
)

type browserCDPClient struct {
	conn      *websocket.Conn
	sessionID string
	nextID    atomic.Int64
	callMu    sync.Mutex
}

func (c *browserCDPClient) call(ctx context.Context, method string, params any, sessionID string) (json.RawMessage, error) {
	c.callMu.Lock()
	defer c.callMu.Unlock()
	id := c.nextID.Add(1)
	request := map[string]any{"id": id, "method": method, "params": params}
	if sessionID != "" {
		request["sessionId"] = sessionID
	}
	if err := c.conn.SetWriteDeadline(time.Now().Add(8 * time.Second)); err != nil {
		return nil, err
	}
	if err := c.conn.WriteJSON(request); err != nil {
		return nil, err
	}
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		_ = c.conn.SetReadDeadline(time.Now().Add(8 * time.Second))
		_, data, err := c.conn.ReadMessage()
		if err != nil {
			return nil, fmt.Errorf("Muse headless CDP connection failed: %w", err)
		}
		var response struct {
			ID     int64           `json:"id"`
			Result json.RawMessage `json:"result"`
			Error  json.RawMessage `json:"error"`
		}
		if json.Unmarshal(data, &response) != nil || response.ID != id {
			continue
		}
		if len(response.Error) > 0 {
			return nil, fmt.Errorf("Muse headless CDP %s failed", method)
		}
		return response.Result, nil
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

func waitDebuggerURL(ctx context.Context, port int) (string, error) {
	client := &http.Client{Timeout: 2 * time.Second}
	for {
		if err := ctx.Err(); err != nil {
			return "", errors.New("Muse headless browser did not start")
		}
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
		select {
		case <-ctx.Done():
			return "", errors.New("Muse headless browser did not start")
		case <-time.After(250 * time.Millisecond):
		}
	}
}
