package webai

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdh"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

const (
	museHTTPBase  = "https://muse.ai"
	museWSBase    = "wss://hatch.metaaivm.com/v1/noise"
	museUserAgent = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/153.0.0.0 Safari/537.36"
)

// MuseReply contains the text returned by Muse and media links explicitly
// present in that reply. Muse's chat transport is an internal Noise gateway.
type MuseReply struct {
	Text      string
	MediaURLs []string
}

// ProbeMuseSession validates saved Muse credentials without opening a browser
// or sending a chat message.
func ProbeMuseSession(ctx context.Context, cookie string) error {
	if strings.TrimSpace(cookie) == "" {
		return errors.New("Muse Chat chưa có cookie phiên")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	g, err := newMuseGateway(ctx, cookie)
	if err != nil {
		return err
	}
	defer g.Close()
	_, err = g.latestAssistantSeq(ctx)
	return err
}

// MuseChat sends one prompt through Muse's authenticated Hatch gateway and
// polls chat.history until a genuine assistant reply arrives.
func MuseChat(ctx context.Context, cookie, deviceID, prompt string, progress func(string)) (MuseReply, error) {
	var out MuseReply
	if ctx == nil {
		ctx = context.Background()
	}
	if strings.TrimSpace(cookie) == "" {
		return out, errors.New("Muse Chat chưa có cookie phiên; kết nối tài khoản Muse trước")
	}
	if strings.TrimSpace(prompt) == "" {
		return out, errors.New("Muse Chat yêu cầu prompt không rỗng")
	}
	if progress == nil {
		progress = func(string) {}
	}
	g, err := newMuseGateway(ctx, cookie)
	if err != nil {
		return out, &MuseGatewayUnavailableError{cause: err}
	}
	defer g.Close()
	stopOnCancel := context.AfterFunc(ctx, g.Close)
	defer stopOnCancel()

	baseline, err := g.latestAssistantSeq(ctx)
	if err != nil {
		return out, &MuseGatewayUnavailableError{cause: fmt.Errorf("Muse Chat không đọc được lịch sử: %w", err)}
	}
	progress("MUSE_SEND: gửi prompt tới Muse Chat")
	if err := g.sendPrompt(ctx, museDeviceID(deviceID), prompt); err != nil {
		return out, fmt.Errorf("Muse Chat không nhận prompt: %w", err)
	}

	deadline := time.Now().Add(12 * time.Minute)
	for time.Now().Before(deadline) {
		if err := ctx.Err(); err != nil {
			return out, err
		}
		events, err := g.history(ctx, 20)
		if err != nil {
			if ctx.Err() != nil {
				return out, ctx.Err()
			}
			progress("MUSE_WAIT: chờ phản hồi Muse Chat")
		} else {
			for _, event := range events {
				if isMuseReply(event, baseline) {
					text := museEventText(event)
					if strings.TrimSpace(text) == "" {
						continue
					}
					out.Text = text
					out.MediaURLs = extractMuseMediaURLs(event, text)
					return out, nil
				}
			}
		}
		select {
		case <-ctx.Done():
			return out, ctx.Err()
		case <-time.After(4 * time.Second):
		}
	}
	return out, errors.New("Muse Chat đã nhận prompt nhưng chưa trả lời trong 12 phút")
}

type museGateway struct {
	conn      *websocket.Conn
	ctx       context.Context
	client    *http.Client
	streamID  uint64
	sendKey   []byte
	recvKey   []byte
	sendNonce uint64
	recvNonce uint64
}

func newMuseGateway(ctx context.Context, cookie string) (*museGateway, error) {
	client := &http.Client{Timeout: 25 * time.Second}
	access, err := museFetchAccess(ctx, client, cookie)
	if err != nil {
		return nil, err
	}
	vmID, err := museFetchVM(ctx, client, cookie)
	if err != nil {
		return nil, err
	}
	hatchToken, err := museFetchHatchToken(ctx, client, cookie, access, vmID)
	if err != nil {
		return nil, err
	}

	endpoint, _ := url.Parse(museWSBase)
	q := endpoint.Query()
	q.Set("vm_id", vmID)
	q.Set("auth_token", hatchToken)
	endpoint.RawQuery = q.Encode()
	header := http.Header{}
	header.Set("Origin", museHTTPBase)
	header.Set("User-Agent", museUserAgent)
	dialer := websocket.Dialer{Proxy: http.ProxyFromEnvironment, HandshakeTimeout: 25 * time.Second}
	conn, response, err := dialer.DialContext(ctx, endpoint.String(), header)
	if err != nil {
		if response != nil {
			return nil, fmt.Errorf("Muse gateway websocket handshake failed (HTTP %d)", response.StatusCode)
		}
		return nil, errors.New("Muse gateway websocket connection failed")
	}
	g := &museGateway{conn: conn, ctx: ctx, client: client, streamID: 1}
	stop := context.AfterFunc(ctx, func() { _ = conn.Close() })
	if err := g.handshake(); err != nil {
		stop()
		_ = conn.Close()
		return nil, err
	}
	stop()
	return g, nil
}

func (g *museGateway) Close() {
	if g != nil && g.conn != nil {
		_ = g.conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""), time.Now().Add(time.Second))
		_ = g.conn.Close()
	}
}

