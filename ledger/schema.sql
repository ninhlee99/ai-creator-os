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
    account_id    INTEGER REFERENCES accounts(id),
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

-- ============ AI Creator Network (multi-account) ============

CREATE TABLE IF NOT EXISTS accounts (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    username      TEXT NOT NULL UNIQUE,
    status        TEXT NOT NULL DEFAULT 'onboarding',
    -- onboarding | researching | persona_assigned | growing
    -- | live_ready | live | paused | penalized | retired
    persona       TEXT,               -- storyteller | teacher | musician | gamer | dancer
    niche         TEXT,               -- free-text niche from research/hint
    niche_hint    TEXT,               -- what the human suggested at add time
    topics_json   TEXT NOT NULL DEFAULT '[]',  -- episode topics from topic engine
    youtube_channel TEXT,             -- YouTube channel handle/id for this account
    youtube_content_types TEXT NOT NULL DEFAULT '[]',
    -- JSON list of content kinds this account's YouTube channel accepts:
    -- subset of ["short_video","short_film","ai_music","ai_remix"].
    -- Empty = YouTube disabled for this account.
    followers     INTEGER NOT NULL DEFAULT 0,
    rtmp_key_ref  TEXT,               -- env var NAME holding the RTMP key, never the key itself
    rest_weekday  INTEGER NOT NULL DEFAULT 0,  -- 0=Mon .. 6=Sun
    gift_usd      REAL NOT NULL DEFAULT 0,     -- cached lifetime gift revenue
    created_at    TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at    TEXT NOT NULL DEFAULT (datetime('now'))
);

-- APPEND-ONLY: LIVE gift revenue per account (provider evidence only)
CREATE TABLE IF NOT EXISTS gifts (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    account_id    INTEGER NOT NULL REFERENCES accounts(id),
    session_id    INTEGER REFERENCES live_sessions(id),
    diamonds      INTEGER NOT NULL,
    usd           REAL NOT NULL,      -- diamonds * usd_per_diamond at record time
    recorded_at   TEXT NOT NULL DEFAULT (datetime('now'))
);
CREATE INDEX IF NOT EXISTS idx_gifts_account ON gifts(account_id);

-- scheduled live slots (scheduler output, auditable)
CREATE TABLE IF NOT EXISTS live_slots (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    account_id    INTEGER NOT NULL REFERENCES accounts(id),
    slot_date     TEXT NOT NULL,      -- YYYY-MM-DD (Asia/Ho_Chi_Minh)
    start_min     INTEGER NOT NULL,   -- minutes since midnight ICT
    duration_min  INTEGER NOT NULL,
    status        TEXT NOT NULL DEFAULT 'planned',
    -- planned | started | done | skipped
    created_at    TEXT NOT NULL DEFAULT (datetime('now'))
);
CREATE INDEX IF NOT EXISTS idx_slots_date ON live_slots(slot_date, start_min);
