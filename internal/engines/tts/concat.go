package tts

import (
	"encoding/binary"
	"fmt"
)

// wavDataOffset scans a WAV file for the "data" chunk and returns the byte
// offset where the raw PCM payload starts.
func wavDataOffset(wav []byte) (int, error) {
	if len(wav) < 44 {
		return 0, fmt.Errorf("too short for WAV header (%d bytes)", len(wav))
	}
	if string(wav[0:4]) != "RIFF" || string(wav[8:12]) != "WAVE" {
		return 0, fmt.Errorf("not a WAV file")
	}
	off := 12
	for off+8 <= len(wav) {
		id := string(wav[off : off+4])
		size := int(binary.LittleEndian.Uint32(wav[off+4 : off+8]))
		if id == "data" {
			return off + 8, nil
		}
		off += 8 + size
		if size%2 == 1 {
			off++ // chunk padding
		}
	}
	return 0, fmt.Errorf("data chunk not found")
}

// ConcatWavs joins WAV chunks end to end, preserving order. The input
// contract is the TTS chain output contract: PCM s16le, 24 kHz, mono.
// Every chunk must share the same audio parameters; a mismatch fails loudly
// instead of producing a corrupt file. Used by the film pipeline to stitch
// per-sentence TTS chunks into one narration track (P0-6).
func ConcatWavs(chunks [][]byte) ([]byte, error) {
	if len(chunks) == 0 {
		return nil, fmt.Errorf("no WAV chunks")
	}
	var pcm []byte
	var rate, ch, bits int
	for i, w := range chunks {
		r, c, b, err := wavParams(w)
		if err != nil {
			return nil, fmt.Errorf("chunk %d: %w", i+1, err)
		}
		if i == 0 {
			rate, ch, bits = r, c, b
			if rate != 24000 || ch != 1 || bits != 16 {
				return nil, fmt.Errorf("chunk 1: want PCM s16le 24kHz mono, got %dHz/%dch/%dbit", rate, ch, bits)
			}
		} else if r != rate || c != ch || b != bits {
			return nil, fmt.Errorf("chunk %d: params %dHz/%dch/%dbit differ from first chunk", i+1, r, c, b)
		}
		off, err := wavDataOffset(w)
		if err != nil {
			return nil, fmt.Errorf("chunk %d: %w", i+1, err)
		}
		pcm = append(pcm, w[off:]...)
	}
	return pcmToWav(pcm, rate), nil
}