func museFetchAccess(ctx context.Context, client *http.Client, cookie string) (string, error) {
	var data map[string]any
	if err := museHTTPJSON(ctx, client, http.MethodPost, museHTTPBase+"/api/auth/check", cookie, "", nil, &data); err != nil {
		return "", fmt.Errorf("Muse authentication check failed: %w", err)
	}
	token, _ := data["access_token"].(string)
	if token == "" {
		return "", errors.New("Muse session expired or auth response omitted access token")
	}
	return token, nil
}

func museFetchVM(ctx context.Context, client *http.Client, cookie string) (string, error) {
	var data map[string]any
	if err := museHTTPJSON(ctx, client, http.MethodGet, museHTTPBase+"/api/session", cookie, "", nil, &data); err != nil {
		return "", fmt.Errorf("Muse session lookup failed: %w", err)
	}
	if status, _ := data["status"].(string); status == "unavailable" {
		return "", errors.New("Muse VM is temporarily unavailable; retry later")
	}
	vmID, _ := data["vm_id"].(string)
	if vmID == "" {
		return "", errors.New("Muse session response omitted VM id")
	}
	return vmID, nil
}

func museFetchHatchToken(ctx context.Context, client *http.Client, cookie, access, vmID string) (string, error) {
	body := map[string]string{"vmAddress": "wss://" + vmID + ".metaaivm.com/", "vmName": vmID}
	var data map[string]any
	if err := museHTTPJSON(ctx, client, http.MethodPost, museHTTPBase+"/api/hatch/token", cookie, access, body, &data); err != nil {
		return "", fmt.Errorf("Muse gateway token request failed: %w", err)
	}
	token, _ := data["token"].(string)
	if token == "" {
		return "", errors.New("Muse gateway response omitted access token")
	}
	return token, nil
}

func museHTTPJSON(ctx context.Context, client *http.Client, method, target, cookie, bearer string, payload any, dst any) error {
	var body io.Reader
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		body = strings.NewReader(string(b))
	}
	req, err := http.NewRequestWithContext(ctx, method, target, body)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", museUserAgent)
	req.Header.Set("Referer", museHTTPBase+"/")
	req.Header.Set("Origin", museHTTPBase)
	req.Header.Set("Sec-Fetch-Site", "same-origin")
	req.Header.Set("Sec-Fetch-Mode", "cors")
	req.Header.Set("Sec-Fetch-Dest", "empty")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Cookie", cookie)
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	resp, err := client.Do(req)
	if err != nil {
		return errors.New("network request failed")
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 2<<20)).Decode(dst); err != nil {
		return errors.New("invalid JSON response")
	}
	return nil
}

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

type museNoiseXX struct {
	curve   ecdh.Curve
	eph     *ecdh.PrivateKey
	static  *ecdh.PrivateKey
	re      []byte
	rs      []byte
	h       []byte
	ck      []byte
	key     []byte
	nonce   uint64
	sendKey []byte
	recvKey []byte
}

func newNoiseXX() (*museNoiseXX, error) {
	curve := ecdh.X25519()
	ephemeral, err := curve.GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	static, err := curve.GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	protocol := []byte("Noise_XX_25519_AESGCM_SHA256")
	h := make([]byte, sha256.Size)
	if len(protocol) <= len(h) {
		copy(h, protocol)
	} else {
		sum := sha256.Sum256(protocol)
		h = sum[:]
	}
	return &museNoiseXX{curve: curve, eph: ephemeral, static: static, h: museHash(h), ck: append([]byte(nil), h...)}, nil
}

func (n *museNoiseXX) writeMessage1() ([]byte, error) {
	message := n.eph.PublicKey().Bytes()
	n.mixHash(message)
	return message, nil
}

