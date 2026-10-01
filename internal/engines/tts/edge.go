package tts

// Edge TTS (Microsoft Edge read-aloud endpoint) — no API key needed.
//
// Unofficial endpoint: free, but ToS-gray and rate limits are unpublished.
// Position in the chain: LAST resort.
//
// Faithful port of engines/tts/edge_tts.py on stdlib only: raw TCP (+TLS)
// with a minimal WebSocket framing implementation (handshake, masked text
// frames, ping/pong) — not a general WS client. The received MP3 is
// normalized to WAV (PCM s16le 24kHz mono) via ffmpeg.

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	edgeHost   = "speech.platform.bing.com"
	edgePath   = "/consumer/speech/synthesize/readaloud/edge/v1?TrustedClientToken=6A5AA1D4EAFF4E9FB37E23D68491D6F4"
	edgeUA     = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/127.0.0.0 Safari/537.36 Edg/127.0.0.0"
	edgeOrigin = "chrome-extension://jdiccldimpdaibmpdkjnbmckianbfoldn"
)

// persona voice style -> Edge neural voice (port of VI_VOICES).
var edgeVoices = map[string]string{
	"default":          "vi-VN-HoaiMyNeural",
	"warm-female":      "vi-VN-HoaiMyNeural",
	"clear-teacher":    "vi-VN-HoaiMyNeural",
	"singer":           "vi-VN-HoaiMyNeural",
	"upbeat-host":      "vi-VN-HoaiMyNeural",
	"energetic-caster": "vi-VN-NamMinhNeural",
	"male":             "vi-VN-NamMinhNeural",
}

// EdgeTTSProvider is the tier-3 TTS provider: the unofficial Edge read-aloud
// endpoint. No key; last resort only.
type EdgeTTSProvider struct {
	// Rate and Pitch are SSML prosody values, e.g. "+0%", "+0Hz".
	Rate  string
	Pitch string
}

// NewEdgeTTSProvider builds the tier-3 TTS provider.
func NewEdgeTTSProvider() *EdgeTTSProvider {
	return &EdgeTTSProvider{Rate: "+0%", Pitch: "+0Hz"}
}

func (e *EdgeTTSProvider) Name() string { return "edge" }

// Healthy is true when ffmpeg is available for the MP3 -> WAV conversion.
// (Network reachability is only known when actually synthesizing.)
func (e *EdgeTTSProvider) Healthy(ctx context.Context) bool {
	_, err := ffmpegBin()
	return err == nil
}

func (e *EdgeTTSProvider) Synthesize(ctx context.Context, text, voice string) ([]byte, error) {
	if strings.TrimSpace(text) == "" {
		return nil, fmt.Errorf("edge: empty text")
	}
	if _, err := ffmpegBin(); err != nil {
		return nil, fmt.Errorf("edge: %w", err)
	}
	mp3, err := edgeSynthesize(ctx, text, voice, e.Rate, e.Pitch)
	if err != nil {
		return nil, fmt.Errorf("edge: %w", err)
	}
	wav, err := mp3ToWav24kMono(ctx, mp3)
	if err != nil {
		return nil, fmt.Errorf("edge: %w", err)
	}
	return wav, nil
}

// ---------------------------------------------------------------------------
// minimal WebSocket client (handshake, masked text frames, recv loop)

type edgeWS struct {
	conn net.Conn
	buf  []byte
}

// proxyFromEnv reads HTTPS_PROXY/https_proxy and returns host, port and an
// optional Proxy-Authorization header value.
func proxyFromEnv() (host string, port int, auth string, ok bool) {
	for _, v := range []string{"HTTPS_PROXY", "https_proxy"} {
		val := os.Getenv(v)
		if val == "" {
			continue
		}
		rest := val
		if i := strings.Index(rest, "://"); i >= 0 {
			rest = rest[i+3:]
		}
		rest = strings.TrimSuffix(rest, "/")
		if i := strings.LastIndex(rest, "@"); i >= 0 {
			// credentials are used for the CONNECT only, never logged
			auth = "Basic " + base64.StdEncoding.EncodeToString([]byte(rest[:i]))
			rest = rest[i+1:]
		}
		h, p, _ := strings.Cut(rest, ":")
		pi, _ := strconv.Atoi(p)
		if pi == 0 {
			pi = 3128
		}
		return h, pi, auth, true
	}
	return "", 0, "", false
}

