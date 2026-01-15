-- Create agent_tokens table for token-based agent authentication
CREATE TABLE agent_tokens (
    id SERIAL PRIMARY KEY,
    token_hash VARCHAR(128) NOT NULL UNIQUE,
    server_id VARCHAR(255) NOT NULL REFERENCES servers(id) ON DELETE CASCADE,
    description TEXT,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    expires_at TIMESTAMP,
    last_used_at TIMESTAMP,
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    created_by_user_id INTEGER REFERENCES users(id) ON DELETE SET NULL
);

-- Create index for efficient token lookups
CREATE INDEX idx_agent_tokens_token_hash ON agent_tokens(token_hash);
CREATE INDEX idx_agent_tokens_server_id ON agent_tokens(server_id);
CREATE INDEX idx_agent_tokens_active ON agent_tokens(is_active) WHERE is_active = TRUE;

-- Add comments for documentation
COMMENT ON TABLE agent_tokens IS 'Authentication tokens for agents to access the API';
COMMENT ON COLUMN agent_tokens.token_hash IS 'SHA-256 hash of the original token for secure storage';
COMMENT ON COLUMN agent_tokens.server_id IS 'Server this token is associated with';
COMMENT ON COLUMN agent_tokens.description IS 'description of the token';
COMMENT ON COLUMN agent_tokens.expires_at IS 'Optional expiration timestamp for the token';
COMMENT ON COLUMN agent_tokens.last_used_at IS 'Timestamp of last successful authentication with this token';
COMMENT ON COLUMN agent_tokens.is_active IS 'Whether the token is active and can be used for authentication';
COMMENT ON COLUMN agent_tokens.created_by_user_id IS 'User who created this token (for audit trail)'; 