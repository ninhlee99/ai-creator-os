"""Avatar engine: local stylized realtime -> paid streaming API.

HONEST CONSTRAINT (see docs/ARCHITECTURE.md §5):
photorealistic + realtime + frame-coherent + free is not production-ready
today. v1 ships a high-quality STYLIZED realtime avatar with viseme-accurate
lip-sync driven by TTS timing. The interface is swappable: a paid streaming
avatar API plugs in later with zero changes to show logic.

v1 "local-stylized" renders a consistent 2D/3D character to a video device
or frame pipe consumed by the stream engine. Lip-sync uses phoneme/viseme
timing from the TTS engine, never guessing.
"""
from __future__ import annotations

from abc import abstractmethod
from dataclasses import dataclass

from ..base import Engine, EngineResult


@dataclass
class VisemeFrame:
    at_ms: int
    viseme: str       # e.g. "A", "E", "MBP", "O", "U", "rest"
    intensity: float  # 0..1


class AvatarProvider(Engine):
    name = "avatar"

    @abstractmethod
    def speak(self, audio: bytes, visemes: list[VisemeFrame],
              **kw) -> EngineResult:
        """Render avatar speaking the audio; returns frame pipe / device."""
        ...


class LocalStylizedAvatar(AvatarProvider):
    name = "avatar-local-stylized"

    def __init__(self, character: str = "default"):
        self.character = character

    def speak(self, audio: bytes, visemes: list[VisemeFrame],
              **kw) -> EngineResult:
        # TODO: wire renderer (see docs/RESEARCH/tts_avatar.md).
        # Contract: consumes viseme timeline, outputs coherent frames.
        return EngineResult(False, self.name,
                            error="avatar renderer not wired yet")


class StreamingAPIAvatar(AvatarProvider):
    """Paid streaming avatar API (HeyGen/D-ID class). Opt-in, capped."""
    name = "avatar-streaming-api"

    def __init__(self, provider: str, api_key: str):
        self.provider = provider
        self.api_key = api_key

    def speak(self, audio: bytes, visemes: list[VisemeFrame],
              **kw) -> EngineResult:
        if not self.api_key:
            return EngineResult(False, self.name, error="no API key")
        return EngineResult(False, self.name,
                            error=f"provider '{self.provider}' not wired yet")


def build_avatar(provider: str, api_key: str = "", character: str = "default",
                 meter=None):
    from ..base import Chain
    if provider == "streaming-api":
        return Chain([StreamingAPIAvatar(provider, api_key),
                      LocalStylizedAvatar(character)], meter=meter)
    return Chain([LocalStylizedAvatar(character)], meter=meter)
