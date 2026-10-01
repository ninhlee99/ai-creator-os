"""Ledger store: SQLite (WAL), append-only money tables."""
from __future__ import annotations

import json
import sqlite3
from pathlib import Path

SCHEMA = Path(__file__).with_name("schema.sql").read_text()


class Ledger:
    def __init__(self, path: str):
        Path(path).parent.mkdir(parents=True, exist_ok=True)
        self.db = sqlite3.connect(path)
        self.db.row_factory = sqlite3.Row
        self.db.executescript(SCHEMA)

    # ---- generic ----
    def _insert(self, table: str, data: dict) -> int:
        cols = ", ".join(data.keys())
        ph = ", ".join("?" for _ in data)
        cur = self.db.execute(
            f"INSERT INTO {table} ({cols}) VALUES ({ph})", tuple(data.values())
        )
        self.db.commit()
        return cur.lastrowid

    # ---- products ----
    def upsert_product(self, platform_pid: str, **fields) -> int:
        row = self.db.execute(
            "SELECT id FROM products WHERE platform_pid = ?", (platform_pid,)
        ).fetchone()
        fields["platform_pid"] = platform_pid
        if row:
            sets = ", ".join(f"{k} = ?" for k in fields if k != "platform_pid")
            self.db.execute(
                f"UPDATE products SET {sets}, updated_at = datetime('now') "
                "WHERE platform_pid = ?",
                tuple(v for k, v in fields.items() if k != "platform_pid")
                + (platform_pid,),
            )
            self.db.commit()
            return row["id"]
        return self._insert("products", fields)

    def set_product_status(self, product_id: int, status: str):
        self.db.execute(
            "UPDATE products SET status = ?, updated_at = datetime('now') "
            "WHERE id = ?",
            (status, product_id),
        )
        self.db.commit()

    def shelf_products(self, limit: int = 20):
        return self.db.execute(
            "SELECT * FROM products WHERE status IN ('shelf','scaled') "
            "ORDER BY score DESC LIMIT ?",
            (limit,),
        ).fetchall()

    # ---- append-only money ----
    def record_order(self, platform_oid: str, **fields) -> int | None:
        """Idempotent: duplicate platform_oid is ignored."""
        try:
            return self._insert("orders", {"platform_oid": platform_oid, **fields})
        except sqlite3.IntegrityError:
            return None

    def record_commission(self, period: str, amount: float,
                          source: str = "tiktok_shop_api") -> int:
        return self._insert(
            "commissions", {"period": period, "amount": amount, "source": source}
        )

    # ---- sessions / events / decisions / usage ----
    def start_session(self) -> int:
        return self._insert("live_sessions", {})

    def end_session(self, session_id: int, **fields):
        sets = ", ".join(f"{k} = ?" for k in fields)
        self.db.execute(
            f"UPDATE live_sessions SET {sets} WHERE id = ?",
            tuple(fields.values()) + (session_id,),
        )
        self.db.commit()

    def log_event(self, session_id: int, kind: str, payload: dict | None = None):
        self._insert(
            "live_events",
            {"session_id": session_id, "kind": kind,
             "payload": json.dumps(payload or {})},
        )

    def decide(self, agent: str, action: str, target: str | None,
               reason: str, inputs: dict | None = None):
        self._insert(
            "decisions",
            {"agent": agent, "action": action, "target": target,
             "reason": reason, "inputs_json": json.dumps(inputs or {})},
        )

    def log_usage(self, engine: str, provider: str, units: float = 1,
                  cost_usd: float = 0):
        self._insert("api_usage", {"engine": engine, "provider": provider,
                                   "units": units, "cost_usd": cost_usd})

    def daily_spend_usd(self) -> float:
        row = self.db.execute(
            "SELECT COALESCE(SUM(cost_usd),0) AS s FROM api_usage "
            "WHERE date(created_at) = date('now')"
        ).fetchone()
        return row["s"]

    # ---- analyst queries ----
    def product_stats(self, product_id: int) -> dict:
        row = self.db.execute(
            "SELECT COUNT(*) AS orders, COALESCE(SUM(amount),0) AS revenue, "
            "COALESCE(SUM(commission),0) AS commission "
            "FROM orders WHERE product_id = ?",
            (product_id,),
        ).fetchone()
        views = self.db.execute(
            "SELECT COALESCE(SUM(views),0) AS v FROM content_items "
            "WHERE product_id = ?",
            (product_id,),
        ).fetchone()
        return {"orders": row["orders"], "revenue": row["revenue"],
                "commission": row["commission"], "views": views["v"]}
