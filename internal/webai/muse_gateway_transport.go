package webai

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/gorilla/websocket"
)

func (g *museGateway) handshake() error {
	state, err := newNoiseXX()
	if err != nil {
		return errors.New("Muse Noise handshake initialization failed")
	}
	msg1, err := state.writeMessage1()
	if err != nil {
		return err
	}
	if err := g.writeBinary(msg1); err != nil {
		return errors.New("Muse Noise handshake send failed")
	}
	msg2, err := g.readBinary(30 * time.Second)
	if err != nil {
		return errors.New("Muse Noise handshake response failed")
	}
	msg3, err := state.readMessage2(msg2)
	if err != nil {
		return errors.New("Muse Noise handshake response could not be decrypted")
	}
	if err := g.writeBinary(msg3); err != nil {
		return errors.New("Muse Noise handshake completion failed")
	}
	g.sendKey, g.recvKey = state.split()
	return nil
}

func (g *museGateway) latestAssistantSeq(ctx context.Context) (uint64, error) {
	events, err := g.history(ctx, 1)
	if err != nil {
		return 0, err
	}
	var seq uint64
	for _, event := range events {
		if n := museEventSeq(event); n > seq {
			seq = n
		}
	}
	return seq, nil
}

func (g *museGateway) history(ctx context.Context, limit int) ([]map[string]any, error) {
	query := url.Values{"limit": {fmt.Sprint(limit)}}
	data, err := g.callJSON(ctx, http.MethodGet, "/chat/history", nil, query, 35*time.Second)
	if err != nil {
		return nil, err
	}
	rows, _ := data["chat_events"].([]any)
	out := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		if event, ok := row.(map[string]any); ok {
			out = append(out, event)
		}
	}
	return out, nil
}

func (g *museGateway) sendPrompt(ctx context.Context, deviceID, prompt string) error {
	if deviceID == "" {
		deviceID = "aicos"
	}
	params := map[string]any{
		"items":        []any{map[string]any{"type": "text", "text": prompt}},
		"node_id":      deviceID,
		"capabilities": map[string]any{},
	}
	return g.openRequest(ctx, http.MethodPost, "/chat/stream", params, nil)
}

func (g *museGateway) callJSON(ctx context.Context, method, path string, body any, query url.Values, timeout time.Duration) (map[string]any, error) {
	raw, err := g.request(ctx, method, path, body, query, timeout)
	if err != nil {
		return nil, err
	}
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, errors.New("Muse gateway returned invalid JSON")
	}
	data, _ := value.(map[string]any)
	if result, ok := data["result"].(map[string]any); ok {
		data = result
	}
	if data == nil {
		return nil, errors.New("Muse gateway response has unexpected shape")
	}
	if ok, exists := data["ok"].(bool); exists && !ok {
		return nil, errors.New("Muse gateway returned an application error")
	}
	return data, nil
}

func (g *museGateway) request(ctx context.Context, method, path string, body any, query url.Values, timeout time.Duration) ([]byte, error) {
	if method == http.MethodGet && body != nil && query == nil {
		if fields, ok := body.(map[string]any); ok {
			query = make(url.Values, len(fields))
			for key, value := range fields {
				query.Set(key, fmt.Sprint(value))
			}
			body = nil
		}
	}
	if query != nil && len(query) > 0 {
		path += "?" + query.Encode()
	}
	return g.openAndRead(ctx, method, path, body, timeout)
}

func (g *museGateway) openAndRead(ctx context.Context, method, path string, body any, timeout time.Duration) ([]byte, error) {
	sid := g.streamID
	g.streamID++
	encodedBody := []byte(nil)
	if body != nil {
		var err error
		encodedBody, err = json.Marshal(body)
		if err != nil {
			return nil, err
		}
	}
	appReq := protoString(1, method)
	appReq = append(appReq, protoString(2, path)...)
	appReq = append(appReq, protoBytes(3, protoHeader("x-request-id", newMuseRequestID()))...)
	if len(encodedBody) > 0 {
		appReq = append(appReq, protoBytes(3, protoHeader("content-type", "application/json"))...)
		appReq = append(appReq, protoBytes(4, encodedBody)...)
	}
	appReq = append(appReq, protoUint(5, 1)...)
	if err := g.openEncodedRequest(ctx, sid, appReq); err != nil {
		return nil, err
	}

	deadline := time.Now().Add(timeout)
	var status uint64
	var responseBody []byte
	for time.Now().Before(deadline) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		remaining := time.Until(deadline)
		frame, err := g.readApplicationFrame(remaining)
		if err != nil {
			return nil, err
		}
		if frame.streamID != sid {
			continue
		}
		if frame.status != 0 {
			status = frame.status
		}
		responseBody = append(responseBody, frame.body...)
		if frame.reset != "" {
			return nil, errors.New("Muse gateway reset the request")
		}
		if frame.end {
			if status < 200 || status >= 300 {
				return nil, fmt.Errorf("Muse gateway HTTP %d", status)
			}
			return responseBody, nil
		}
	}
	return nil, errors.New("Muse gateway request timed out")
}

