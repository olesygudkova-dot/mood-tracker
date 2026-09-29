CREATE TABLE IF NOT EXISTS users (
    id            SERIAL PRIMARY KEY,
    email         VARCHAR(255) UNIQUE NOT NULL,
    password_hash VARCHAR(255) NOT NULL,
    name          VARCHAR(100),
    created_at    TIMESTAMP DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS mood_entries (
    id          SERIAL PRIMARY KEY,
    user_id     INTEGER REFERENCES users(id) ON DELETE CASCADE,
    entry_date  DATE NOT NULL,
    entry_hour  SMALLINT,
    mood_score  SMALLINT CHECK (mood_score BETWEEN 1 AND 10),
    note        TEXT,
    created_at  TIMESTAMP DEFAULT NOW(),
    updated_at  TIMESTAMP DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_mood_user_date ON mood_entries(user_id, entry_date);

CREATE TABLE IF NOT EXISTS tags (
    id    SERIAL PRIMARY KEY,
    name  VARCHAR(50) UNIQUE NOT NULL,
    color VARCHAR(7)
);

CREATE TABLE IF NOT EXISTS mood_tags (
    mood_entry_id INTEGER REFERENCES mood_entries(id) ON DELETE CASCADE,
    tag_id        INTEGER REFERENCES tags(id) ON DELETE CASCADE,
    PRIMARY KEY (mood_entry_id, tag_id)
);

CREATE TABLE IF NOT EXISTS advice (
    id        SERIAL PRIMARY KEY,
    tag_id    INTEGER REFERENCES tags(id),
    min_score SMALLINT,
    max_score SMALLINT,
    text      TEXT NOT NULL
);

INSERT INTO tags (name, color) VALUES
    ('работа', '#e74c3c'),
    ('учёба', '#e67e22'),
    ('семья', '#f1c40f'),
    ('здоровье', '#2ecc71'),
    ('сон', '#3498db'),
    ('отдых', '#9b59b6'),
    ('спорт', '#1abc9c'),
    ('еда', '#e84393'),
    ('тревога', '#c0392b'),
    ('радость', '#f39c12'),
    ('грусть', '#7f8c8d'),
    ('спокойствие', '#16a085')
ON CONFLICT (name) DO NOTHING;