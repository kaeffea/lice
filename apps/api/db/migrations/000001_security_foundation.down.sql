REVOKE ALL PRIVILEGES ON ALL TABLES IN SCHEMA identity, platform, audit FROM lice_runtime;
REVOKE USAGE ON SCHEMA identity, platform, audit FROM lice_runtime;

DROP SCHEMA audit CASCADE;
DROP SCHEMA platform CASCADE;
DROP SCHEMA identity CASCADE;
