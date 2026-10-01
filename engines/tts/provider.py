"""TTS engine: Gemini TTS (free tier) -> VieNeu local -> Edge TTS fallback.

Chain order per docs/RESEARCH/tts_avatar.md:
1. Gemini TTS — most expressive (style prompting), free tier via AI Studio key
2. VieNeu-TTS v3 local — offline, no quota, native Vietnamese (needs model dl)
3. Edge TTS — no key, but unofficial endpoint (ToS-gray); last resort only

All providers return WAV bytes (PCM s16le, 24kHz, mono) so the streamer and
avatar lip-sync get one stable format.
"""
from __future__ import annotations

import base64
import json
import shutil
import struct
import subprocess
import tempfile
import urllib.request
from abc import abstractmethod

from ..base import Engine, EngineResult
from . import edge_tts

# persona voice style -> provider voice
GEMINI_VOICES = {
    "default": "Kore",
    "warm-female": "Aoede",      # storyteller: warm, expressive
    "clear-teacher": "Kore",     # teacher: clear, steady
    "singer": "Aoede",
    "upbeat-host": "Puck",
    "energetic-caster": "Puck",  # gamer: high energy
    "male": "Charon",
}

# style keyword -> prompt prefix (Gemini TTS supports style prompting)
STYLE_PREFIX = {
    "cheerful": "Nói một cách vui vẻ, rạng rỡ: ",
    "warm": "Nói một cách ấm áp, gần gũi: ",
    "suspense": "Kể một cách hồi hộp, lôi cuốn: ",
    "calm": "Nói một cách chậm rãi, rõ ràng: ",
    "energetic": "Nói một cách đầy năng lượng, hào hứng: ",
}


def pcm_to_wav(pcm: bytes, sample_rate: int = 24000) -> bytes:
    n = len(pcm)
    header = struct.pack(
        "<4sI4s4sIHHIIHH4sI",
        b"RIFF", 36 + n, b"WAVE", b"fmt ", 16, 1, 1, sample_rate,
        sample_rate * 2, 2, 16, b"data", n)
    return header + pcm


class TTSProvider(Engine):
    name = "tts"

    @abstractmethod
    def run(self, text: str, voice: str = "default",
            **kw) -> EngineResult:
        """Return WAV bytes (PCM s16le 24kHz mono)."""
        ...


class GeminiTTS(TTSProvider):
    """Gemini TTS via AI Studio generateContent (responseModalities AUDIO).

    Endpoint verified 2026-10-01. Output is base64 PCM 24kHz -> wrapped WAV.
    """

    name = "tts-gemini"

    def __init__(self, api_key: str,
                 model: str = "gemini-2.5-flash-preview-tts"):
        self.api_key = api_key
        self.model = model

    def run(self, text: str, voice: str = "default",
            style: str = "", **kw) -> EngineResult:
        if not self.api_key:
            return EngineResult(False, self.name, error="no API key")
        if not text or not text.strip():
            return EngineResult(False, self.name, error="empty text")
        voice_name = GEMINI_VOICES.get(voice, GEMINI_VOICES["default"])
        prompt = STYLE_PREFIX.get(style, "") + text.strip()
        url = (f"https://generativelanguage.googleapis.com/v1beta/models/"
               f"{self.model}:generateContent?key={self.api_key}")
        body = {
            "contents": [{"parts": [{"text": prompt}]}],
            "generationConfig": {
                "responseModalities": ["AUDIO"],
                "speechConfig": {"voiceConfig": {"prebuiltVoiceConfig": {
                    "voiceName": voice_name}}},
            },
        }
        req = urllib.request.Request(
            url, data=json.dumps(body).encode(),
            headers={"Content-Type": "application/json"}, method="POST")
        try:
            with urllib.request.urlopen(req, timeout=60) as r:
                data = json.load(r)
            for part in data["candidates"][0]["content"]["parts"]:
                inline = part.get("inlineData") or part.get("inline_data")
                if inline and inline.get("data"):
                    pcm = base64.b64decode(inline["data"])
                    return EngineResult(True, self.name,
                                        payload=pcm_to_wav(pcm))
            return EngineResult(False, self.name,
                                error="no audio in response")
        except Exception as e:  # noqa: BLE001
            return EngineResult(False, self.name, error=str(e)[:200])


class LocalTTS(TTSProvider):
    """VieNeu-TTS v3 local (Apache-2.0). Best free Vietnamese voice, offline.

    Not wired yet: needs the model downloaded + its python env on the Mac.
    See docs/RESEARCH/tts_avatar.md. Glue contract: stdin text -> stdout WAV.
    """

    name = "tts-local-vieneu"

    def __init__(self, cli: str = ""):
        self.cli = cli  # path to vieneu infer script, when installed

    def run(self, text: str, voice: str = "default",
            **kw) -> EngineResult:
        if not self.cli:
            return EngineResult(False, self.name,
                                error="VieNeu not installed "
                                      "(see docs/RESEARCH/tts_avatar.md)")
        return EngineResult(False, self.name,
                            error="VieNeu glue not implemented yet")


class EdgeTTS(TTSProvider):
    """Edge read-aloud endpoint. No key. LAST resort (unofficial, ToS-gray).

    MP3 output is normalized to WAV via ffmpeg (required by stream-engine).
    """

    name = "tts-edge"

    def run(self, text: str, voice: str = "default",
            **kw) -> EngineResult:
        if not shutil.which("ffmpeg"):
            return EngineResult(False, self.name,
                                error="ffmpeg not found (needed for mp3->wav)")
        try:
            mp3 = edge_tts.synthesize(text, voice=voice)
        except Exception as e:  # noqa: BLE001
            return EngineResult(False, self.name, error=str(e)[:200])
        try:
            with tempfile.NamedTemporaryFile(
                    suffix=".mp3", delete=False) as f:
                f.write(mp3)
                mp3_path = f.name
            wav_path = mp3_path + ".wav"
            subprocess.run(
                ["ffmpeg", "-v", "error", "-y", "-i", mp3_path,
                 "-ar", "24000", "-ac", "1", "-c:a", "pcm_s16le", wav_path],
                check=True, timeout=60)
            with open(wav_path, "rb") as f:
                wav = f.read()
            return EngineResult(True, self.name, payload=wav)
        except Exception as e:  # noqa: BLE001
            return EngineResult(False, self.name, error=str(e)[:200])


def build_tts(api_key: str = "",
              gemini_model: str = "gemini-2.5-flash-preview-tts",
              vieneu_cli: str = "", meter=None):
    from ..base import Chain
    return Chain([GeminiTTS(api_key, gemini_model),
                  LocalTTS(vieneu_cli),
                  EdgeTTS()], meter=meter)
