-- Repair migration: restore the runtime role's privileges on an already-migrated
-- database.
--
-- 0001 originally granted the runtime role with
--   GRANT ... ON ALL TABLES IN SCHEMA public TO agentdeck_app
-- placed above its own CREATE TABLE statements. That form only matches objects
-- that already exist, so it granted nothing, and every API query failed with
-- "permission denied for table orgs".
--
-- Editing 0001 heals a database migrated from empty, but not one that already
-- recorded versions 1 and 2: Apply never re-runs a recorded version. This
-- migration is the only path that repairs a live installation.
--
-- The runtime role owns nothing and grants nothing; it is only ever granted TO
-- (ARCHITECTURE 3.1 / P6). Keeping it a grantee and never a grantor is what
-- keeps it disposable.

GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA public TO agentdeck_app;
GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA public TO agentdeck_app;

-- Tables created by later migrations need the same privileges without another
-- repair migration. Default privileges are per-grantor: this runs as the schema
-- owner, which is the role every migration runs as.
ALTER DEFAULT PRIVILEGES IN SCHEMA public
    GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO agentdeck_app;
ALTER DEFAULT PRIVILEGES IN SCHEMA public
    GRANT USAGE, SELECT ON SEQUENCES TO agentdeck_app;
