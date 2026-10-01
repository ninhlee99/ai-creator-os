"""Governance rules must hold without any LLM or network."""
import sys
sys.path.insert(0, ".")

from apps.orchestrator.config import Config
from apps.orchestrator.governance import (
    check_live_session, evaluate_all, hunter_score, product_eligible,
    should_kill_product, should_scale_product)


def test_hunter_score_prefers_earnings_over_rate():
    # 30% of cheap vs 10% of expensive with conversion
    cheap = hunter_score(100_000, 0.30, 0.02, 1.0)
    pricey = hunter_score(1_000_000, 0.10, 0.02, 1.0)
    assert pricey > cheap


def test_product_eligible_blocks():
    cfg = Config()
    assert not product_eligible(cfg, -1, 0.2, 4.5, "gia dụng").allowed
    assert not product_eligible(cfg, 100_000, 0.2, 3.0, "gia dụng").allowed
    assert not product_eligible(cfg, 100_000, 0.2, 4.5,
                                "thuốc", ("thuốc",)).allowed
    assert product_eligible(cfg, 100_000, 0.2, 4.5, "gia dụng").allowed


def test_kill_rules():
    cfg = Config()
    v = should_kill_product(cfg, {"views": 50_000, "orders": 0}, 5)
    assert v.allowed
    v = should_kill_product(cfg, {"views": 100, "orders": 0}, 1)
    assert not v.allowed


def test_scale_rules():
    v = should_scale_product({"orders": 10, "commission": 500_000, "cost": 100_000})
    assert v.allowed
    v = should_scale_product({"orders": 2, "commission": 500_000, "cost": 100_000})
    assert not v.allowed


def test_session_limit_and_killswitch():
    cfg = Config()
    assert check_live_session(cfg, 10).allowed
    assert not check_live_session(cfg, 10_000).allowed
    object.__setattr__(cfg, "kill_switch", True)
    assert not evaluate_all(cfg, 0).allowed


def test_dry_run_blocks_external():
    cfg = Config()
    assert not evaluate_all(cfg, 0).allowed  # dry_run=True by default
