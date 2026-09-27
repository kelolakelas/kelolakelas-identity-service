-- KEL-89: one email address maps to at most one account regardless of letter
-- case. Rows are NOT rewritten: legacy mixed-case addresses stay as stored and
-- the application matches them through LOWER(email), which this index serves.
--
-- Preflight: if two or more accounts share an address that differs only by
-- letter case, stop before creating the index so an operator can resolve the
-- accounts manually. There is no automatic merge or rewrite. The file runs as
-- one implicit transaction, so an abort leaves schema and data unchanged.
-- golang-migrate still records version 11 as dirty. Because nothing was
-- applied, recovery is: resolve the conflicting accounts, then mark version 10
-- clean (golang-migrate CLI `force 10`, or equivalently
-- `UPDATE schema_migrations SET version = 10, dirty = false`) and apply up again.
-- Conflicts are reported by user id, never by address, to keep PII out of
-- deploy logs. List the addresses on the database with the HINT query.
DO $$
DECLARE
    conflict_count integer;
    conflict_ids text;
BEGIN
    SELECT count(*), string_agg(ids, '; ')
      INTO conflict_count, conflict_ids
      FROM (
          SELECT string_agg(id::text, ', ' ORDER BY created_at, id) AS ids
            FROM users
           GROUP BY lower(email)
          HAVING count(*) > 1
      ) AS duplicates;
    IF conflict_count > 0 THEN
        RAISE EXCEPTION 'email case conflict: % email address(es) belong to more than one account when compared case-insensitively; resolve these accounts manually before applying migration 000011', conflict_count
            USING DETAIL = 'conflicting user ids, one group per address: ' || conflict_ids,
                  HINT = 'SELECT lower(email), array_agg(id) FROM users GROUP BY lower(email) HAVING count(*) > 1';
    END IF;
END
$$;

-- Covers soft-deleted rows too, matching the original users_email_key
-- constraint, which stays in place so that rollback is a plain index drop.
CREATE UNIQUE INDEX uq_users_email_lower ON users (lower(email));
