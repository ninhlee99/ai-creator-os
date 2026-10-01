"""Tests: topic engine (hint honored vs auto), publishers, film pipeline."""
import json
import subprocess
import sys
sys.path.insert(0, ".")

from apps.orchestrator.network.account import Account, AccountManager
from apps.orchestrator.network.onboarding import onboard_step
from apps.orchestrator.network.topics import (_parse_plan, FALLBACK_NICHES,
                                              plan_live_topic, resolve_topic)
from ledger.store import Ledger
from publishers import build_publishers
from publishers.base import CONTENT_KINDS
from publishers.facebook import FacebookPublisher
from publishers.tiktok import TikTokPublisher
from publishers.youtube import YouTubePublisher, CATEGORY_BY_KIND


def make_ledger():
    return Ledger(":memory:")


class FakeLLM:
    def __init__(self, payload=None, ok=True):
        self.payload = payload
        self.ok = ok
        self.last_prompt = ""

    def run(self, system="", prompt="", **kw):
        self.last_prompt = prompt
        from engines.base import EngineResult
        return EngineResult(ok=self.ok, provider="fake",
                            payload=self.payload, error="" if self.ok else "x")


def mk_account(**kw):
    d = dict(id=1, username="u1", status="onboarding", persona="storyteller",
             niche=None, niche_hint="", followers=0, rtmp_key_ref=None)
    d.update(kw)
    return Account(**d)


# ---------- topic engine ----------

def test_hint_honored_as_soft_suggestion():
    llm = FakeLLM(payload=json.dumps({
        "niche": "sách self-help",
        "topics": ["tập 1", "tập 2"]}))
    acct = mk_account(niche_hint="bán sách self-help")
    plan = resolve_topic(acct, llm=llm, network_niches=[])
    # hint is a soft suggestion, not a final decision: source="hint"
    assert plan.source == "hint"
    assert plan.niche == "sách self-help"
    assert plan.topics == ["tập 1", "tập 2"]
    assert "bán sách self-help" in llm.last_prompt  # hint fed to the model
    assert "gợi ý" in llm.last_prompt.lower() or "GỢI Ý" in llm.last_prompt


def test_auto_avoids_taken_niches():
    llm = FakeLLM(payload=json.dumps({
        "niche": "chuyện ma học đường",
        "topics": ["tập 1"]}))
    acct = mk_account()
    plan = resolve_topic(acct, llm=llm,
                         network_niches=["kể chuyện đêm khuya"])
    assert plan.source == "auto"
    assert "kể chuyện đêm khuya" in llm.last_prompt  # told to avoid
    assert plan.niche != "kể chuyện đêm khuya"


def test_llm_failure_falls_back():
    acct = mk_account(niche_hint="",
                      persona="teacher")
    plan = resolve_topic(acct, llm=FakeLLM(ok=False))
    assert plan.source == "fallback"
    assert plan.niche  # persona default, never empty


def test_parse_plan_tolerates_fences():
    raw = '```json\n{"niche": "x", "topics": ["a"]}\n```'
    assert _parse_plan(raw)["niche"] == "x"
    assert _parse_plan("not json") is None


def test_onboarding_persists_topic_plan():
    ledger = make_ledger()
    mgr = AccountManager(ledger)
    acct = mgr.add("topicacct", niche_hint="đồ gia dụng")
    llm = FakeLLM(payload=json.dumps({
        "niche": "review đồ gia dụng",
        "topics": ["nồi chiên", "máy hút bụi"]}))
    st = onboard_step(mgr, acct.id, llm=llm)
    assert st == "researching"
    got = mgr.get(acct.id)
    assert got.niche == "review đồ gia dụng"
    assert got.topics == ["nồi chiên", "máy hút bụi"]


# ---------- publishers ----------

def test_youtube_kind_gating():
    storyteller_yt = YouTubePublisher("u1", allowed_kinds=["short_film"])
    musician_yt = YouTubePublisher(
        "u2", allowed_kinds=["ai_music", "ai_remix"])
    assert storyteller_yt.handles("short_film")
    assert not storyteller_yt.handles("ai_music")
    assert musician_yt.handles("ai_remix")
    assert not musician_yt.handles("short_film")


def test_youtube_category_map():
    assert CATEGORY_BY_KIND["ai_music"] == "10"
    assert CATEGORY_BY_KIND["short_film"] == "24"
    assert set(CONTENT_KINDS) >= {"short_video", "short_film",
                                  "ai_music", "ai_remix"}


