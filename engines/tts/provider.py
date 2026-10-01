"""TTS engine: free Vietnamese TTS API -> local model.

REQUIREMENTS (non-negotiable for the streamer):
- natural Vietnamese prosody: rhythm, pitch variation, stress, emotion
- streaming/low-latency synthesis (live can't wait 10s per sentence)

Provider choice is confirmed by docs/RESEARCH/tts_avatar.md. The interface
stays stable regardless of provider.
"""
from __future__ import annotations

from abc import abstractmethod

from ..base import Engine, EngineResult


class TTSProvider(Engine):
    name = "tts"

    @abstractmethod
    def run(self, text: str, voice: str = "default",
            **kw) -> EngineResult:
        """Return audio bytes (wav/mp3)."""
        ...


class FreeAPITTS(TTSProvider):
    """Free-tier Vietnamese TTS API. Concrete provider wired after research."""
    name = "tts-free-api"

    def __init__(self, provider: str, api_key: str):
        self.provider = provider
        self.api_key = api_key

    def run(self, text: str, voice: str = "default", **kw) -> EngineResult:
        if not self.api_key:
            return EngineResult(False, self.name, error="no API key")
        # TODO: wire concrete provider (see docs/RESEARCH/tts_avatar.md)
        return EngineResult(False, self.name,
                            error=f"provider '{self.provider}' not wired yet")


class LocalTTS(TTSProvider):
    """Local Vietnamese TTS fallback (e.g. Piper/vi model)."""
    name = "tts-local"

    def run(self, text: str, voice: str = "default", **kw) -> EngineResult:
        # TODO: wire local model after research
        return EngineResult(False, self.name, error="local TTS not wired yet")


def build_tts(provider: str, api_key: str, meter=None):
    from ..base import Chain
    return Chain([FreeAPITTS(provider, api_key), LocalTTS()], meter=meter)
