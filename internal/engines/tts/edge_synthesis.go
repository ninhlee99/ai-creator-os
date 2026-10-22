package tts

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"strings"
)

func extractEdgeAudio(frame []byte) []byte {
	if len(frame) < 2 {
		return nil
	}
	headerLen := int(binary.BigEndian.Uint16(frame[:2]))
	if headerLen > 0 && len(frame) >= 2+headerLen {
		header := frame[2 : 2+headerLen]
		if bytes.Contains(header, []byte("Path:audio")) {
			return frame[2+headerLen:]
		}
	}
	// Fallback for raw text-prefixed frames
	if idx := bytes.Index(frame, []byte("Path:audio")); idx >= 0 {
		if crlf := bytes.Index(frame[idx:], []byte("\r\n\r\n")); crlf >= 0 {
			return frame[idx+crlf+4:]
		}
		if crlf := bytes.Index(frame[idx:], []byte("\r\n")); crlf >= 0 {
			return frame[idx+crlf+2:]
		}
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