func (n *museNoiseXX) readMessage2(message []byte) ([]byte, error) {
	if len(message) < 32+48+16 {
		return nil, errors.New("short handshake message")
	}
	n.re = append([]byte(nil), message[:32]...)
	n.mixHash(n.re)
	peer, err := n.curve.NewPublicKey(n.re)
	if err != nil {
		return nil, err
	}
	shared, err := n.eph.ECDH(peer)
	if err != nil {
		return nil, err
	}
	n.mixKey(shared)
	staticPlain, err := n.decryptAndHash(message[32:80])
	if err != nil || len(staticPlain) != 32 {
		return nil, errors.New("invalid peer static key")
	}
	n.rs = append([]byte(nil), staticPlain...)
	peerStatic, err := n.curve.NewPublicKey(n.rs)
	if err != nil {
		return nil, err
	}
	shared, err = n.eph.ECDH(peerStatic)
	if err != nil {
		return nil, err
	}
	n.mixKey(shared)
	if payload, err := n.decryptAndHash(message[80:]); err != nil || len(payload) != 0 {
		return nil, errors.New("invalid handshake payload")
	}

	msg3, err := n.encryptAndHash(n.static.PublicKey().Bytes())
	if err != nil {
		return nil, err
	}
	peerEphemeral, err := n.curve.NewPublicKey(n.re)
	if err != nil {
		return nil, err
	}
	shared, err = n.static.ECDH(peerEphemeral)
	if err != nil {
		return nil, err
	}
	n.mixKey(shared)
	emptyPayload, err := n.encryptAndHash(nil)
	if err != nil {
		return nil, err
	}
	msg3 = append(msg3, emptyPayload...)
	n.sendKey, n.recvKey = museNoiseHKDF(n.ck, nil)
	return msg3, nil
}

func (n *museNoiseXX) split() ([]byte, []byte) {
	return n.sendKey, n.recvKey
}

func (n *museNoiseXX) mixHash(data []byte) {
	n.h = museHash(append(append([]byte(nil), n.h...), data...))
}

func (n *museNoiseXX) mixKey(input []byte) {
	n.ck, n.key = museNoiseHKDF(n.ck, input)
	n.nonce = 0
}

func (n *museNoiseXX) encryptAndHash(plain []byte) ([]byte, error) {
	out, err := museNoiseEncryptWithAD(n.key, n.nonce, plain, n.h)
	if err != nil {
		return nil, err
	}
	n.nonce++
	n.mixHash(out)
	return out, nil
}

func (n *museNoiseXX) decryptAndHash(ciphertext []byte) ([]byte, error) {
	out, err := museNoiseDecryptWithAD(n.key, n.nonce, ciphertext, n.h)
	if err != nil {
		return nil, err
	}
	n.nonce++
	n.mixHash(ciphertext)
	return out, nil
}

func museNoiseHKDF(chainingKey, input []byte) ([]byte, []byte) {
	mac := hmac.New(sha256.New, chainingKey)
	_, _ = mac.Write(input)
	prk := mac.Sum(nil)
	mac = hmac.New(sha256.New, prk)
	_, _ = mac.Write([]byte{1})
	out1 := mac.Sum(nil)
	mac = hmac.New(sha256.New, prk)
	_, _ = mac.Write(out1)
	_, _ = mac.Write([]byte{2})
	return out1, mac.Sum(nil)
}

func museNoiseEncrypt(key []byte, nonce uint64, plain []byte) ([]byte, error) {
	return museNoiseEncryptWithAD(key, nonce, plain, nil)
}

func museNoiseDecrypt(key []byte, nonce uint64, encrypted []byte) ([]byte, error) {
	return museNoiseDecryptWithAD(key, nonce, encrypted, nil)
}

func museNoiseEncryptWithAD(key []byte, nonce uint64, plain, aad []byte) ([]byte, error) {
	aead, err := museAEAD(key)
	if err != nil {
		return nil, err
	}
	return aead.Seal(nil, museNonce(nonce), plain, aad), nil
}

func museNoiseDecryptWithAD(key []byte, nonce uint64, encrypted, aad []byte) ([]byte, error) {
	aead, err := museAEAD(key)
	if err != nil {
		return nil, err
	}
	return aead.Open(nil, museNonce(nonce), encrypted, aad)
}

