"""Central configuration: env vars only, typed, validated once."""
from __future__ import annotations

import os
from dataclasses import dataclass


def _get(name: str, default: str = "") -> str:
    return os.environ.get(name, default)


def _get_float(name: str, default: float) -> float:
    try:
        return float(_get(name, str(default)))
    except ValueError:
        return default


def _get_int(name: str, default: int) -> int:
    try:
        return int(_get(name, str(default)))
    except ValueError:
        return default


def _get_bool(name: str, default: bool = False) -> bool:
    return _get(name, str(default)).lower() in ("1", "true", "yes")


@dataclass(frozen=True)
class Config:
    app_env: str = _get("APP_ENV", "development")
    dry_run: bool = _get_bool("DRY_RUN", True)
    database_path: str = _get("DATABASE_PATH", "./data/ledger.db")
    timezone: str = _get("TIMEZONE", "Asia/Ho_Chi_Minh")

    # LLM chain
    gemini_api_key: str = _get("GEMINI_API_KEY")
    ollama_base_url: str = _get("OLLAMA_BASE_URL", "http://localhost:11434")
    ollama_model: str = _get("OLLAMA_MODEL", "qwen3:4b")

    # TTS / avatar
    tts_api_key: str = _get("TTS_API_KEY")
    avatar_provider: str = _get("AVATAR_PROVIDER", "local-stylized")
    avatar_api_key: str = _get("AVATAR_API_KEY")

    # TikTok
    tiktok_shop_app_key: str = _get("TIKTOK_SHOP_APP_KEY")
    tiktok_shop_app_secret: str = _get("TIKTOK_SHOP_APP_SECRET")
    tiktok_shop_access_token: str = _get("TIKTOK_SHOP_ACCESS_TOKEN")
    tiktok_shop_cipher: str = _get("TIKTOK_SHOP_CIPHER")
    tiktok_rtmp_url: str = _get("TIKTOK_RTMP_URL")
    tiktok_rtmp_key: str = _get("TIKTOK_RTMP_KEY")
    ai_disclosure_text: str = _get("AI_DISCLOSURE_TEXT", "AI-generated stream")

    # governance
    daily_api_budget_usd: float = _get_float("DAILY_API_BUDGET_USD", 5.0)
    max_live_minutes_per_session: int = _get_int("MAX_LIVE_MINUTES_PER_SESSION", 120)
    kill_switch: bool = _get_bool("KILL_SWITCH", False)

    # hunter / analyst tuning
    min_seller_rating: float = _get_float("MIN_SELLER_RATING", 4.0)
    max_price: float = _get_float("MAX_PRICE", 1_000_000)  # VND
    kill_views_no_order: int = _get_int("KILL_VIEWS_NO_ORDER", 10_000)
    kill_sessions_no_order: int = _get_int("KILL_SESSIONS_NO_ORDER", 3)

    # alerting
    telegram_bot_token: str = _get("TELEGRAM_BOT_TOKEN")
    telegram_chat_id: str = _get("TELEGRAM_CHAT_ID")

    @property
    def live_enabled(self) -> bool:
        return not self.dry_run and not self.kill_switch

    def validate_for_live(self) -> list[str]:
        """Return list of blockers; empty means clear to go live."""
        blockers = []
        if self.kill_switch:
            blockers.append("KILL_SWITCH is on")
        if not self.tiktok_shop_access_token:
            blockers.append("missing TIKTOK_SHOP_ACCESS_TOKEN")
        if not self.tiktok_rtmp_url or not self.tiktok_rtmp_key:
            blockers.append("missing TIKTOK_RTMP_URL/KEY")
        if not self.tts_api_key:
            blockers.append("missing TTS_API_KEY (Gemini; Edge fallback needs no key)")
        return blockers


config = Config()
