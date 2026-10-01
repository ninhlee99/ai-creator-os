"""Multi-platform publisher abstraction.

One rendered video can go to TikTok, Facebook (Page) and YouTube — each
account decides which platforms are wired and, for YouTube, which content
kinds its channel accepts (e.g. a music channel takes ai_music + ai_remix,
a story channel takes short_film).

Draft-first everywhere it is supported: nothing goes public without the
platform's own review surface or an explicit env flip.
"""
from __future__ import annotations

from abc import ABC, abstractmethod
from dataclasses import dataclass
from pathlib import Path

# Content kinds the factory can produce. A publisher declares which of
# these it handles; YouTube additionally filters per account via
# account.youtube_content_types.
CONTENT_KINDS = ("short_video", "short_film", "ai_music", "ai_remix")


@dataclass
class PublishResult:
    ok: bool
    platform: str
    remote_id: str | None = None
    url: str | None = None
    error: str | None = None
    draft: bool = False


class Publisher(ABC):
    name: str = "base"
    kinds: tuple = ()

    @abstractmethod
    def is_configured(self) -> bool:
        """True when credentials for this platform exist."""

    def handles(self, kind: str) -> bool:
        return kind in self.kinds

    @abstractmethod
    def publish(self, video_path: str, title: str, description: str,
                kind: str, **kw) -> PublishResult:
        """Upload + (draft-)publish. Never raises for expected API
        failures — returns PublishResult(ok=False) instead."""


def _env_for(username: str, *names: str) -> str | None:
    """Per-account env override first (NAME_<USERNAME>), then shared NAME."""
    import os
    uname = "".join(c if c.isalnum() else "_" for c in username).upper()
    for n in names:
        v = os.environ.get(f"{n}_{uname}") or os.environ.get(n)
        if v:
            return v
    return None
