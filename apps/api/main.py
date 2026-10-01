"""AI Creator OS — web dashboard.

Toàn bộ quản trị hệ thống qua UI/UX, không cần CLI:
tài khoản, onboarding, topic, lịch live, sản xuất video,
đa nền tảng, shop affiliate, analytics, cài đặt, kill switch.

Run:  uvicorn apps.api.main:app --port 8080   (từ thư mục repo)
"""
from __future__ import annotations

import json
import os
import re
import subprocess
import sys
import threading
import uuid
from datetime import date, datetime
from pathlib import Path

sys.path.insert(0, ".")

from fastapi import BackgroundTasks, FastAPI, Form, Request
from fastapi.responses import FileResponse, HTMLResponse, RedirectResponse
from fastapi.staticfiles import StaticFiles
from fastapi.templating import Jinja2Templates

from apps.orchestrator.config import config
from apps.orchestrator.network import onboarding as ob
from apps.orchestrator.network.account import (
    FOLLOWERS_TO_LIVE,
    TRANSITIONS,
    AccountManager,
)
from apps.orchestrator.network.persona import PERSONAS
from apps.orchestrator.network.scheduler import build_schedule
from ledger.store import Ledger
from publishers import build_publishers

BASE = Path(".")
DATA = BASE / "data"
OUT = DATA / "output"
JOBS_FILE = DATA / "content_jobs.json"
DATA.mkdir(exist_ok=True)
OUT.mkdir(exist_ok=True)

app = FastAPI(title="AI Creator OS")
app.mount("/static", StaticFiles(directory="apps/api/static"), name="static")
templates = Jinja2Templates(directory="apps/api/templates")

ledger = Ledger(config.database_path)
manager = AccountManager(ledger)

_jobs_lock = threading.Lock()


# ---------------------------------------------------------------- jobs ---
def _load_jobs() -> list[dict]:
    try:
        return json.loads(JOBS_FILE.read_text())
    except Exception:
        return []


def _save_jobs(jobs: list[dict]) -> None:
    tmp = JOBS_FILE.with_suffix(".tmp")
    tmp.write_text(json.dumps(jobs, ensure_ascii=False, indent=1))
    tmp.replace(JOBS_FILE)


def _update_job(job_id: str, **fields) -> None:
    with _jobs_lock:
        jobs = _load_jobs()
        for j in jobs:
            if j["id"] == job_id:
                j.update(fields)
        _save_jobs(jobs)


def _find_font() -> str | None:
    candidates = [
        "/usr/share/fonts/truetype/noto/NotoSans-Bold.ttf",
        "/usr/share/fonts/truetype/dejavu/DejaVuSans-Bold.ttf",
        "/System/Library/Fonts/Supplemental/Arial Bold.ttf",
        "/System/Library/Fonts/Hiragino Sans GB.ttc",
    ]
    for c in candidates:
        if Path(c).exists():
            return c
    try:
        out = subprocess.run(["fc-list", ":style=Bold", "file"],
                             capture_output=True, text=True,
                             timeout=10).stdout
        for line in out.splitlines():
            p = line.split(":")[0].strip()
            if p.endswith((".ttf", ".otf")) and Path(p).exists():
                return p
    except Exception:
        pass
    return None


def _dt_escape(text: str) -> str:
    return (text.replace("\\", "\\\\").replace("'", "\\'")
                .replace(":", "\\:").replace("%", "%%"))


def _tts_wav(text: str, workdir: Path) -> Path | None:
    """Narration via repo TTS chain (Gemini -> VieNeu -> Edge). None if unusable."""
    try:
        from engines.tts.provider import build_tts
        chain = build_tts(api_key=os.environ.get("TTS_API_KEY", ""))
        res = chain.run(text, voice="default")
        if res.ok and isinstance(res.payload, (bytes, bytearray)) and res.payload:
            wav = workdir / "vo.wav"
            wav.write_bytes(bytes(res.payload))
            return wav
    except Exception:
        pass
    return None


def _wav_seconds(path: Path) -> float:
    out = subprocess.run(
        ["ffprobe", "-v", "error", "-show_entries", "format=duration",
         "-of", "csv=p=0", str(path)],
        capture_output=True, text=True, timeout=30).stdout.strip()
    return float(out or 0)


