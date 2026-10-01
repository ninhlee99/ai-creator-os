"""Edge TTS (Microsoft Edge read-aloud endpoint) — no API key needed.

Unofficial endpoint: free, but ToS-gray and rate limits are unpublished.
Position in the chain: LAST resort (see docs/RESEARCH/tts_avatar.md).

Implemented on stdlib only: raw socket + minimal WebSocket framing
(text/binary frames, client masking, ping/pong). Not a general WS client.
"""
from __future__ import annotations

import base64
import hashlib
import os
import socket
import ssl
import struct
import uuid
import xml.sax.saxutils as saxutils

_HOST = "speech.platform.bing.com"
_PATH = ("/consumer/speech/synthesize/readaloud/edge/v1"
         "?TrustedClientToken=6A5AA1D4EAFF4E9FB37E23D68491D6F4")
_UA = ("Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) "
       "AppleWebKit/537.36 (KHTML, like Gecko) "
       "Chrome/127.0.0.0 Safari/537.36 Edg/127.0.0.0")
_ORIGIN = "chrome-extension://jdiccldimpdaibmpdkjnbmckianbfoldn"

# persona voice style -> Edge neural voice
VI_VOICES = {
    "default": "vi-VN-HoaiMyNeural",
    "warm-female": "vi-VN-HoaiMyNeural",
    "clear-teacher": "vi-VN-HoaiMyNeural",
    "singer": "vi-VN-HoaiMyNeural",
    "upbeat-host": "vi-VN-HoaiMyNeural",
    "energetic-caster": "vi-VN-NamMinhNeural",
    "male": "vi-VN-NamMinhNeural",
}


def _proxy_from_env() -> tuple[str, int, str | None] | None:
    """Return (host, port, auth_header) from HTTPS_PROXY env, or None."""
    for var in ("HTTPS_PROXY", "https_proxy"):
        val = os.environ.get(var)
        if not val:
            continue
        # http://user:pass@host:port  (credentials never logged)
        rest = val.split("://", 1)[-1]
        auth = None
        if "@" in rest:
            creds, rest = rest.rsplit("@", 1)
            auth = "Basic " + base64.b64encode(creds.encode()).decode()
        host, _, port = rest.partition(":")
        return (host, int(port or 3128), auth)
    return None


class _WS:
    """Minimal WebSocket client: handshake, masked text frames, recv loop."""

    def __init__(self, timeout: int = 30):
        self._timeout = timeout
        self.sock: socket.socket | None = None
        self._buf = b""

    def _tunnel(self) -> socket.socket:
        """Direct TCP, or HTTP CONNECT through the env proxy."""
        proxy = _proxy_from_env()
        if proxy is None:
            return socket.create_connection((_HOST, 443), timeout=20)
        phost, pport, auth = proxy
        sock = socket.create_connection((phost, pport), timeout=20)
        req = (f"CONNECT {_HOST}:443 HTTP/1.1\r\nHost: {_HOST}:443\r\n")
        if auth:
            req += f"Proxy-Authorization: {auth}\r\n"
        sock.sendall((req + "\r\n").encode())
        resp = b""
        while b"\r\n\r\n" not in resp:
            chunk = sock.recv(4096)
            if not chunk:
                raise RuntimeError("proxy closed CONNECT")
            resp += chunk
        if b" 200 " not in resp.split(b"\r\n", 1)[0]:
            raise RuntimeError(
                f"proxy CONNECT failed: {resp[:80]!r}")
        return sock

    def connect(self) -> None:
        raw = self._tunnel()
        ctx = ssl.create_default_context()
        self.sock = ctx.wrap_socket(raw, server_hostname=_HOST)
        self.sock.settimeout(self._timeout)
        key = base64.b64encode(os.urandom(16)).decode()
        req = (
            f"GET {_PATH} HTTP/1.1\r\n"
            f"Host: {_HOST}\r\n"
            "Upgrade: websocket\r\n"
            "Connection: Upgrade\r\n"
            f"Sec-WebSocket-Key: {key}\r\n"
            "Sec-WebSocket-Version: 13\r\n"
            f"Origin: {_ORIGIN}\r\n"
            f"User-Agent: {_UA}\r\n"
            "Pragma: no-cache\r\n"
            "Cache-Control: no-cache\r\n"
            "\r\n"
        )
        self.sock.sendall(req.encode())
        head = self._read_http_head()
        status = head.split("\r\n", 1)[0]
        if "101" not in status:
            raise RuntimeError(f"WS handshake failed: {status[:120]}")

    def _read_http_head(self) -> str:
        data = b""
        while b"\r\n\r\n" not in data:
            chunk = self.sock.recv(4096)
            if not chunk:
                break
            data += chunk
        head, _, rest = data.partition(b"\r\n\r\n")
        self._buf = rest
        return head.decode("latin1")

    def _recv_exact(self, n: int) -> bytes:
        while len(self._buf) < n:
            chunk = self.sock.recv(65536)
            if not chunk:
                raise RuntimeError("connection closed mid-frame")
            self._buf += chunk
        out, self._buf = self._buf[:n], self._buf[n:]
        return out

    def _send_frame(self, opcode: int, payload: bytes) -> None:
        mask = os.urandom(4)
        header = bytes([0x80 | opcode])
        ln = len(payload)
        if ln < 126:
            header += bytes([0x80 | ln])
        elif ln < 65536:
            header += bytes([0x80 | 126]) + struct.pack(">H", ln)
        else:
            header += bytes([0x80 | 127]) + struct.pack(">Q", ln)
        masked = bytes(b ^ mask[i % 4] for i, b in enumerate(payload))
        self.sock.sendall(header + mask + masked)

    def send_text(self, text: str) -> None:
        self._send_frame(0x1, text.encode("utf-8"))

    def recv_message(self) -> tuple[str, bytes | str]:
        """Returns (kind, data); kind in {text, binary, close}."""
        while True:
            b1, b2 = self._recv_exact(2)
            opcode = b1 & 0x0F
            ln = b2 & 0x7F
            if ln == 126:
                ln = struct.unpack(">H", self._recv_exact(2))[0]
            elif ln == 127:
                ln = struct.unpack(">Q", self._recv_exact(8))[0]
            mask = self._recv_exact(4) if (b2 & 0x80) else None
            payload = self._recv_exact(ln)
            if mask:
                payload = bytes(b ^ mask[i % 4]
                                for i, b in enumerate(payload))
            if opcode == 0x8:
                return ("close", payload)
            if opcode == 0x9:  # ping -> pong
                self._send_frame(0xA, payload)
                continue
            if opcode == 0xA:
                continue
            if opcode == 0x1:
                return ("text", payload.decode("utf-8", "replace"))
            return ("binary", payload)  # 0x2 or continuation

    def close(self) -> None:
        try:
            self._send_frame(0x8, b"")
        except Exception:
            pass
        try:
            self.sock.close()
        except Exception:
            pass


