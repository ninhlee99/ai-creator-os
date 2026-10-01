-- tiktok-affiliate-os ledger schema (SQLite, WAL mode)
-- Money tables are append-only: never UPDATE/DELETE orders, commissions.

PRAGMA journal_mode=WAL;

CREATE TABLE IF NOT EXISTS products (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    platform_pid  TEXT NOT NULL UNIQUE,   -- TikTok Shop product id
    title         TEXT NOT NULL,
    category      TEXT,
    price         REAL NOT NULL,
    commission_rate REAL NOT NULL,        -- 0..1
    commission_value REAL NOT NULL,       -- price * rate
    seller_rating REAL,
    score         REAL DEFAULT 0,         -- hunter score
    status        TEXT NOT NULL DEFAULT 'candidate',
    -- candidate | shelf | killed | scaled
    created_at    TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at    TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS content_items (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    product_id    INTEGER REFERENCES products(id),
    kind          TEXT NOT NULL,          -- short_video | live_segment
    script        TEXT,
    media_path    TEXT,
    tiktok_post_id TEXT,
    status        TEXT NOT NULL DEFAULT 'draft',
    -- draft | posted | failed
    views         INTEGER DEFAULT 0,
    created_at    TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS live_sessions (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    started_at    TEXT NOT NULL DEFAULT (datetime('now')),
    ended_at      TEXT,
    duration_min  INTEGER DEFAULT 0,
    peak_viewers  INTEGER DEFAULT 0,
    status        TEXT NOT NULL DEFAULT 'running',
    -- running | ended | killed | failed
    notes         TEXT
);

CREATE TABLE IF NOT EXISTS live_events (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    session_id    INTEGER NOT NULL REFERENCES live_sessions(id),
    kind          TEXT NOT NULL,          -- segment | product_moment | gift | follow | error
    payload       TEXT,                   -- JSON
    created_at    TEXT NOT NULL DEFAULT (datetime('now'))
);
CREATE INDEX IF NOT EXISTS idx_live_events_session ON live_events(session_id);

-- APPEND-ONLY: orders
CREATE TABLE IF NOT EXISTS orders (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    platform_oid  TEXT NOT NULL UNIQUE,   -- TikTok Shop order id
    product_id    INTEGER REFERENCES products(id),
    session_id    INTEGER REFERENCES live_sessions(id),
    amount        REAL NOT NULL,
    commission    REAL NOT NULL,
    ordered_at    TEXT NOT NULL,
    recorded_at   TEXT NOT NULL DEFAULT (datetime('now'))
);

-- APPEND-ONLY: commission settlements from provider
CREATE TABLE IF NOT EXISTS commissions (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    period        TEXT NOT NULL,          -- e.g. 2026-10
    amount        REAL NOT NULL,
    source        TEXT NOT NULL DEFAULT 'tiktok_shop_api',
    recorded_at   TEXT NOT NULL DEFAULT (datetime('now'))
);

-- every governance decision, with inputs (audit trail)
CREATE TABLE IF NOT EXISTS decisions (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    agent         TEXT NOT NULL,
    action        TEXT NOT NULL,          -- kill_product | scale_product | approve | deny ...
    target        TEXT,
    reason        TEXT,
    inputs_json   TEXT,
    created_at    TEXT NOT NULL DEFAULT (datetime('now'))
);

-- engine usage metering (free-tier caps)
CREATE TABLE IF NOT EXISTS api_usage (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    engine        TEXT NOT NULL,          -- llm | tts | avatar | tiktok
    provider      TEXT NOT NULL,
    units         REAL NOT NULL DEFAULT 1,
    cost_usd      REAL NOT NULL DEFAULT 0,
    created_at    TEXT NOT NULL DEFAULT (datetime('now'))
);
CREATE INDEX IF NOT EXISTS idx_api_usage_day ON api_usage(engine, created_at);
