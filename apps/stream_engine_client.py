"""Client for the Go stream-engine (local HTTP control)."""
from __future__ import annotations

import json
import urllib.request


class StreamEngine:
    def __init__(self, base_url: str = "http://127.0.0.1:8090"):
        self.base = base_url.rstrip("/")

    def _post(self, path: str, data: dict) -> dict:
        req = urllib.request.Request(
            self.base + path, data=json.dumps(data).encode(),
            headers={"Content-Type": "application/json"}, method="POST")
        with urllib.request.urlopen(req, timeout=10) as r:
            return json.load(r)

    def start(self, rtmp_url: str, rtmp_key: str, disclosure: str = ""):
        return self._post("/start", {"rtmp_url": rtmp_url,
                                     "rtmp_key": rtmp_key,
                                     "disclosure": disclosure})

    def push_segment(self, audio: bytes, caption: str = ""):
        # v1: audio bytes via temp file; v2: shared frame pipe
        return {"ok": True, "note": "segment queued (stub)"}

    def stop(self):
        return self._post("/stop", {})
