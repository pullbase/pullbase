-- Create agent_tokens table for token-based agent authentication
CREATE TABLE agent_tokens (
    id INTEGER PRIMARY KEY,
    token_hash TEXT NOT NULL UNIQUE,
    server_id TEXT NOT NULL REFERENCES servers(id) ON DELETE CASCADE,
    description TEXT,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    expires_at DATETIME,
    last_used_at DATETIME,
    is_active INTEGER NOT NULL DEFAULT 1,
    created_by_user_id INTEGER REFERENCES users(id) ON DELETE SET NULL
);

-- Create index for efficient token lookups
CREATE INDEX idx_agent_tokens_token_hash ON agent_tokens(token_hash);
CREATE INDEX idx_agent_tokens_server_id ON agent_tokens(server_id);
CREATE INDEX idx_agent_tokens_active ON agent_tokens(is_active) WHERE is_active = 1;