func (g *museGateway) openRequest(ctx context.Context, method, path string, body any, query url.Values) error {
	encodedBody, err := json.Marshal(body)
	if err != nil {
		return err
	}
	if query != nil && len(query) > 0 {
		path += "?" + query.Encode()
	}
	appReq := protoString(1, method)
	appReq = append(appReq, protoString(2, path)...)
	appReq = append(appReq, protoBytes(3, protoHeader("x-request-id", newMuseRequestID()))...)
	appReq = append(appReq, protoBytes(3, protoHeader("content-type", "application/json"))...)
	appReq = append(appReq, protoBytes(4, encodedBody)...)
	appReq = append(appReq, protoUint(5, 1)...)
	sid := g.streamID
	g.streamID++
	return g.openEncodedRequest(ctx, sid, appReq)
}

func protoHeader(key, value string) []byte {
	out := protoString(1, key)
	return append(out, protoString(2, value)...)
}

func (g *museGateway) openEncodedRequest(ctx context.Context, sid uint64, appReq []byte) error {
	serviceFrame := protoUint(1, sid)
	serviceFrame = append(serviceFrame, protoBytes(2, appReq)...)
	serviceRequest := protoUint(1, 0)
	serviceRequest = append(serviceRequest, protoBytes(2, serviceFrame)...)
	chunk := protoUint(1, uint64(time.Now().UnixNano()))
	chunk = append(chunk, protoUint(3, 1)...)
	chunk = append(chunk, protoBytes(4, serviceRequest)...)
	transport, err := museNoiseEncrypt(g.sendKey, g.sendNonce, chunk)
	if err != nil {
		return errors.New("Muse gateway request encryption failed")
	}
	g.sendNonce++
	if err := g.writeBinary(transport); err != nil {
		return errors.New("Muse gateway request send failed")
	}
	return nil
}

type museApplicationFrame struct {
	streamID uint64
	status   uint64
	body     []byte
	end      bool
	reset    string
}

func (g *museGateway) readApplicationFrame(timeout time.Duration) (museApplicationFrame, error) {
	var out museApplicationFrame
	if timeout <= 0 {
		return out, errors.New("Muse gateway request timed out")
	}
	if err := g.conn.SetReadDeadline(time.Now().Add(timeout)); err != nil {
		return out, errors.New("Muse gateway read deadline failed")
	}
	data, err := g.readBinary(timeout)
	if err != nil {
		return out, errors.New("Muse gateway response timed out or disconnected")
	}
	plain, err := museNoiseDecrypt(g.recvKey, g.recvNonce, data)
	if err != nil {
		return out, errors.New("Muse gateway response decryption failed")
	}
	g.recvNonce++
	outer, err := parseProto(plain)
	if err != nil {
		return out, errors.New("Muse gateway response frame malformed")
	}
	servicePayload := firstBytes(outer, 4)
	serviceResponse, err := parseProto(servicePayload)
	if err != nil {
		return out, errors.New("Muse gateway response envelope malformed")
	}
	serviceFrame, err := parseProto(firstBytes(serviceResponse, 1))
	if err != nil {
		return out, errors.New("Muse gateway response stream malformed")
	}
	out.streamID = firstVarint(serviceFrame, 1)
	if response := firstBytes(serviceFrame, 3); len(response) > 0 {
		fields, err := parseProto(response)
		if err != nil {
			return out, errors.New("Muse application response malformed")
		}
		out.status = firstVarint(fields, 1)
		out.body = append(out.body, firstBytes(fields, 3)...)
		out.end = firstVarint(fields, 4) != 0
	}
	if chunk := firstBytes(serviceFrame, 4); len(chunk) > 0 {
		fields, err := parseProto(chunk)
		if err != nil {
			return out, errors.New("Muse response chunk malformed")
		}
		out.body = append(out.body, firstBytes(fields, 1)...)
		out.end = out.end || firstVarint(fields, 2) != 0
	}
	if reset := firstBytes(serviceFrame, 5); len(reset) > 0 {
		fields, _ := parseProto(reset)
		out.reset = string(firstBytes(fields, 2))
		if out.reset == "" {
			out.reset = "reset"
		}
	}
	return out, nil
}

func (g *museGateway) writeBinary(data []byte) error {
	if err := g.conn.SetWriteDeadline(time.Now().Add(25 * time.Second)); err != nil {
		return err
	}
	return g.conn.WriteMessage(websocket.BinaryMessage, data)
}

func (g *museGateway) readBinary(timeout time.Duration) ([]byte, error) {
	if timeout <= 0 {
		return nil, errors.New("timeout")
	}
	_ = g.conn.SetReadDeadline(time.Now().Add(timeout))
	for {
		kind, data, err := g.conn.ReadMessage()
		if err != nil {
			return nil, err
		}
		if kind == websocket.BinaryMessage {
			return data, nil
		}
	}
}

func newMuseRequestID() string {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return fmt.Sprintf("aicos-%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(raw[:])
}

func museDeviceID(accountID string) string {
	if accountID == "" {
		return "aicos"
	}
	sum := sha256.Sum256([]byte("aicos-muse-device:" + accountID))
	return hex.EncodeToString(sum[:8])
}
