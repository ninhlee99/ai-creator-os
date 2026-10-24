-- ai-creator-os ledger schema (SQLite, WAL mode)
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
    kind          TEXT NOT NULL,          -- TikTok short video
    script        TEXT,
    media_path    TEXT,
    tiktok_post_id TEXT,
    status        TEXT NOT NULL DEFAULT 'draft',
    -- draft | posted | failed
    views         INTEGER DEFAULT 0,
    created_at    TEXT NOT NULL DEFAULT (datetime('now'))
);

-- APPEND-ONLY: orders
CREATE TABLE IF NOT EXISTS orders (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    platform_oid  TEXT NOT NULL UNIQUE,   -- TikTok Shop order id
    product_id    INTEGER REFERENCES products(id),
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
    -- onboarding | researching | persona_assigned | growing | paused | penalized | retired
    persona       TEXT,               -- storyteller | teacher | musician | gamer | dancer
    niche         TEXT,               -- free-text niche from research/hint
    niche_hint    TEXT,               -- what the human suggested at add time
    topics_json   TEXT NOT NULL DEFAULT '[]',  -- episode topics from topic engine
    followers     INTEGER NOT NULL DEFAULT 0,
    followers_source TEXT NOT NULL DEFAULT 'legacy', -- legacy | tiktok_manual | tiktok_api
    followers_verified_at TEXT NOT NULL DEFAULT '', -- last provider or human verification time
    created_at    TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at    TEXT NOT NULL DEFAULT (datetime('now'))
);

-- dashboard settings (key-value config, e.g. provider chains)
CREATE TABLE IF NOT EXISTS settings (
    key TEXT PRIMARY KEY,
    value TEXT NOT NULL DEFAULT '',
    updated_at TEXT NOT NULL DEFAULT (datetime('now'))
);

-- avatar characters: one visual identity per AI persona. identity_lock =
-- sha256(reference image bytes + seed); a render is refused when the lock
-- does not match the image on disk (prevents silent face drift).
CREATE TABLE IF NOT EXISTS characters (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    name            TEXT NOT NULL,
    reference_image TEXT NOT NULL,   -- path under <data>/avatars/, PNG/JPG
    seed            INTEGER NOT NULL DEFAULT 0,
    identity_lock   TEXT NOT NULL DEFAULT '',
    voice_preset    TEXT NOT NULL DEFAULT '',
    notes           TEXT NOT NULL DEFAULT '',
    created_at      TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
