package webai

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

func flowWaitForValue(ctx context.Context, client *browserCDPClient, expression string, timeout time.Duration) (any, error) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		value, err := client.evaluate(ctx, expression)
		if err == nil && value == true {
			return value, nil
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
	}
	return nil, errors.New("Flow page did not reach the expected state")
}

func flowWaitAndClick(ctx context.Context, client *browserCDPClient, expression string, timeout time.Duration) (bool, error) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if err := ctx.Err(); err != nil {
			return false, err
		}
		value, err := client.evaluate(ctx, expression)
		if err == nil {
			if point, ok := value.(map[string]any); ok {
				if _, err := client.call(ctx, "Input.dispatchMouseEvent", map[string]any{"type": "mouseMoved", "x": point["x"], "y": point["y"]}, client.sessionID); err != nil {
					return false, err
				}
				if _, err := client.call(ctx, "Input.dispatchMouseEvent", map[string]any{"type": "mousePressed", "x": point["x"], "y": point["y"], "button": "left", "clickCount": 1}, client.sessionID); err != nil {
					return false, err
				}
				if _, err := client.call(ctx, "Input.dispatchMouseEvent", map[string]any{"type": "mouseReleased", "x": point["x"], "y": point["y"], "button": "left", "clickCount": 1}, client.sessionID); err != nil {
					return false, err
				}
				return true, nil
			}
		}
		select {
		case <-ctx.Done():
			return false, ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
	}
	return false, nil
}

func flowWaitForProjectID(ctx context.Context, client *browserCDPClient, timeout time.Duration) (string, error) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		value, err := client.evaluate(ctx, `location.pathname.match(/\/project\/([^/?]+)/)?.[1]||''`)
		if err == nil {
			if id, ok := value.(string); ok && id != "" {
				return id, nil
			}
		}
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
	}
	return "", errors.New("Flow did not open a new project")
}

func flowSetAspect(ctx context.Context, client *browserCDPClient, aspect string) error {
	ratio := "9:16"
	if aspect == "VIDEO_ASPECT_RATIO_LANDSCAPE" {
		ratio = "16:9"
	} else if aspect == "VIDEO_ASPECT_RATIO_SQUARE" {
		ratio = "1:1"
	}
	settings := `(() => {const e=document.querySelector('button[aria-label="Settings trigger"],.settings-trigger-button');return e?(e.innerText||e.getAttribute('aria-label')||''):''})()`
	if value, err := client.evaluate(ctx, settings); err == nil {
		if text, ok := value.(string); ok && (strings.Contains(text, ratio) || strings.Contains(text, "crop_"+strings.ReplaceAll(ratio, ":", "_"))) {
			return nil
		}
	}
	opened, err := flowWaitAndClick(ctx, client, `(() => {const e=document.querySelector('button[aria-label="Settings trigger"],.settings-trigger-button');if(!e)return null;const r=e.getBoundingClientRect();return r.width>0&&r.height>0?{x:r.x+r.width/2,y:r.y+r.height/2}:null})()`, 5*time.Second)
	if err != nil || !opened {
		return errors.New("Flow settings control was not found")
	}
	videoScript := `(() => {const e=[...document.querySelectorAll('.cdk-overlay-pane mat-button-toggle')].find(x=>/video/i.test(x.innerText||''));if(!e)return null;const r=e.getBoundingClientRect();return {x:r.x+r.width/2,y:r.y+r.height/2}})()`
	_, _ = flowWaitAndClick(ctx, client, videoScript, 5*time.Second)
	ratioJSON, _ := json.Marshal(ratio)
	ratioScript := `(() => {const t=` + string(ratioJSON) + `;const e=[...document.querySelectorAll('.cdk-overlay-pane mat-button-toggle')].find(x=>(x.innerText||'').includes(t));if(!e)return null;const b=e.querySelector('button')||e;const r=b.getBoundingClientRect();return {x:r.x+r.width/2,y:r.y+r.height/2}})()`
	clicked, err := flowWaitAndClick(ctx, client, ratioScript, 5*time.Second)
	if _, closeErr := client.call(ctx, "Input.dispatchKeyEvent", map[string]any{"type": "keyDown", "key": "Escape", "code": "Escape", "windowsVirtualKeyCode": 27}, client.sessionID); closeErr != nil {
		return closeErr
	}
	_, _ = client.call(ctx, "Input.dispatchKeyEvent", map[string]any{"type": "keyUp", "key": "Escape", "code": "Escape", "windowsVirtualKeyCode": 27}, client.sessionID)
	if err != nil {
		return err
	}
	if !clicked {
		return errors.New("Flow aspect ratio toggle was not found")
	}
	return nil
}

func stringSet(value any) map[string]bool {
	result := make(map[string]bool)
	for _, item := range valueStrings(value) {
		result[item] = true
	}
	return result
}

func valueStrings(value any) []string {
	items, ok := value.([]any)
	if !ok {
		return nil
	}
	result := make([]string, 0, len(items))
	for _, item := range items {
		if value, ok := item.(string); ok && value != "" {
			result = append(result, value)
		}
	}
	return result
}