// edgeDial opens a TCP connection: direct, or HTTP CONNECT through the env proxy.
func edgeDial(ctx context.Context) (net.Conn, error) {
	dialer := &net.Dialer{Timeout: 20 * time.Second}
	if phost, pport, auth, ok := proxyFromEnv(); ok {
		conn, err := dialer.DialContext(ctx, "tcp", net.JoinHostPort(phost, strconv.Itoa(pport)))
		if err != nil {
			return nil, err
		}
		var sb strings.Builder
		sb.WriteString("CONNECT " + edgeHost + ":443 HTTP/1.1\r\nHost: " + edgeHost + ":443\r\n")
		if auth != "" {
			sb.WriteString("Proxy-Authorization: " + auth + "\r\n")
		}
		sb.WriteString("\r\n")
		if _, err := conn.Write([]byte(sb.String())); err != nil {
			conn.Close()
			return nil, err
		}
		// read the CONNECT response head byte-by-byte (no buffering, so the
		// TLS handshake that follows sees every byte)
		head, err := readHead(conn, 20*time.Second)
		if err != nil {
			conn.Close()
			return nil, fmt.Errorf("proxy CONNECT: %w", err)
		}
		status := strings.SplitN(head, "\r\n", 2)[0]
		if !strings.Contains(status, " 200 ") {
			conn.Close()
			return nil, fmt.Errorf("proxy CONNECT failed: %.80s", status)
		}
		return conn, nil
	}
	return dialer.DialContext(ctx, "tcp", edgeHost+":443")
}

// readHead reads until "\r\n\r\n", returning the head as latin1 text.
func readHead(conn net.Conn, timeout time.Duration) (string, error) {
	if timeout > 0 {
		conn.SetReadDeadline(time.Now().Add(timeout))
	}
	var data []byte
	tmp := make([]byte, 1)
	for {
		n, err := conn.Read(tmp)
		if n > 0 {
			data = append(data, tmp[:n]...)
			if bytes.HasSuffix(data, []byte("\r\n\r\n")) {
				return string(data[:len(data)-4]), nil
			}
			if len(data) > 65536 {
				return "", fmt.Errorf("response head too large")
			}
		}
		if err != nil {
			return "", err
		}
	}
}

func (w *edgeWS) connect(ctx context.Context) error {
	raw, err := edgeDial(ctx)
	if err != nil {
		return err
	}
	tlsConn := tls.Client(raw, &tls.Config{ServerName: edgeHost})
	if deadline, ok := ctx.Deadline(); ok {
		tlsConn.SetDeadline(deadline)
	} else {
		tlsConn.SetDeadline(time.Now().Add(30 * time.Second))
	}
	if err := tlsConn.HandshakeContext(ctx); err != nil {
		raw.Close()
		return fmt.Errorf("TLS handshake: %w", err)
	}
	w.conn = tlsConn

	key := make([]byte, 16)
	if _, err := rand.Read(key); err != nil {
		w.conn.Close()
		return err
	}
	req := "GET " + edgePath + " HTTP/1.1\r\n" +
		"Host: " + edgeHost + "\r\n" +
		"Upgrade: websocket\r\n" +
		"Connection: Upgrade\r\n" +
		"Sec-WebSocket-Key: " + base64.StdEncoding.EncodeToString(key) + "\r\n" +
		"Sec-WebSocket-Version: 13\r\n" +
		"Origin: " + edgeOrigin + "\r\n" +
		"User-Agent: " + edgeUA + "\r\n" +
		"Pragma: no-cache\r\n" +
		"Cache-Control: no-cache\r\n" +
		"\r\n"
	if _, err := w.conn.Write([]byte(req)); err != nil {
		w.conn.Close()
		return err
	}
	head, err := readHead(w.conn, 0) // deadline already set from ctx
	if err != nil {
		w.conn.Close()
		return fmt.Errorf("handshake read: %w", err)
	}
	status := strings.SplitN(head, "\r\n", 2)[0]
	if !strings.Contains(status, "101") {
		w.conn.Close()
		return fmt.Errorf("WS handshake failed: %.120s", status)
	}
	return nil
}

func (w *edgeWS) recvExact(n int) ([]byte, error) {
	for len(w.buf) < n {
		tmp := make([]byte, 65536)
		m, err := w.conn.Read(tmp)
		if m > 0 {
			w.buf = append(w.buf, tmp[:m]...)
		}
		if err != nil {
			return nil, fmt.Errorf("connection closed mid-frame: %w", err)
		}
	}
	out := w.buf[:n]
	w.buf = w.buf[n:]
	return out, nil
}

func (w *edgeWS) sendFrame(opcode byte, payload []byte) error {
	var mask [4]byte
	if _, err := rand.Read(mask[:]); err != nil {
		return err
	}
	header := []byte{0x80 | opcode}
	ln := len(payload)
	switch {
	case ln < 126:
		header = append(header, 0x80|byte(ln))
	case ln < 65536:
		header = append(header, 0x80|126, byte(ln>>8), byte(ln))
	default:
		var ext [8]byte
		binary.BigEndian.PutUint64(ext[:], uint64(ln))
		header = append(header, 0x80|127)
		header = append(header, ext[:]...)
	}
	masked := make([]byte, ln)
	for i, b := range payload {
		masked[i] = b ^ mask[i%4]
	}
	frame := append(header, mask[:]...)
	frame = append(frame, masked...)
	_, err := w.conn.Write(frame)
	return err
}

func (w *edgeWS) sendText(text string) error {
	return w.sendFrame(0x1, []byte(text))
}

