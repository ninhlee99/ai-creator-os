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
    def start_session(self, account_id: int | None = None) -> int:
        if account_id is None:
            self.db.execute("INSERT INTO live_sessions DEFAULT VALUES")
        else:
            self.db.execute(
                "INSERT INTO live_sessions (account_id) VALUES (?)",
                (account_id,),
            )
        self.db.commit()
        return self.db.execute("SELECT last_insert_rowid()").fetchone()[0]

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

    # ---- accounts (AI Creator Network) ----
    def add_account(self, username: str, niche_hint: str = "",
                    rtmp_key_ref: str = "") -> int:
        """Register a new TikTok account. Returns account id.
        The RTMP key itself is NEVER stored here — only the env var name."""
        try:
            return self._insert("accounts", {
                "username": username,
                "niche_hint": niche_hint,
                "rtmp_key_ref": rtmp_key_ref,
                "rest_weekday": self.db.execute(
                    "SELECT COUNT(*) FROM accounts").fetchone()[0] % 7,
            })
        except sqlite3.IntegrityError:
            row = self.db.execute(
                "SELECT id FROM accounts WHERE username = ?", (username,)
            ).fetchone()
            return row["id"]

    def get_account(self, account_id: int):
        return self.db.execute(
            "SELECT * FROM accounts WHERE id = ?", (account_id,)).fetchone()

    def list_accounts(self, statuses: tuple | None = None):
        if statuses:
            ph = ",".join("?" for _ in statuses)
            return self.db.execute(
                f"SELECT * FROM accounts WHERE status IN ({ph}) "
                "ORDER BY id", statuses).fetchall()
        return self.db.execute("SELECT * FROM accounts ORDER BY id").fetchall()

    def set_account_status(self, account_id: int, status: str,
                           **fields) -> None:
        fields["status"] = status
        sets = ", ".join(f"{k} = ?" for k in fields)
        self.db.execute(
            f"UPDATE accounts SET {sets}, updated_at = datetime('now') "
            "WHERE id = ?", tuple(fields.values()) + (account_id,))
        self.db.commit()

    def set_followers(self, account_id: int, followers: int) -> None:
        self.db.execute(
            "UPDATE accounts SET followers = ?, updated_at = datetime('now') "
            "WHERE id = ?", (followers, account_id))
        self.db.commit()

    # ---- gifts (append-only, provider evidence) ----
    def record_gift(self, account_id: int, diamonds: int, usd: float,
                    session_id: int | None = None) -> int:
        gid = self._insert("gifts", {
            "account_id": account_id, "session_id": session_id,
            "diamonds": diamonds, "usd": usd})
        self.db.execute(
            "UPDATE accounts SET gift_usd = gift_usd + ? WHERE id = ?",
            (usd, account_id))
        self.db.commit()
        return gid

    def account_gift_usd(self, account_id: int) -> float:
        row = self.db.execute(
            "SELECT COALESCE(SUM(usd),0) AS s FROM gifts WHERE account_id = ?",
            (account_id,)).fetchone()
        return row["s"]

    # ---- live slots ----
    def save_slots(self, slots: list[dict]) -> None:
        for s in slots:
            self._insert("live_slots", s)

    def due_slots(self, slot_date: str, now_min: int):
        """Slots planned for today whose start time has arrived."""
        return self.db.execute(
            "SELECT * FROM live_slots WHERE slot_date = ? "
            "AND status = 'planned' AND start_min <= ? "
            "ORDER BY start_min", (slot_date, now_min)).fetchall()

    def set_slot_status(self, slot_id: int, status: str) -> None:
        self.db.execute("UPDATE live_slots SET status = ? WHERE id = ?",
                        (status, slot_id))
        self.db.commit()
