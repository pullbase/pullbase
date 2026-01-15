CREATE TABLE environments (
    id INTEGER PRIMARY KEY,
    name TEXT NOT NULL UNIQUE,
    repo_url TEXT NOT NULL,
    provider TEXT NOT NULL CHECK (provider = 'github'),
    token TEXT NOT NULL,
    webhook_secret TEXT NOT NULL,
    webhook_id TEXT,
    webhook_url TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'active', 'error')),
    last_webhook_at DATETIME,
    retry_count INTEGER DEFAULT 0,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_environments_repo_url ON environments(repo_url);
CREATE INDEX idx_environments_provider ON environments(provider);
CREATE INDEX idx_environments_status ON environments(status);
