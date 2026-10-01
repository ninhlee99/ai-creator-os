"""Tests for TTS providers: protocol units (no network) + chain behavior."""
import socket
import struct
import sys
import threading

sys.path.insert(0, ".")

from engines.tts import edge_tts
from engines.tts.provider import (EdgeTTS, GeminiTTS, LocalTTS, build_tts,
                                  pcm_to_wav)


def test_extract_audio_double_crlf():
    frame = b"Path:audio\r\nContent-Type:audio/mpeg\r\n\r\nMP3DATA"
    assert edge_tts._extract_audio(frame) == b"MP3DATA"


def test_extract_audio_single_header():
    frame = b"Path:audio\r\nMP3DATA"
    assert edge_tts._extract_audio(frame) == b"MP3DATA"


def test_extract_audio_rejects_non_audio():
    assert edge_tts._extract_audio(b"Path:turn.end\r\n{}") is None


def test_ws_frame_roundtrip_over_socketpair():
    """Client masking + server frame parsing, without TLS."""
    a, b = socket.socketpair()
    try:
        client = edge_tts._WS()
        client.sock = a
        client._buf = b""

        def server():
            # read one masked text frame, reply with unmasked binary frame
            hdr = b.recv(2)
            ln = hdr[1] & 0x7F
            mask = b.recv(4)
            payload = b.recv(ln)
            text = bytes(x ^ mask[i % 4] for i, x in enumerate(payload))
            assert text == b"hello-edge"
            body = b"Path:audio\r\nAUDIOBYTES"
            b.sendall(bytes([0x82, len(body)]) + body)
            b.close()

        t = threading.Thread(target=server)
        t.start()
        client.send_text("hello-edge")
        kind, data = client.recv_message()
        t.join()
        assert kind == "binary"
        assert edge_tts._extract_audio(data) == b"AUDIOBYTES"
    finally:
        a.close()


def test_pcm_to_wav_header():
    pcm = b"\x00\x01" * 24000  # 1s of 24kHz s16 mono
    wav = pcm_to_wav(pcm)
    assert wav[:4] == b"RIFF" and wav[8:12] == b"WAVE"
    assert struct.unpack("<I", wav[24:28])[0] == 24000  # sample rate
    assert len(wav) == 44 + len(pcm)


def test_gemini_no_key_fails_fast():
    r = GeminiTTS(api_key="").run("xin chào")
    assert not r.ok and "key" in r.error.lower()


def test_edge_no_network_reports_cleanly():
    # monkeypatch synthesize to simulate blocked network
    orig = edge_tts.synthesize
    edge_tts.synthesize = lambda *a, **k: (_ for _ in ()).throw(
        RuntimeError("simulated block"))
    try:
        r = EdgeTTS().run("xin chào")
        assert not r.ok and "simulated block" in r.error
    finally:
        edge_tts.synthesize = orig


def test_chain_skips_failed_providers():
    chain = build_tts(api_key="")  # gemini: no key, local: not wired
    assert [p.name for p in chain.providers] == [
        "tts-gemini", "tts-local-vieneu", "tts-edge"]
    # edge will fail in sandbox (403) or succeed on user machine;
    # either way the chain must not raise.
    r = chain.run("test")
    assert isinstance(r.ok, bool)


def test_voice_maps_cover_personas():
    for style in ("warm-female", "clear-teacher", "singer",
                  "energetic-caster", "upbeat-host"):
        assert style in edge_tts.VI_VOICES
