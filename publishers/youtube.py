"""YouTube publisher: Data API v3 resumable upload, per-account channel.

Each TikTok account may own a YouTube channel (account.youtube_channel)
and declares which content kinds that channel accepts via
account.youtube_content_types, e.g.:

    storyteller account -> ["short_film"]            (story films)
    musician account    -> ["ai_music", "ai_remix"]  (AI songs + remixes)

Credentials: one OAuth client (YOUTUBE_CLIENT_ID / YOUTUBE_CLIENT_SECRET),
one refresh-token file per account: youtube_token_<username>.json
(gitignored, never logged). Scope: youtube.upload.

Default privacy is "private" — flip with YOUTUBE_DEFAULT_PRIVACY=public
(or unlisted) once the channel is reviewed. Quota note: videos.insert
costs ~1600 units against the 10k/day default quota, so a handful of
uploads per day per project is the free-tier reality.
"""
from __future__ import annotations

import json
import os
import sys
import urllib.parse
import urllib.request
from pathlib import Path
sys.path.insert(0, ".")

from .base import CONTENT_KINDS, PublishResult, Publisher, _env_for

_OAUTH_TOKEN_URL = "https://oauth2.googleapis.com/token"
_UPLOAD_URL = ("https://www.googleapis.com/upload/youtube/v3/videos"
               "?uploadType=resumable&part=snippet,status")

# persona-ish category mapping for uploads
CATEGORY_BY_KIND = {
    "short_film": "24",    # Entertainment
    "ai_music": "10",      # Music
    "ai_remix": "10",      # Music
    "short_video": "22",   # People & Blogs
}


class YouTubePublisher(Publisher):
    name = "youtube"
    kinds = CONTENT_KINDS  # further filtered per account below

    def __init__(self, username: str,
                 allowed_kinds: list | None = None,
                 channel: str | None = None):
        self.username = username
        self.allowed_kinds = tuple(allowed_kinds or ())
        self.channel = channel
        safe = "".join(c if c.isalnum() else "_" for c in username)
        self.token_path = Path(f"youtube_token_{safe}.json")
        self.client_id = _env_for(username, "YOUTUBE_CLIENT_ID")
        self.client_secret = _env_for(username, "YOUTUBE_CLIENT_SECRET")
        self.privacy = os.environ.get("YOUTUBE_DEFAULT_PRIVACY", "private")

    # ---- config ----
    def is_configured(self) -> bool:
        return bool(self.client_id and self.client_secret
                    and self.token_path.exists()
                    and self.allowed_kinds)

    def handles(self, kind: str) -> bool:
        return kind in self.allowed_kinds

    def _refresh_token(self) -> str | None:
        try:
            data = json.loads(self.token_path.read_text())
        except Exception:
            return None
        return data.get("refresh_token")

    # ---- http ----
    def _http(self, method: str, url: str, headers: dict,
              body: bytes | None = None) -> tuple[int, dict, dict]:
        req = urllib.request.Request(url, data=body, headers=headers,
                                     method=method)
        try:
            with urllib.request.urlopen(req, timeout=120) as r:
                raw = r.read().decode("utf-8", "replace")
                return r.status, dict(r.headers), (
                    json.loads(raw) if raw else {})
        except urllib.error.HTTPError as e:
            raw = e.read().decode("utf-8", "replace")
            try:
                payload = json.loads(raw) if raw else {}
            except Exception:
                payload = {"raw": raw[:300]}
            return e.code, dict(e.headers), payload

    def _access_token(self) -> str:
        refresh = self._refresh_token()
        if not refresh:
            raise RuntimeError("no youtube refresh token stored")
        body = urllib.parse.urlencode({
            "client_id": self.client_id,
            "client_secret": self.client_secret,
            "refresh_token": refresh,
            "grant_type": "refresh_token",
        }).encode()
        status, _, data = self._http(
            "POST", _OAUTH_TOKEN_URL,
            {"Content-Type": "application/x-www-form-urlencoded"}, body)
        if status != 200 or not data.get("access_token"):
            raise RuntimeError(
                f"oauth refresh failed: HTTP {status} {str(data)[:200]}")
        return data["access_token"]

    # ---- publish ----
    def publish(self, video_path: str, title: str, description: str,
                kind: str, **kw) -> PublishResult:
        if not self.is_configured():
            return PublishResult(False, self.name,
                                 error="youtube not configured")
        if not self.handles(kind):
            return PublishResult(False, self.name,
                                 error=f"kind {kind} not enabled for "
                                       f"this channel")
        try:
            size = os.path.getsize(video_path)
            token = self._access_token()
            meta = {
                "snippet": {
                    "title": (title or "AI video")[:100],
                    "description": (description or "")[:5000],
                    "categoryId": CATEGORY_BY_KIND.get(kind, "22"),
                },
                "status": {"privacyStatus": self.privacy,
                           "selfDeclaredMadeForKids": False},
            }
            status, headers, data = self._http(
                "POST", _UPLOAD_URL,
                {"Authorization": f"Bearer {token}",
                 "Content-Type": "application/json; charset=UTF-8",
                 "X-Upload-Content-Length": str(size),
                 "X-Upload-Content-Type": "video/mp4"},
                json.dumps(meta).encode())
            session = headers.get("Location") or headers.get("location")
            if status != 200 or not session:
                return PublishResult(False, self.name,
                                     error=f"upload init failed: HTTP "
                                           f"{status} {str(data)[:200]}")
            with open(video_path, "rb") as f:
                blob = f.read()
            status, _, data = self._http(
                "PUT", session,
                {"Content-Length": str(size),
                 "Content-Range": f"bytes 0-{size - 1}/{size}"},
                blob)
            if status not in (200, 201):
                return PublishResult(False, self.name,
                                     error=f"upload failed: HTTP {status} "
                                           f"{str(data)[:200]}")
            vid = data.get("id")
            url = f"https://youtu.be/{vid}" if vid else None
            return PublishResult(True, self.name, remote_id=vid, url=url,
                                 draft=(self.privacy != "public"))
        except Exception as e:
            return PublishResult(False, self.name, error=str(e)[:300])
