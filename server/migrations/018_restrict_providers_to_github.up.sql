ALTER TABLE environments DROP CONSTRAINT IF EXISTS environments_provider_check;
ALTER TABLE environments
    ADD CONSTRAINT environments_provider_check CHECK (provider = 'github');