def test_unconfigured_publishers_skip():
    acct = mk_account(username="nouser",
                      youtube_content_types=["short_film"])
    pubs = build_publishers(acct, kinds=("short_film",))
    # no credentials in test env -> nothing configured
    assert pubs == []
    assert not TikTokPublisher("nouser").is_configured()
    assert not FacebookPublisher("nouser").is_configured()
    assert not YouTubePublisher(
        "nouser", allowed_kinds=["short_film"]).is_configured()


def test_facebook_draft_result_shape():
    pub = FacebookPublisher("nouser")
    r = pub.publish("/tmp/x.mp4", "t", "d", "short_video")
    assert r.ok is False and r.platform == "facebook"


# ---------- film ----------

def test_build_srt_timing():
    from agents.content.film import build_srt
    srt = build_srt(["chào", "tạm biệt"], [2.0, 3.0])
    assert "00:00:00,000 --> 00:00:02,000" in srt
    assert "00:00:02,000 --> 00:00:05,000" in srt


def test_film_end_to_end(tmp_path=None):
    import tempfile
    from pathlib import Path
    from engines.base import EngineResult
    from agents.content.film import make_film

    work = Path(tempfile.mkdtemp())

    def image_fn(prompt, out):
        subprocess.run(["ffmpeg", "-y", "-f", "lavfi", "-i",
                        "color=c=0x223344:s=1080x1920:d=1",
                        "-frames:v", "1", str(out)],
                       check=True, capture_output=True)
        return out

    class TTS:
        def run(self, text, voice="default", **kw):
            wav = work / "t.wav"
            subprocess.run(["ffmpeg", "-y", "-f", "lavfi", "-i",
                            "sine=frequency=440:duration=2",
                            "-ar", "24000", "-ac", "1", str(wav)],
                           check=True, capture_output=True)
            return EngineResult(ok=True, provider="fake",
                                payload=wav.read_bytes())

    llm = FakeLLM(payload=json.dumps({
        "title": "Phim test",
        "scenes": [
            {"narration": "cảnh một", "image_prompt": "night city",
             "seconds": 2},
            {"narration": "cảnh hai", "image_prompt": "sunrise",
             "seconds": 2},
        ]}))
    res = make_film("chủ đề test", llm, TTS(), image_fn, work,
                    target_seconds=30)
    assert res["ok"] and res["scenes"] == 2
    out = Path(res["path"])
    assert out.exists() and out.stat().st_size > 10_000
    probe = subprocess.run(
        ["ffmpeg", "-i", str(out)], capture_output=True, text=True)
    assert "1080x1920" in probe.stderr
    # ~4s of narration
    assert 3.0 <= res["seconds"] <= 6.0


# ---------- topic engine: fallback + live planner ----------

def test_fallback_niche_covers_coder():
    assert "coder" in FALLBACK_NICHES
    acct = mk_account(persona="coder")
    plan = resolve_topic(acct, llm=None)
    assert plan.source == "fallback"
    assert plan.niche == FALLBACK_NICHES["coder"]


def test_plan_live_topic_round_robin_and_logged():
    ledger = make_ledger()
    mgr = AccountManager(ledger)
    acct = mgr.add("live_chan")
    mgr.set_topic_plan(acct.id, "truyện ma", ["tập A", "tập B"])

    first = plan_live_topic(mgr, acct.id)
    assert first["topic"] == "tập A"
    second = plan_live_topic(mgr, acct.id)
    assert second["topic"] == "tập B"  # avoids the recently used topic
    third = plan_live_topic(mgr, acct.id)
    assert third["topic"] == "tập A"  # wraps around when all used

    rows = ledger.db.execute(
        "SELECT COUNT(*) c FROM decisions WHERE agent = 'live_planner' "
        "AND action = 'session_topic'").fetchone()
    assert rows["c"] == 3


def test_plan_live_topic_without_plan_uses_niche():
    ledger = make_ledger()
    mgr = AccountManager(ledger)
    acct = mgr.add("plain_chan", niche_hint="nấu ăn")
    mgr.set_topic_plan(acct.id, "nấu ăn", [])
    res = plan_live_topic(mgr, acct.id)
    assert res["topic"] == "nấu ăn"
    assert "no episode plan" in res["basis"]