def _render_kinetic(job: dict) -> None:
    """Render a 1080x1920 kinetic-typography video from captions (+optional VO)."""
    jid = job["id"]
    work = OUT / jid
    work.mkdir(exist_ok=True)
    log: list[str] = []
    try:
        _update_job(jid, status="running", log="Bắt đầu dựng video…")
        captions = [c for c in job.get("captions", []) if c.strip()]
        if not captions:
            captions = [job.get("title", "AI Creator OS")]
        narration = (job.get("narration") or "").strip()

        audio: Path | None = None
        if narration:
            log.append("Đang tạo lồng tiếng AI…")
            _update_job(jid, status="running", log="\n".join(log))
            audio = _tts_wav(narration, work)
            if audio is None:
                log.append("Không có TTS khả dụng — dựng bản phụ đề (không lời).")

        if audio:
            total = max(_wav_seconds(audio), 3.0)
        else:
            total = max(len(captions) * 4.0, 6.0)
        per = total / len(captions)

        font = _find_font()
        if not font:
            raise RuntimeError("Không tìm thấy font hệ thống để vẽ phụ đề.")
        bg_img = None
        for cand in ("docs/assets/hero-network.webp",
                     "docs/assets/personas-team.webp"):
            if Path(cand).exists():
                bg_img = cand
                break

        vf = []
        if bg_img:
            vf.append(
                "[0:v]scale=1080:1920:force_original_aspect_ratio=increase,"
                "crop=1080:1920,boxblur=18:2,eq=brightness=-0.5:saturation=0.6,"
                "fps=30,format=yuv420p[bg]")
            base = "[bg]"
        else:
            vf.append(
                "color=c=#0d1117:s=1080x1920:r=30:d={:.2f},format=yuv420p[bg]"
                .format(total))
            base = "[bg]"
        for i, cap in enumerate(captions):
            t0, t1 = i * per, (i + 1) * per
            color = "#FFD166" if i % 2 else "white"
            vf.append(
                f"{base}drawtext=fontfile={font}:text='{_dt_escape(cap)}':"
                f"fontsize=68:fontcolor={color}:borderw=3:bordercolor=black:"
                f"x=(w-text_w)/2:y=860:enable='between(t,{t0:.2f},{t1:.2f})'[t{i}]")
            base = f"[t{i}]"
        vf.append(
            f"{base}drawtext=fontfile={font}:text='AI CREATOR OS':fontsize=28:"
            f"fontcolor=white@0.7:x=(w-text_w)/2:y=60,"
            f"drawbox=x=0:y=1900:w='1080*t/{total:.2f}':h=20:c=#EF476F:t=fill[v]")
        filt = ";".join(vf)

        out_mp4 = OUT / f"{jid}.mp4"
        cmd = ["ffmpeg", "-y", "-v", "error"]
        if bg_img:
            cmd += ["-loop", "1", "-i", bg_img]
        if audio:
            cmd += ["-i", str(audio)]
        cmd += ["-filter_complex", filt, "-map", "[v]"]
        if audio:
            cmd += ["-map", f"{1 if bg_img else 0}:a"]
        cmd += ["-t", f"{total:.2f}", "-c:v", "libx264", "-preset", "medium",
                "-crf", "20"]
        if audio:
            cmd += ["-c:a", "aac", "-b:a", "128k"]
        cmd += ["-movflags", "+faststart", str(out_mp4)]
        subprocess.run(cmd, check=True, timeout=600)
        log.append(f"Xong: {out_mp4.name} ({total:.1f}s)")
        ledger.decide("content", "video_rendered", job.get("title"),
                      f"kinetic video {total:.1f}s",
                      {"job": jid, "voice": bool(audio)})
        _update_job(jid, status="done", output=f"{jid}.mp4",
                    log="\n".join(log), finished_at=_now())
    except Exception as e:  # never crash the dashboard
        _update_job(jid, status="failed", log=f"Lỗi: {e}",
                    finished_at=_now())


def _run_job(job_id: str, job: dict) -> None:
    _render_kinetic(job)


# -------------------------------------------------------------- helpers ---
def _now() -> str:
    return datetime.now().isoformat(timespec="seconds")


