"""Governance: deterministic rules engine. Pure functions, NO LLM inside.

LLMs propose; this module disposes. Every decision is recorded in the ledger.
"""
from __future__ import annotations

from dataclasses import dataclass

from .config import Config


@dataclass
class Verdict:
    allowed: bool
    reason: str


def check_kill_switch(cfg: Config) -> Verdict:
    if cfg.kill_switch:
        return Verdict(False, "global kill switch is ON")
    return Verdict(True, "ok")


def check_budget(cfg: Config, spent_today_usd: float) -> Verdict:
    if spent_today_usd >= cfg.daily_api_budget_usd:
        return Verdict(
            False,
            f"daily API budget exhausted "
            f"({spent_today_usd:.2f}/{cfg.daily_api_budget_usd:.2f} USD)",
        )
    return Verdict(True, "ok")


def check_live_session(cfg: Config, elapsed_min: int) -> Verdict:
    """Called every minute during a live session."""
    v = check_kill_switch(cfg)
    if not v.allowed:
        return v
    if elapsed_min >= cfg.max_live_minutes_per_session:
        return Verdict(
            False,
            f"session reached max duration "
            f"({cfg.max_live_minutes_per_session} min) — anti-ban pacing",
        )
    return Verdict(True, "ok")


def product_eligible(cfg: Config, price: float, commission_rate: float,
                     seller_rating: float | None, category: str,
                     blocked_categories: tuple[str, ...] = ()) -> Verdict:
    """Hunter gate: product may enter the shelf only if eligible."""
    if category in blocked_categories:
        return Verdict(False, f"category blocked: {category}")
    if price <= 0 or price > cfg.max_price:
        return Verdict(False, f"price out of band: {price}")
    if not 0 < commission_rate <= 1:
        return Verdict(False, f"invalid commission rate: {commission_rate}")
    if seller_rating is not None and seller_rating < cfg.min_seller_rating:
        return Verdict(False, f"seller rating too low: {seller_rating}")
    return Verdict(True, "ok")


def hunter_score(price: float, commission_rate: float, conversion_rate: float,
                 competition: float) -> float:
    """Score = expected commission value x conversion / (1 + competition).

    Optimizes for earnings, NOT commission % alone.
    """
    commission_value = price * commission_rate
    return commission_value * conversion_rate / (1.0 + max(0.0, competition))


def should_kill_product(cfg: Config, stats: dict, sessions_featured: int) -> Verdict:
    """Analyst rule: cut losers fast."""
    views = stats.get("views", 0)
    orders = stats.get("orders", 0)
    if orders == 0 and views >= cfg.kill_views_no_order:
        return Verdict(
            True, f"kill: 0 orders after {views} views "
                   f"(threshold {cfg.kill_views_no_order})")
    if orders == 0 and sessions_featured >= cfg.kill_sessions_no_order:
        return Verdict(
            True, f"kill: 0 orders after {sessions_featured} live sessions")
    return Verdict(False, "keep: within tolerance")


def should_scale_product(stats: dict, min_orders: int = 5,
                         min_roi: float = 2.0) -> Verdict:
    """Scale winners: enough orders and commission covers cost multiple."""
    orders = stats.get("orders", 0)
    commission = stats.get("commission", 0.0)
    cost = stats.get("cost", 0.0)
    if orders < min_orders:
        return Verdict(False, "not yet: too few orders")
    roi = (commission / cost) if cost > 0 else float("inf")
    if roi >= min_roi:
        return Verdict(True, f"scale: {orders} orders, ROI {roi:.1f}x")
    return Verdict(False, f"not yet: ROI {roi:.1f}x < {min_roi}x")


def evaluate_all(cfg: Config, spent_today_usd: float) -> Verdict:
    """Top-level gate before any external action."""
    for check in (check_kill_switch(cfg),
                  check_budget(cfg, spent_today_usd)):
        if not check.allowed:
            return check
    if cfg.dry_run:
        return Verdict(False, "dry-run mode: external actions disabled")
    return Verdict(True, "ok")
