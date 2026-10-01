"""Hunter agent: discover high-commission affiliate products.

Scheduled daily. Pulls candidates from TikTok Shop affiliate marketplace,
scores by EXPECTED EARNINGS (not commission % alone), passes governance,
writes the shelf to the ledger.

TikTok Shop API client details: docs/RESEARCH/tiktok_shop_api.md
"""
from __future__ import annotations

from dataclasses import dataclass

import sys
sys.path.insert(0, ".")
from apps.orchestrator.config import Config
from apps.orchestrator.governance import (
    Verdict, check_kill_switch, hunter_score, product_eligible)
from ledger.store import Ledger


@dataclass
class Candidate:
    platform_pid: str
    title: str
    category: str
    price: float            # VND
    commission_rate: float  # 0..1
    seller_rating: float | None
    conversion_rate: float  # estimated, 0..1
    competition: float      # 0..n, higher = more crowded


class TikTokShopAffiliateClient:
    """Thin wrapper over TikTok Shop Open Platform affiliate endpoints.

    TODO: wire endpoints after research (auth, product search, commission).
    """

    def __init__(self, cfg: Config):
        self.cfg = cfg

    def search_products(self, keyword: str, page: int = 1) -> list[dict]:
        raise NotImplementedError("wire after docs/RESEARCH/tiktok_shop_api.md")

    def commission_report(self) -> list[dict]:
        raise NotImplementedError("wire after docs/RESEARCH/tiktok_shop_api.md")


BLOCKED_CATEGORIES = ("thuốc", "thực phẩm chức năng không rõ nguồn gốc")


def run(cfg: Config, ledger: Ledger,
        client: TikTokShopAffiliateClient | None = None) -> dict:
    v = check_kill_switch(cfg)
    if not v.allowed:
        return {"ok": False, "reason": v.reason}
    client = client or TikTokShopAffiliateClient(cfg)

    # TODO: keyword list from config / past winners
    added, rejected = 0, 0
    for keyword in ["gia dụng", "làm đẹp", "thời trang"]:
        try:
            raw = client.search_products(keyword)
        except NotImplementedError as e:
            return {"ok": False, "reason": str(e)}
        for r in raw:
            cand = Candidate(
                platform_pid=r["id"], title=r["title"],
                category=r.get("category", ""), price=r["price"],
                commission_rate=r["commission_rate"],
                seller_rating=r.get("seller_rating"),
                conversion_rate=r.get("conversion_rate", 0.02),
                competition=r.get("competition", 1.0),
            )
            ev: Verdict = product_eligible(
                cfg, cand.price, cand.commission_rate,
                cand.seller_rating, cand.category, BLOCKED_CATEGORIES)
            if not ev.allowed:
                rejected += 1
                continue
            score = hunter_score(cand.price, cand.commission_rate,
                                 cand.conversion_rate, cand.competition)
            pid = ledger.upsert_product(
                cand.platform_pid, title=cand.title, category=cand.category,
                price=cand.price, commission_rate=cand.commission_rate,
                commission_value=cand.price * cand.commission_rate,
                seller_rating=cand.seller_rating, score=score,
                status="shelf")
            ledger.decide("hunter", "shelf_product", str(pid),
                           f"score={score:.0f}", {"keyword": keyword})
            added += 1
    return {"ok": True, "added": added, "rejected": rejected}
