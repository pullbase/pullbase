-- Drop indexes first
DROP INDEX IF EXISTS idx_agent_tokens_active;
DROP INDEX IF EXISTS idx_agent_tokens_server_id;
DROP INDEX IF EXISTS idx_agent_tokens_token_hash;
 
-- Drop the agent_tokens table
DROP TABLE IF EXISTS agent_tokens; 