// recvMessage returns kind ("text", "binary" or "close") and the payload.
func (w *edgeWS) recvMessage() (string, []byte, error) {
	for {
		hdr, err := w.recvExact(2)
		if err != nil {
			return "", nil, err
		}
		b1, b2 := hdr[0], hdr[1]
		opcode := b1 & 0x0F
		length := int64(b2 & 0x7F)
		switch length {
		case 126:
			ext, err := w.recvExact(2)
			if err != nil {
				return "", nil, err
			}
			length = int64(binary.BigEndian.Uint16(ext))
		case 127:
			ext, err := w.recvExact(8)
			if err != nil {
				return "", nil, err
			}
			length = int64(binary.BigEndian.Uint64(ext))
		}
		if length < 0 || length > 32<<20 {
			return "", nil, fmt.Errorf("absurd frame length %d", length)
		}
		var mask []byte
		if b2&0x80 != 0 {
			mask, err = w.recvExact(4)
			if err != nil {
				return "", nil, err
			}
		}
		payload, err := w.recvExact(int(length))
		if err != nil {
			return "", nil, err
		}
		if mask != nil {
			for i := range payload {
				payload[i] ^= mask[i%4]
			}
		}
		switch opcode {
		case 0x8:
			return "close", payload, nil
		case 0x9: // ping -> pong
			if err := w.sendFrame(0xA, payload); err != nil {
				return "", nil, err
			}
			continue
		case 0xA: // pong
			continue
		case 0x1:
			return "text", payload, nil
		default: // 0x2 binary or continuation
			return "binary", payload, nil
		}
	}
}

// close sends a close frame (best effort) and closes the connection.
func (w *edgeWS) close() {
	if w.conn == nil {
		return
	}
	_ = w.sendFrame(0x8, nil)
	w.conn.Close()
	w.conn = nil
}

// forceClose closes the connection immediately (used on ctx cancel).
func (w *edgeWS) forceClose() {
	if w.conn == nil {
		return
	}
	w.conn.Close()
	w.conn = nil
}

// ---------------------------------------------------------------------------

// extractEdgeAudio strips the "Path:audio" header from a binary Edge message.
func extractEdgeAudio(frame []byte) []byte {
	if !bytes.HasPrefix(frame, []byte("Path:audio")) {
		return nil
	}
	if idx := bytes.Index(frame, []byte("\r\n\r\n")); idx >= 0 {
		return frame[idx+4:]
	}
	if idx := bytes.Index(frame, []byte("\r\n")); idx >= 0 {
		return frame[idx+2:]
	}
	return nil
}

var xmlEscaper = strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;")

func newUUID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	b[6] = (b[6] & 0x0f) | 0x40 // version 4
	b[8] = (b[8] & 0x3f) | 0x80 // variant
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// edgeSynthesize synthesizes Vietnamese text -> MP3 bytes (24kHz mono).
func edgeSynthesize(ctx context.Context, text, voice, rate, pitch string) ([]byte, error) {
	voiceName := edgeVoices[voice]
	if voiceName == "" {
		voiceName = edgeVoices["default"]
	}
	ws := &edgeWS{}
	if err := ws.connect(ctx); err != nil {
		return nil, err
	}
	defer ws.close()
	stop := context.AfterFunc(ctx, ws.forceClose)
	defer stop()

	if err := ws.sendText("Content-Type: application/json; charset=utf-8\r\n" +
		"Path: speech.config\r\n\r\n" +
		`{"context":{"synthesis":{"audio":{"metadataoptions":` +
		`{"sentenceBoundaryEnabled":"false",` +
		`"wordBoundaryEnabled":"false"},` +
		`"outputFormat":"audio-24khz-48kbitrate-mono-mp3"}}}}`); err != nil {
		return nil, err
	}
	ssml := "<speak version='1.0' " +
		"xmlns='http://www.w3.org/2001/10/synthesis' xml:lang='vi-VN'>" +
		"<voice name='" + voiceName + "'>" +
		"<prosody rate='" + rate + "' pitch='" + pitch + "'>" +
		xmlEscaper.Replace(text) + "</prosody></voice></speak>"
	if err := ws.sendText("X-RequestId: " + newUUID() + "\r\n" +
		"Content-Type: application/ssml+xml\r\n" +
		"Path: ssml\r\n\r\n" + ssml); err != nil {
		return nil, err
	}

	var audio bytes.Buffer
	for {
		kind, data, err := ws.recvMessage()
		if err != nil {
			return nil, err
		}
		switch kind {
		case "binary":
			if chunk := extractEdgeAudio(data); chunk != nil {
				audio.Write(chunk)
			}
		case "text":
			s := string(data)
			switch {
			case strings.Contains(s, "Path:turn.end"):
				goto done
			case strings.Contains(s, "Path:turn.error"):
				return nil, fmt.Errorf("Edge TTS error: %.200s", s)
			}
		case "close":
			goto done
		}
	}
done:
	if audio.Len() == 0 {
		return nil, fmt.Errorf("no audio received from Edge TTS")
	}
	return audio.Bytes(), nil
}
