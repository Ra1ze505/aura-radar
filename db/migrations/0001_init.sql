CREATE TABLE IF NOT EXISTS chats (
    chat_id INTEGER PRIMARY KEY,
    title TEXT,
    radar_enabled INTEGER NOT NULL DEFAULT 1,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS users (
    user_id INTEGER PRIMARY KEY,
    username TEXT,
    first_name TEXT,
    last_name TEXT,
    updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS chat_scores (
    chat_id INTEGER NOT NULL,
    user_id INTEGER NOT NULL,
    points INTEGER NOT NULL DEFAULT 0,
    strong_count INTEGER NOT NULL DEFAULT 0,
    weak_count INTEGER NOT NULL DEFAULT 0,
    updated_at TEXT NOT NULL,
    PRIMARY KEY (chat_id, user_id)
);

CREATE TABLE IF NOT EXISTS message_buffer (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    chat_id INTEGER NOT NULL,
    message_id INTEGER NOT NULL,
    user_id INTEGER,
    text TEXT NOT NULL,
    sent_at TEXT NOT NULL,
    UNIQUE (chat_id, message_id)
);

CREATE INDEX IF NOT EXISTS idx_message_buffer_chat_sent
    ON message_buffer (chat_id, sent_at);

CREATE TABLE IF NOT EXISTS aura_events (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    chat_id INTEGER NOT NULL,
    user_id INTEGER NOT NULL,
    message_id INTEGER NOT NULL,
    verdict TEXT NOT NULL,
    delta INTEGER NOT NULL,
    confidence REAL NOT NULL,
    comment TEXT NOT NULL,
    reaction TEXT,
    trigger TEXT NOT NULL,
    published_reaction INTEGER NOT NULL DEFAULT 0,
    published_text INTEGER NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL,
    UNIQUE (chat_id, message_id)
);

CREATE INDEX IF NOT EXISTS idx_aura_events_chat_created
    ON aura_events (chat_id, created_at);
CREATE INDEX IF NOT EXISTS idx_aura_events_chat_user_created
    ON aura_events (chat_id, user_id, created_at);
CREATE INDEX IF NOT EXISTS idx_aura_events_chat_trigger_created
    ON aura_events (chat_id, trigger, created_at);
CREATE INDEX IF NOT EXISTS idx_aura_events_chat_pub_text_created
    ON aura_events (chat_id, published_text, created_at);
