"""Publisher factory: which platforms are live for an account."""
from __future__ import annotations

import sys
sys.path.insert(0, ".")

from .base import Publisher
from .tiktok import TikTokPublisher
from .facebook import FacebookPublisher
from .youtube import YouTubePublisher


def build_publishers(account, kinds: tuple | None = None) -> list[Publisher]:
    """Return configured publishers for an account, each already filtered
    to the content kinds it handles. Unconfigured platforms are skipped
    silently — the ledger records what actually published."""
    pubs: list[Publisher] = [
        TikTokPublisher(account.username),
        FacebookPublisher(account.username),
        YouTubePublisher(account.username,
                         allowed_kinds=account.youtube_content_types,
                         channel=account.youtube_channel),
    ]
    out = [p for p in pubs if p.is_configured()]
    if kinds:
        out = [p for p in out
               if any(p.handles(k) for k in kinds)]
    return out
