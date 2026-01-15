-- Drop tables in reverse order
DROP TABLE IF EXISTS audit_log;
DROP TABLE IF EXISTS agent_status;
DROP TABLE IF EXISTS pulls;
DROP TABLE IF EXISTS servers;
DROP TABLE IF EXISTS users;

-- Drop function
DROP FUNCTION IF EXISTS update_updated_at_column(); 