def _ctx(**kw) -> dict:
    return {"config": config,
            "kill": config.kill_switch, "dry_run": config.dry_run, **kw}


def _advance_onboarding(account_id: int) -> str:
    status = ""
    for _ in range(8):
        before = manager.get(account_id).status
        try:
            status = ob.onboard_step(manager, account_id)
        except Exception:
            break
        if status == before:
            break
    return status


def _env_present(name: str) -> bool:
    return bool(os.environ.get(name))


# ------------------------------------------------------------------ UI ---
@app.get("/", response_class=HTMLResponse)
def dashboard(request: Request):
    accounts = manager.list()
    by_status: dict[str, int] = {}
    for a in accounts:
        by_status[a.status] = by_status.get(a.status, 0) + 1
    revenue = ledger.db.execute(
        "SELECT COALESCE(SUM(commission),0) s FROM orders").fetchone()["s"]
    today = date.today().isoformat()
    slots = [dict(r) for r in ledger.db.execute(
        "SELECT * FROM live_slots WHERE slot_date=? ORDER BY start_min",
        (today,)).fetchall()]
    decisions = [dict(r) for r in ledger.db.execute(
        "SELECT * FROM decisions ORDER BY id DESC LIMIT 8").fetchall()]
    acct_names = {a.id: a.username for a in accounts}
    return templates.TemplateResponse(request, "dashboard.html", _ctx(accounts=accounts, by_status=by_status, revenue=revenue,
        slots=slots, decisions=decisions, today=today,
        acct_names=acct_names, n_jobs=len(_load_jobs())))


@app.get("/accounts", response_class=HTMLResponse)
def accounts_page(request: Request):
    return templates.TemplateResponse(request, "accounts.html", _ctx(accounts=manager.list(), personas=PERSONAS,
        followers_need=FOLLOWERS_TO_LIVE))


@app.get("/accounts/new", response_class=HTMLResponse)
def account_new(request: Request):
    return templates.TemplateResponse(request, "account_new.html", _ctx())


@app.post("/accounts")
def account_create(username: str = Form(...),
                   niche_hint: str = Form(""),
                   rtmp_key_ref: str = Form(""),
                   youtube_channel: str = Form("")):
    username = username.strip().lstrip("@")
    acct = manager.add(username, niche_hint.strip(),
                       rtmp_key_ref.strip() or None)
    if youtube_channel.strip():
        ledger.set_account_status(acct.id, acct.status,
                                  youtube_channel=youtube_channel.strip())
    _advance_onboarding(acct.id)
    return RedirectResponse(f"/accounts/{acct.id}", status_code=303)


@app.get("/accounts/{account_id}", response_class=HTMLResponse)
def account_detail(request: Request, account_id: int):
    acct = manager.get(account_id)
    allowed = sorted(TRANSITIONS.get(acct.status, set()))
    persona_info = PERSONAS.get(acct.persona or "")
    decisions = [dict(r) for r in ledger.db.execute(
        "SELECT * FROM decisions WHERE target=? ORDER BY id DESC LIMIT 20",
        (acct.username,)).fetchall()]
    pubs = build_publishers(acct)
    gifts = ledger.account_gift_usd(account_id)
    planned = ledger.db.execute(
        "SELECT target, reason, created_at FROM decisions "
        "WHERE agent = 'live_planner' AND action = 'session_topic' "
        "AND target LIKE ? ORDER BY id DESC LIMIT 1",
        (f"{acct.username}:%",)).fetchone()
    return templates.TemplateResponse(request, "account_detail.html", _ctx(acct=acct, allowed=allowed, persona_info=persona_info,
        decisions=decisions, publishers=[p.platform for p in pubs],
        gifts=gifts, last_topic=dict(planned) if planned else None))


@app.post("/accounts/{account_id}/transition")
def account_transition(account_id: int, to: str = Form(...)):
    try:
        manager.transition(account_id, to)
    except ValueError:
        pass
    return RedirectResponse(f"/accounts/{account_id}", status_code=303)


@app.post("/accounts/{account_id}/followers")
def account_followers(account_id: int, n: int = Form(...)):
    ledger.set_followers(account_id, max(0, n))
    return RedirectResponse(f"/accounts/{account_id}", status_code=303)