def _extract_audio(frame: bytes) -> bytes | None:
    """Strip the Path:audio header from a binary Edge message."""
    if not frame.startswith(b"Path:audio"):
        return None
    idx = frame.find(b"\r\n\r\n")
    if idx != -1:
        return frame[idx + 4:]
    idx = frame.find(b"\r\n")
    return frame[idx + 2:] if idx != -1 else None


def synthesize(text: str, voice: str = "default",
               rate: str = "+0%", pitch: str = "+0Hz",
               timeout: int = 30) -> bytes:
    """Synthesize Vietnamese text -> MP3 bytes (24kHz mono)."""
    if not text or not text.strip():
        raise ValueError("empty text")
    voice_name = VI_VOICES.get(voice, VI_VOICES["default"])
    ws = _WS(timeout=timeout)
    ws.connect()
    try:
        ws.send_text(
            "Content-Type: application/json; charset=utf-8\r\n"
            "Path: speech.config\r\n\r\n"
            '{"context":{"synthesis":{"audio":{"metadataoptions":'
            '{"sentenceBoundaryEnabled":"false",'
            '"wordBoundaryEnabled":"false"},'
            '"outputFormat":"audio-24khz-48kbitrate-mono-mp3"}}}}'
        )
        ssml = (
            "<speak version='1.0' "
            "xmlns='http://www.w3.org/2001/10/synthesis' xml:lang='vi-VN'>"
            f"<voice name='{voice_name}'>"
            f"<prosody rate='{rate}' pitch='{pitch}'>"
            f"{saxutils.escape(text)}</prosody></voice></speak>"
        )
        ws.send_text(
            f"X-RequestId: {uuid.uuid4()}\r\n"
            "Content-Type: application/ssml+xml\r\n"
            "Path: ssml\r\n\r\n" + ssml
        )
        audio = bytearray()
        while True:
            kind, data = ws.recv_message()
            if kind == "binary":
                chunk = _extract_audio(data)
                if chunk:
                    audio += chunk
            elif kind == "text":
                if "Path:turn.end" in data:
                    break
                if "Path:turn.error" in data:
                    raise RuntimeError(f"Edge TTS error: {data[:200]}")
    finally:
        ws.close()
    if not audio:
        raise RuntimeError("no audio received from Edge TTS")
    return bytes(audio)
