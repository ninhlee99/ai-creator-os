"""Analyst agent: money. Runs after each live session + daily rollup.

- Pulls orders/commissions from TikTok Shop API (PROVIDER EVIDENCE ONLY).
- Computes per-product / per-session ROI.
- Applies kill/scale rules via governance. Analyst proposes; governance disposes.
- Never invents numbers. Never spends.
"""
from __future__ import annotations

import sys
sys.path.insert(0, ".")

from apps.orchestrator.config import Config
from apps.orchestrator.governance import (
    check_kill_switch, should_kill_product, should_scale_product)
from ledger.store import Ledger


def reconcile_orders(cfg: Config, ledger: Ledger, shop_client) -> int:
    """Fetch affiliate orders from TikTok Shop API, idempotent insert."""
    v = check_kill_switch(cfg)
    if not v.allowed:
        return 0
    try:
        raw_orders = shop_client.affiliate_orders()
    except NotImplementedError:
        return 0
    n = 0
    for o in raw_orders:
        rid = ledger.record_order(
            o["order_id"], product_id=o.get("product_id"),
            session_id=o.get("session_id"), amount=o["amount"],
            commission=o["commission"], ordered_at=o["ordered_at"])
        if rid:
            n += 1
    return n


def review_products(cfg: Config, ledger: Ledger) -> dict:
    killed, scaled, kept = [], [], []
    rows = ledger.db.execute(
        "SELECT * FROM products WHERE status IN ('shelf','scaled')").fetchall()
    for r in rows:
        p = dict(r)
        stats = ledger.product_stats(p["id"])
        sessions_featured = ledger.db.execute(
            "SELECT COUNT(DISTINCT session_id) AS c FROM live_events "
            "WHERE payload LIKE ?", (f"%{p['title'][:20]}%",)).fetchone()["c"]
        kv = should_kill_product(cfg, stats, sessions_featured)
        if kv.allowed:
            ledger.set_product_status(p["id"], "killed")
            ledger.decide("analyst", "kill_product", str(p["id"]), kv.reason, stats)
            killed.append(p["platform_pid"])
            continue
        sv = should_scale_product(stats)
        if sv.allowed:
            ledger.set_product_status(p["id"], "scaled")
            ledger.decide("analyst", "scale_product", str(p["id"]), sv.reason, stats)
            scaled.append(p["platform_pid"])
        else:
            kept.append(p["platform_pid"])
    return {"killed": killed, "scaled": scaled, "kept": kept}


def run(cfg: Config, ledger: Ledger, shop_client=None) -> dict:
    v = check_kill_switch(cfg)
    if not v.allowed:
        return {"ok": False, "reason": v.reason}
    reconciled = reconcile_orders(cfg, ledger, shop_client) if shop_client else 0
    review = review_products(cfg, ledger)
    return {"ok": True, "orders_reconciled": reconciled, **review}
