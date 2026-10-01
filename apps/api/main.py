"""Control plane API: start/stop, kill switch, review queue, stats.

Run: uvicorn apps.api.main:app --port 8080
"""
from __future__ import annotations

import sys
sys.path.insert(0, ".")

from fastapi import FastAPI
from fastapi.responses import HTMLResponse

from apps.orchestrator.config import config
from ledger.store import Ledger

app = FastAPI(title="tiktok-affiliate-os")
ledger = Ledger(config.database_path)


@app.get("/", response_class=HTMLResponse)
def index():
    return """<h1>tiktok-affiliate-os</h1>
    <ul>
      <li><a href="/stats">stats</a></li>
      <li><a href="/products">products</a></li>
      <li><a href="/decisions">decisions</a></li>
    </ul>
    <form method="post" action="/kill"><button>GLOBAL KILL SWITCH</button></form>
    <form method="post" action="/unkill"><button>release kill switch</button></form>
    """


@app.get("/stats")
def stats():
    rev = ledger.db.execute(
        "SELECT COALESCE(SUM(commission),0) s FROM orders").fetchone()["s"]
    return {
        "dry_run": config.dry_run,
        "kill_switch": config.kill_switch,
        "total_commission": rev,
        "daily_spend_usd": ledger.daily_spend_usd(),
    }


@app.get("/products")
def products():
    return [dict(r) for r in ledger.db.execute(
        "SELECT * FROM products ORDER BY score DESC LIMIT 50").fetchall()]


@app.get("/decisions")
def decisions():
    return [dict(r) for r in ledger.db.execute(
        "SELECT * FROM decisions ORDER BY id DESC LIMIT 100").fetchall()]


@app.post("/kill")
def kill():
    object.__setattr__(config, "kill_switch", True)
    ledger.decide("human", "kill_switch", None, "engaged via dashboard")
    return {"kill_switch": True}


@app.post("/unkill")
def unkill():
    object.__setattr__(config, "kill_switch", False)
    ledger.decide("human", "kill_switch", None, "released via dashboard")
    return {"kill_switch": False}
