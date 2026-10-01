"""TikTok publisher: Content Posting API (draft-first).

Uses tiktok.posting.client.PostingClient. One OAuth token file per TikTok
account: tiktok_token_<username>.json (gitignored, never logged).
"""
from __future__ import annotations

import os
import sys
from pathlib import Path
sys.path.insert(0, ".")

from tiktok.posting.client import PostingClient, TokenStore

from .base import PublishResult, Publisher, _env_for


class TikTokPublisher(Publisher):
    name = "tiktok"
    kinds = ("short_video", "short_film")

    def __init__(self, username: str):
        self.username = username
        safe = "".join(c if c.isalnum() else "_" for c in username)
        self.token_path = Path(f"tiktok_token_{safe}.json")
        self.client_key = _env_for(username, "TIKTOK_CLIENT_KEY")
        self.client_secret = _env_for(username, "TIKTOK_CLIENT_SECRET")
        self.draft_only = os.environ.get("TIKTOK_DRAFT_ONLY", "1") == "1"
        self.privacy = os.environ.get("TIKTOK_PRIVACY", "SELF_ONLY")

    def is_configured(self) -> bool:
        return bool(self.client_key and self.client_secret
                    and self.token_path.exists())

    def _client(self) -> PostingClient:
        return PostingClient(self.client_key, self.client_secret,
                             TokenStore(str(self.token_path)))

    def publish(self, video_path: str, title: str, description: str,
                kind: str, **kw) -> PublishResult:
        if not self.is_configured():
            return PublishResult(False, self.name,
                                 error="tiktok not configured")
        try:
            client = self._client()
            publish_id = client.publish_file(
                video_path, title or description[:90],
                privacy_level=self.privacy, draft=self.draft_only)
            return PublishResult(True, self.name, remote_id=publish_id,
                                 draft=self.draft_only)
        except Exception as e:
            return PublishResult(False, self.name, error=str(e)[:300])