@app.post("/accounts/{account_id}/youtube")
def account_youtube(account_id: int,
                    channel: str = Form(""),
                    content_types: str = Form("")):
    kinds = [k.strip() for k in content_types.split(",") if k.strip()]
    acct = manager.get(account_id)
    ledger.set_account_status(
        account_id, acct.status,
        youtube_channel=channel.strip() or None,
        youtube_content_types=json.dumps(kinds) if kinds else None)
    return RedirectResponse(f"/accounts/{account_id}", status_code=303)


@app.post("/accounts/{account_id}/onboard")
def account_onboard(account_id: int):
    _advance_onboarding(account_id)
    return RedirectResponse(f"/accounts/{account_id}", status_code=303)


@app.post("/accounts/{account_id}/replan")
def account_replan(account_id: int):
    acct = manager.get(account_id)
    research = ob.make_topic_research(manager, account_id)
    research(acct.niche_hint or "")
    return RedirectResponse(f"/accounts/{account_id}", status_code=303)


@app.post("/accounts/{account_id}/live-topic")
def account_live_topic(account_id: int):
    from apps.orchestrator.network.topics import plan_live_topic
    plan_live_topic(manager, account_id)
    return RedirectResponse(f"/accounts/{account_id}", status_code=303)


@app.get("/schedule", response_class=HTMLResponse)
def schedule_page(request: Request):
    today = date.today().isoformat()
    slots = [dict(r) for r in ledger.db.execute(
        "SELECT * FROM live_slots WHERE slot_date=? ORDER BY start_min",
        (today,)).fetchall()]
    eligible = [a for a in manager.list()
                if a.status in ("live_ready", "live")]
    return templates.TemplateResponse(request, "schedule.html", _ctx(slots=slots, today=today, eligible=eligible,
        weekday=date.today().weekday()))


@app.post("/schedule/build")
def schedule_build():
    today = date.today().isoformat()
    slots = build_schedule(manager.list(), weekday=date.today().weekday())
    ledger.db.execute("DELETE FROM live_slots WHERE slot_date=?", (today,))
    ledger.save_slots([{
        "account_id": s.account_id, "slot_date": today,
        "start_min": s.start_min, "duration_min": s.duration_min,
        "status": "planned"} for s in slots])
    ledger.db.commit()
    ledger.decide("scheduler", "build_schedule", today,
                  f"{len(slots)} slots", {})
    return RedirectResponse("/schedule", status_code=303)


@app.get("/content", response_class=HTMLResponse)
def content_page(request: Request):
    return templates.TemplateResponse(request, "content.html",
                                      _ctx(jobs=_load_jobs()))


@app.post("/content")
def content_create(background: BackgroundTasks,
                   title: str = Form(...),
                   captions: str = Form(...),
                   narration: str = Form("")):
    job = {"id": uuid.uuid4().hex[:8], "title": title.strip(),
           "captions": [c.strip() for c in captions.splitlines()
                        if c.strip()],
           "narration": narration.strip(), "status": "queued",
           "created_at": _now(), "output": None, "log": "Đang chờ…"}
    with _jobs_lock:
        jobs = _load_jobs()
        jobs.insert(0, job)
        _save_jobs(jobs)
    background.add_task(_run_job, job["id"], job)
    return RedirectResponse("/content", status_code=303)


@app.get("/media/{name}")
def media_file(name: str):
    if not re.fullmatch(r"[A-Za-z0-9_-]+\.mp4", name):
        return HTMLResponse("Not found", status_code=404)
    path = OUT / name
    if not path.exists():
        return HTMLResponse("Not found", status_code=404)
    return FileResponse(path, media_type="video/mp4")


@app.get("/publishers", response_class=HTMLResponse)
def publishers_page(request: Request):
    rows = []
    for acct in manager.list():
        pubs = build_publishers(acct)
        rows.append({
            "username": acct.username,
            "status": acct.status,
            "tiktok_token": Path(f"tiktok_token_{acct.username}.json").exists(),
            "youtube_token": Path(f"youtube_token_{acct.username}.json").exists(),
            "fb_page": _env_present(f"FB_PAGE_ID_{acct.username.upper()}"),
            "rtmp": bool(acct.rtmp_key()),
            "platforms": [p.platform for p in pubs],
        })
    return templates.TemplateResponse(request, "publishers.html",
                                      _ctx(rows=rows))


