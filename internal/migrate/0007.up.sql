-- M1 — US-AD09 AC2: a duplicate board NAME inside one project is a 409.
--
-- The DDL in 0004 made only the slug unique (boards_project_slug_key), so
-- creating "Sprint 24" twice in the same project with different slugs succeeded.
-- The acceptance criterion names the *name*: "Nama board duplikat dalam satu
-- project mengembalikan 409". The agents table already carries exactly this
-- index for the same criterion (agents_project_name_key, US-AD20/US-AD67 AC3),
-- so boards were simply missing their counterpart.
--
-- Postgres stays the arbiter rather than a pre-flight SELECT, because a SELECT
-- followed by an INSERT races two concurrent creates (the same reasoning as the
-- slug mapping in internal/board/pgx.go).
--
-- Existing duplicates must be disambiguated before the index can exist. Rows are
-- never deleted: the oldest row keeps the plain name and each later collision is
-- suffixed with the last 6 characters of its ULID, which is stable, unique, and
-- leaves the original text readable. No other column is touched.
WITH ranked AS (
    SELECT id,
           name,
           row_number() OVER (PARTITION BY project_id, name ORDER BY created_at, id) AS rn
    FROM boards
)
UPDATE boards b
SET name = b.name || ' (' || right(b.id, 6) || ')'
FROM ranked r
WHERE b.id = r.id
  AND r.rn > 1;

CREATE UNIQUE INDEX IF NOT EXISTS boards_project_name_key ON boards (project_id, name);