func museAEAD(key []byte) (cipher.AEAD, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

func museNonce(n uint64) []byte {
	var nonce [12]byte
	binary.LittleEndian.PutUint64(nonce[4:], n)
	return nonce[:]
}

func museHash(data []byte) []byte {
	sum := sha256.Sum256(data)
	return sum[:]
}

type museProtoField struct {
	number int
	wire   uint64
	value  uint64
	bytes  []byte
}

func parseProto(data []byte) ([]museProtoField, error) {
	var fields []museProtoField
	for len(data) > 0 {
		key, n := binary.Uvarint(data)
		if n <= 0 || key>>3 == 0 {
			return nil, errors.New("invalid protobuf key")
		}
		data = data[n:]
		f := museProtoField{number: int(key >> 3), wire: key & 7}
		switch f.wire {
		case 0:
			v, n := binary.Uvarint(data)
			if n <= 0 {
				return nil, errors.New("invalid protobuf varint")
			}
			f.value, data = v, data[n:]
		case 1:
			if len(data) < 8 {
				return nil, errors.New("short protobuf fixed64")
			}
			data = data[8:]
		case 2:
			length, n := binary.Uvarint(data)
			if n <= 0 || length > uint64(len(data)-n) {
				return nil, errors.New("invalid protobuf length")
			}
			data = data[n:]
			f.bytes = data[:int(length)]
			data = data[int(length):]
		case 5:
			if len(data) < 4 {
				return nil, errors.New("short protobuf fixed32")
			}
			data = data[4:]
		default:
			return nil, errors.New("unsupported protobuf wire type")
		}
		fields = append(fields, f)
	}
	return fields, nil
}

func firstBytes(fields []museProtoField, number int) []byte {
	for _, f := range fields {
		if f.number == number && f.wire == 2 {
			return f.bytes
		}
	}
	return nil
}

func firstVarint(fields []museProtoField, number int) uint64 {
	for _, f := range fields {
		if f.number == number && f.wire == 0 {
			return f.value
		}
	}
	return 0
}

func protoUint(number int, value uint64) []byte {
	out := appendUvarint(nil, uint64(number<<3))
	return appendUvarint(out, value)
}

func protoBytes(number int, value []byte) []byte {
	out := appendUvarint(nil, uint64(number<<3|2))
	out = appendUvarint(out, uint64(len(value)))
	return append(out, value...)
}

func protoString(number int, value string) []byte { return protoBytes(number, []byte(value)) }

func appendUvarint(dst []byte, value uint64) []byte {
	for value >= 0x80 {
		dst = append(dst, byte(value)|0x80)
		value >>= 7
	}
	return append(dst, byte(value))
}

func museEventSeq(event map[string]any) uint64 {
	switch value := event["seq"].(type) {
	case float64:
		if value > 0 {
			return uint64(value)
		}
	case json.Number:
		parsed, _ := value.Int64()
		if parsed > 0 {
			return uint64(parsed)
		}
	}
	return 0
}

func museEventText(event map[string]any) string {
	payload, _ := event["payload"].(map[string]any)
	if value, ok := payload["display_text"].(string); ok && value != "" {
		return value
	}
	if value, ok := event["display_text"].(string); ok && value != "" {
		return value
	}
	if value, ok := payload["content"].(string); ok && value != "" {
		return value
	}
	if value, ok := event["text"].(string); ok {
		return value
	}
	return ""
}

func isMuseReply(event map[string]any, baseline uint64) bool {
	name, _ := event["event_name"].(string)
	if name != "message.assistant" || museEventSeq(event) <= baseline {
		return false
	}
	payload, _ := event["payload"].(map[string]any)
	if reply, _ := event["reply_to_message_id"].(string); reply != "" {
		return false
	}
	if reply, _ := payload["reply_to_message_id"].(string); reply != "" {
		return false
	}
	if ready, ok := payload["display_text_ready"].(bool); ok && !ready {
		return false
	}
	return strings.TrimSpace(museEventText(event)) != ""
}

func extractMuseMediaURLs(event map[string]any, text string) []string {
	serialized, _ := json.Marshal(event)
	content := string(serialized) + "\n" + text
	start := 0
	seen := map[string]bool{}
	var links []string
	for {
		i := strings.Index(content[start:], "https://muse.ai/files/")
		if i < 0 {
			break
		}
		start += i
		end := start
		for end < len(content) && !strings.ContainsRune(" \t\r\n\"'<>)]}", rune(content[end])) {
			end++
		}
		link := strings.TrimRight(content[start:end], ".,;!?")
		parsed, err := url.Parse(link)
		if err == nil && parsed.Scheme == "https" && (parsed.Hostname() == "muse.ai" || parsed.Hostname() == "www.muse.ai") && strings.HasPrefix(parsed.Path, "/files/") {
			ext := strings.ToLower(filepath.Ext(parsed.Path))
			if (ext == ".mp4" || ext == ".webm" || ext == ".png" || ext == ".jpg" || ext == ".jpeg" || ext == ".webp") && !seen[link] {
				seen[link] = true
				links = append(links, link)
			}
		}
		start = end
		if start >= len(content) {
			break
		}
	}
	return links
}