@app.get("/shop", response_class=HTMLResponse)
def shop_page(request: Request):
    products = [dict(r) for r in ledger.shelf_products(50)]
    return templates.TemplateResponse(request, "shop.html",
                                      _ctx(products=products))


@app.post("/shop/add")
def shop_add(platform_pid: str = Form(...),
             title: str = Form(...),
             price: float = Form(...),
             commission_rate: float = Form(...),
             category: str = Form("")):
    ledger.upsert_product(
        platform_pid.strip(), title=title.strip(), price=price,
        commission_rate=commission_rate,
        commission_value=price * commission_rate,
        category=category.strip() or None, status="shelf", score=0.0)
    return RedirectResponse("/shop", status_code=303)


@app.get("/analytics", response_class=HTMLResponse)
def analytics_page(request: Request):
    revenue = ledger.db.execute(
        "SELECT COALESCE(SUM(commission),0) s FROM orders").fetchone()["s"]
    commissions = ledger.db.execute(
        "SELECT COALESCE(SUM(amount),0) s FROM commissions").fetchone()["s"]
    sessions = ledger.db.execute(
        "SELECT COUNT(*) c FROM live_sessions").fetchone()["c"]
    gift_rows = []
    for acct in manager.list():
        g = ledger.account_gift_usd(acct.id)
        if g:
            gift_rows.append({"username": acct.username, "usd": g})
    usage = [dict(r) for r in ledger.db.execute(
        "SELECT engine, provider, COALESCE(SUM(cost_usd),0) cost, "
        "COUNT(*) n FROM api_usage GROUP BY engine, provider").fetchall()]
    return templates.TemplateResponse(request, "analytics.html", _ctx(revenue=revenue, commissions=commissions,
        sessions=sessions, gift_rows=gift_rows, usage=usage,
        spend=ledger.daily_spend_usd()))


@app.get("/settings", response_class=HTMLResponse)
def settings_page(request: Request):
    envs = ["TTS_API_KEY", "TIKTOK_SHOP_APP_KEY", "TIKTOK_SHOP_APP_SECRET",
            "TIKTOK_CLIENT_KEY", "TIKTOK_CLIENT_SECRET",
            "YOUTUBE_CLIENT_ID", "YOUTUBE_CLIENT_SECRET", "FB_PAGE_ID"]
    env_status = [(e, _env_present(e)) for e in envs]
    rtmp_rows = [(a.username, a.rtmp_key_ref or "",
                  bool(a.rtmp_key())) for a in manager.list()]
    return templates.TemplateResponse(request, "settings.html", _ctx(env_status=env_status, rtmp_rows=rtmp_rows,
        db_path=config.database_path))


@app.post("/settings/dryrun")
def settings_dryrun(value: str = Form(...)):
    object.__setattr__(config, "dry_run", value == "on")
    ledger.decide("human", "dry_run", None,
                  f"set dry_run={config.dry_run} via dashboard")
    return RedirectResponse("/settings", status_code=303)


@app.post("/kill")
def kill():
    object.__setattr__(config, "kill_switch", True)
    ledger.decide("human", "kill_switch", None, "engaged via dashboard")
    return RedirectResponse("/", status_code=303)


@app.post("/unkill")
def unkill():
    object.__setattr__(config, "kill_switch", False)
    ledger.decide("human", "kill_switch", None, "released via dashboard")
    return RedirectResponse("/", status_code=303)


# ------------------------------------------------------- legacy JSON API ---
@app.get("/api/stats")
def api_stats():
    rev = ledger.db.execute(
        "SELECT COALESCE(SUM(commission),0) s FROM orders").fetchone()["s"]
    return {"dry_run": config.dry_run, "kill_switch": config.kill_switch,
            "total_commission": rev,
            "daily_spend_usd": ledger.daily_spend_usd()}


@app.get("/api/products")
def api_products():
    return [dict(r) for r in ledger.db.execute(
        "SELECT * FROM products ORDER BY score DESC LIMIT 50").fetchall()]


@app.get("/api/decisions")
def api_decisions():
    return [dict(r) for r in ledger.db.execute(
        "SELECT * FROM decisions ORDER BY id DESC LIMIT 100").fetchall()]
