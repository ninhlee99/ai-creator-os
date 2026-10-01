"""Facebook publisher: managed Page video posts via facebook-cli.

Draft-first: creates a native Page draft with the MP4 attached, then
publishes the same draft (skipped when FB_DRAFT_ONLY=1 so a human can
review the draft in the Page first).

Per-account Page mapping: FB_PAGE_ID_<USERNAME>, else shared FB_PAGE_ID.
facebook-cli handles its own auth + approval flow.
"""
from __future__ import annotations

import json
import os
import shutil
import subprocess
import sys
import uuid
sys.path.insert(0, ".")

from .base import PublishResult, Publisher, _env_for


class FacebookPublisher(Publisher):
    name = "facebook"
    kinds = ("short_video", "short_film")

    def __init__(self, username: str):
        self.username = username
        self.page_id = _env_for(username, "FB_PAGE_ID")
        self.draft_only = os.environ.get("FB_DRAFT_ONLY", "1") == "1"

    def is_configured(self) -> bool:
        return bool(self.page_id and shutil.which("facebook-cli"))

    def _run(self, *args: str) -> dict:
        out = subprocess.run(["facebook-cli", *args],
                             capture_output=True, text=True, timeout=300)
        try:
            return json.loads(out.stdout or "{}")
        except Exception:
            return {"_raw": out.stdout[-500:], "_stderr": out.stderr[-500:],
                    "_exit": out.returncode}

    def publish(self, video_path: str, title: str, description: str,
                kind: str, **kw) -> PublishResult:
        if not self.is_configured():
            return PublishResult(False, self.name,
                                 error="facebook page not configured")
        try:
            text = f"{title}\n\n{description}".strip()[:2000]
            draft = self._run(
                "pages", "drafts", "create",
                "--page-id", self.page_id,
                "--text", text,
                "--request-id", str(uuid.uuid4()),
                "--file", video_path)
            draft_id = (draft.get("draft_id") or draft.get("id")
                        or draft.get("data", {}).get("id"))
            if not draft_id:
                return PublishResult(False, self.name, draft=True,
                                     error=f"draft create failed: "
                                           f"{str(draft)[:300]}")
            if self.draft_only:
                return PublishResult(True, self.name,
                                     remote_id=str(draft_id), draft=True)
            pub = self._run(
                "pages", "drafts", "publish",
                "--page-id", self.page_id,
                "--draft-id", str(draft_id),
                "--request-id", str(uuid.uuid4()),
                "--privacy", "PUBLIC")
            post_id = (pub.get("post_id") or pub.get("id")
                       or str(draft_id))
            if pub.get("error") or pub.get("_exit"):
                return PublishResult(False, self.name,
                                     error=f"draft publish failed: "
                                           f"{str(pub)[:300]}")
            return PublishResult(True, self.name, remote_id=str(post_id))
        except Exception as e:
            return PublishResult(False, self.name, error=str(e)[:300])
