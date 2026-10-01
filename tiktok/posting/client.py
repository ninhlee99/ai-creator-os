"""TikTok Content Posting API client (developers.tiktok.com).

Flow (verified from integration walkthrough, see docs/RESEARCH/tiktok_shop_api.md):
  OAuth (Login Kit + PKCE) -> token store (24h access / 365d refresh)
  -> POST /v2/post/publish/video/init/ -> PUT chunks to upload_url
  -> POST /v2/post/publish/status/fetch/ until PUBLISH_COMPLETE

Gates (human steps, not code):
  1. Developer app with Content Posting API product enabled.
  2. App audit (~1-2 wks) + Direct Post audit (~5-10 biz days) for PUBLIC posts.
     Until then: max 5 test users, SELF_ONLY visibility.
  3. Request only the scopes you use: video.publish / video.upload.

Rate limit: 6 req/min per user token on publish endpoints.
~15 posts/day per creator, shared across ALL API clients (Direct Post).

Stdlib only. Inject `http` transport for tests.
"""
from __future__ import annotations

import json
import math
import os
import time
import urllib.parse
import urllib.request

BASE = "https://open.tiktokapis.com"
AUTH_URL = "https://www.tiktok.com/v2/auth/authorize/"

SCOPES_DIRECT_POST = "video.publish"
SCOPES_UPLOAD_DRAFT = "video.upload"

_CHUNK = 10 * 1024 * 1024  # 10 MB, inside the 5-64 MB window


def _default_http(method: str, url: str, headers: dict,
                  data: bytes | None):
    req = urllib.request.Request(url, data=data, headers=headers,
                                 method=method)
    try:
        with urllib.request.urlopen(req, timeout=60) as r:
            raw = r.read()
            ctype = r.headers.get("Content-Type", "")
            return r.status, (json.loads(raw) if "json" in ctype
                              else raw)
    except urllib.error.HTTPError as e:
        body = e.read()[:500]
        raise RuntimeError(f"HTTP {e.code} {url}: {body!r}")


class TokenStore:
    """Persisted OAuth tokens. NEVER commit the file (gitignored).

    Lifecycle: access_token 24h, refresh_token 365d (rotates on refresh).
    """

    def __init__(self, path: str):
        self.path = path

    def load(self) -> dict:
        if os.path.exists(self.path):
            with open(self.path) as f:
                return json.load(f)
        return {}

    def save(self, tokens: dict) -> None:
        tokens = dict(tokens)
        tokens["obtained_at"] = int(time.time())
        os.makedirs(os.path.dirname(self.path) or ".", exist_ok=True)
        with open(self.path, "w") as f:
            json.dump(tokens, f)


class PostingClient:
    def __init__(self, client_key: str, client_secret: str,
                 store: TokenStore, http=None):
        self.client_key = client_key
        self.client_secret = client_secret
        self.store = store
        self.http = http or _default_http

    # ---------- OAuth ----------

    def authorize_url(self, redirect_uri: str, scope: str,
                      state: str = "") -> str:
        q = {"client_key": self.client_key, "redirect_uri": redirect_uri,
             "response_type": "code", "scope": scope}
        if state:
            q["state"] = state
        return AUTH_URL + "?" + urllib.parse.urlencode(q)

    def exchange_code(self, code: str, redirect_uri: str) -> dict:
        status, data = self.http(
            "POST", BASE + "/v2/oauth/token/",
            {"Content-Type": "application/x-www-form-urlencoded"},
            urllib.parse.urlencode({
                "client_key": self.client_key,
                "client_secret": self.client_secret,
                "grant_type": "authorization_code",
                "code": code, "redirect_uri": redirect_uri,
            }).encode())
        self.store.save(data)
        return data

    def access_token(self) -> str:
        t = self.store.load()
        fresh = (t.get("access_token") and time.time()
                 < t.get("obtained_at", 0) + t.get("expires_in", 0) - 60)
        if fresh:
            return t["access_token"]
        if not t.get("refresh_token"):
            raise RuntimeError("no refresh_token: complete OAuth flow first")
        status, data = self.http(
            "POST", BASE + "/v2/oauth/token/",
            {"Content-Type": "application/x-www-form-urlencoded"},
            urllib.parse.urlencode({
                "client_key": self.client_key,
                "client_secret": self.client_secret,
                "grant_type": "refresh_token",
                "refresh_token": t["refresh_token"],
            }).encode())
        self.store.save(data)  # persist ROTATED refresh_token
        return data["access_token"]

    def _auth(self) -> dict:
        return {"Authorization": f"Bearer {self.access_token()}",
                "Content-Type": "application/json; charset=UTF-8"}

    # ---------- publish ----------

    def creator_info(self) -> dict:
        """Max duration, allowed privacy levels — call before init."""
        status, data = self.http(
            "POST", BASE + "/v2/post/publish/creator_info/",
            self._auth(), b"{}")
        return data["data"]

    def publish_file(self, path: str, title: str,
                     privacy_level: str = "SELF_ONLY",
                     draft: bool = False,
                     disable_comment: bool = False,
                     disable_duet: bool = False,
                     disable_stitch: bool = False) -> str:
        """Upload a local mp4 and (Direct Post) publish it. Returns publish_id.

        privacy_level: SELF_ONLY until the Direct Post audit passes.
        """
        size = os.path.getsize(path)
        chunk_size = _CHUNK if size >= 5 * 1024 * 1024 else size
        chunks = 1 if size < 5 * 1024 * 1024 else math.ceil(size / _CHUNK)
        endpoint = ("/v2/post/publish/inbox/video/init/" if draft
                    else "/v2/post/publish/video/init/")
        status, data = self.http(
            "POST", BASE + endpoint, self._auth(),
            json.dumps({
                "post_info": {
                    "title": title,
                    "privacy_level": privacy_level,
                    "disable_comment": disable_comment,
                    "disable_duet": disable_duet,
                    "disable_stitch": disable_stitch,
                },
                "source_info": {
                    "source": "FILE_UPLOAD",
                    "video_size": size,
                    "chunk_size": chunk_size,
                    "total_chunk_count": chunks,
                },
            }).encode())
        err = data.get("error", {})
        if err.get("code") != "ok":
            raise RuntimeError(f"init failed: {err}")
        publish_id = data["data"]["publish_id"]
        upload_url = data["data"]["upload_url"]

        with open(path, "rb") as f:
            for i in range(chunks):
                first = i * _CHUNK
                blob = f.read(_CHUNK)
                last = first + len(blob) - 1
                status, _ = self.http(
                    "PUT", upload_url,
                    {"Content-Type": "video/mp4",
                     "Content-Range": f"bytes {first}-{last}/{size}"},
                    blob)
                if status not in (200, 201, 206):
                    raise RuntimeError(
                        f"chunk {i} upload failed: HTTP {status}")
        return publish_id

    def poll_status(self, publish_id: str, timeout_s: int = 600,
                    interval_s: int = 15) -> str:
        """Poll until PUBLISH_COMPLETE / FAILED. Respects 6 req/min."""
        deadline = time.time() + timeout_s
        while True:
            status, data = self.http(
                "POST", BASE + "/v2/post/publish/status/fetch/",
                self._auth(),
                json.dumps({"publish_id": publish_id}).encode())
            st = data["data"]["status"]
            if st in ("PUBLISH_COMPLETE", "FAILED"):
                return st
            if time.time() > deadline:
                raise TimeoutError(f"publish {publish_id} not done "
                                   f"in {timeout_s}s (last: {st})")
            time.sleep(interval_s)
