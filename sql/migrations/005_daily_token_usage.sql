CREATE TABLE IF NOT EXISTS daily_token_usage (
    user_id    UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    usage_date DATE NOT NULL DEFAULT CURRENT_DATE,
    tokens_used INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (user_id, usage_date)
);
