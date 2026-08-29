package migrate

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ownershipDrift is an object in appSchema owned by something other than the
// role running the migrations.
//
// Such an object is un-replaceable by that role: CREATE OR REPLACE, DROP, and
// ALTER ... OWNER TO all require ownership, and the migrating role is neither the
// owner nor (by design) a superuser. So the moment any migration touches it the
// statement fails, the migration aborts partway, and golang-migrate leaves the
// ledger dirty — after which it refuses every subsequent run.
//
// TRA-1104: that is exactly how preview wedged. trakrf.normalize_tag_value(text)
// had been left owned by postgres; migration 000039 was the first to try to
// replace it. The deploy could not self-recover, because the repair needs rights
// the migrate Job does not have, and because the Job is an ArgoCD PreSync hook the
// Deployment was never updated — the old pod kept serving and ArgoCD reported
// OutOfSync/Healthy rather than a crashloop. It went unnoticed for over an hour.
type ownershipDrift struct {
	kind  string // "table", "view", "function", ... as reported by the catalog
	name  string // already schema-qualified and quoted by the catalog query
	owner string
}

// alterVerb maps an object kind to the ALTER form that changes its owner.
//
// The verb has to match the kind: ALTER TABLE against a view is a syntax error,
// which would turn the one actionable line in the refusal into a second problem
// to debug. An unrecognised kind returns "" and the caller omits the repair line
// rather than inventing invalid DDL — Postgres gains object kinds over time, and
// a wrong statement is worse than none.
func alterVerb(kind string) string {
	switch kind {
	case "table":
		return "ALTER TABLE"
	case "view":
		return "ALTER VIEW"
	case "materialized view":
		return "ALTER MATERIALIZED VIEW"
	case "sequence":
		return "ALTER SEQUENCE"
	case "foreign table":
		return "ALTER FOREIGN TABLE"
	case "function":
		return "ALTER FUNCTION"
	case "procedure":
		return "ALTER PROCEDURE"
	case "aggregate":
		return "ALTER AGGREGATE"
	case "type":
		return "ALTER TYPE"
	case "schema":
		return "ALTER SCHEMA"
	default:
		return ""
	}
}

// ownershipDriftError renders the refusal an operator reads off a failed migrate
// Job. The failed Job is auto-cleaned and takes its pod logs with it, so this
// message is frequently the only surviving evidence — it carries every offending
// object, and the exact statement that repairs each one.
//
// It says plainly that the repair needs superuser or owner rights. Without that
// the obvious next move is to run the ALTER as the migrating role, hit the very
// same "must be owner" error, and conclude the message is wrong.
//
// Like strayLedgerError, it suggests nothing destructive. Failing safe — ledger
// clean, database untouched, old pod still serving — is the point of a preflight,
// and advice that undoes that on being followed would defeat it.
func ownershipDriftError(drifts []ownershipDrift, role string) error {
	quotedRole := pgx.Identifier{role}.Sanitize()

	var b strings.Builder
	fmt.Fprintf(&b, "refusing to migrate: %d object(s) in schema %q are not owned by the "+
		"migrating role %s (TRA-1104)", len(drifts), appSchema, quotedRole)
	for _, d := range drifts {
		fmt.Fprintf(&b, "\n  %s %s: owner=%s", d.kind, d.name, d.owner)
	}

	fmt.Fprintf(&b, "\n  CREATE OR REPLACE, DROP and ALTER ... OWNER TO all require ownership, "+
		"so a migration touching any of the above would abort partway and leave the ledger "+
		"dirty — after which every later migrate run refuses to start.")
	fmt.Fprintf(&b, "\n  The migrating role cannot repair this itself; it is not the owner and "+
		"not a superuser. Run as a superuser (or as the current owner):")

	for _, d := range drifts {
		verb := alterVerb(d.kind)
		if verb == "" {
			fmt.Fprintf(&b, "\n      -- unrecognised object kind %q: transfer %s to %s by hand",
				d.kind, d.name, quotedRole)
			continue
		}
		fmt.Fprintf(&b, "\n      %s %s OWNER TO %s;", verb, d.name, quotedRole)
	}

	fmt.Fprintf(&b, "\n  Then re-run migrate; nothing was applied, so it resumes unchanged.")
	return fmt.Errorf("%s", b.String())
}

// findOwnershipDrift returns objects in appSchema that the current role could not
// replace, or nil when the schema is uniformly owned.
//
// Ownership is not simple equality: Postgres accepts an ownership check when the
// current role is a *member* of the owning role, directly or indirectly. So the
// test is pg_has_role(..., 'USAGE') against the owner rather than owner =
// current_user, otherwise a perfectly workable role hierarchy reports as drift.
//
// That one predicate also covers superusers, which are implicitly members of
// every role — so a superuser sees no drift and needs no special case. An
// explicit rolsuper exemption was written first and then removed: mutation
// testing showed nothing could kill it, because pg_has_role had already answered
// the question. It mattered — while both were present, each masked a defect in
// the other, and the superuser test could not fail. That exemption is not
// cosmetic: local development and the integration harness both migrate as
// postgres, and reporting drift there would break every `just db reset` for a
// condition that cannot affect a superuser.
//
// pg_class covers tables, partitioned tables, views, matviews, sequences and
// foreign tables; pg_proc covers functions, procedures and aggregates; pg_type
// covers standalone enum/domain/composite types, excluding the row types that
// pg_class entries create implicitly (those follow their relation's ownership and
// would double-report). The schema itself is included — an unowned schema blocks
// CREATE just as surely as an unowned table blocks REPLACE.
//
// Objects belonging to an EXTENSION are excluded (pg_depend deptype 'e'),
// TRA-1190. They are not drift, and no repair can make them stop looking like
// it: pgcrypto is a trusted extension, so `CREATE EXTENSION pgcrypto` succeeds
// for a non-superuser and Postgres nevertheless assigns every resulting object
// to the bootstrap superuser. Migration 000001 creates it inside the trakrf
// schema, so every database migrated by trakrf-migrate carries 36 permanently
// postgres-owned functions there, by design and beyond the role's reach.
//
// Including them made this preflight fire on every local database forever:
// `just backend migrate` worked once on a fresh database and refused every run
// afterwards — including as a no-op, since the preflight precedes golang-migrate
// deciding there is nothing to do. `just dev` migrates each time, so the second
// `just dev` failed. That was invisible only because PG_URL_MIGRATE_LOCAL was
// itself unset, so the command had never run at all.
//
// The exclusion gives up nothing this guard protects. A migration never
// CREATE OR REPLACEs an extension member — the extension owns its definitions —
// so such an object cannot produce the half-applied migration and dirty ledger
// the preflight exists to prevent. Genuine drift on a non-extension object is
// still caught, which the integration suite asserts alongside this.
func findOwnershipDrift(ctx context.Context, pool *pgxpool.Pool) ([]ownershipDrift, error) {
	const q = `
SELECT kind, name, owner FROM (
    SELECT CASE c.relkind
               WHEN 'r' THEN 'table'
               WHEN 'p' THEN 'table'
               WHEN 'v' THEN 'view'
               WHEN 'm' THEN 'materialized view'
               WHEN 'S' THEN 'sequence'
               WHEN 'f' THEN 'foreign table'
           END                                            AS kind,
           format('%I.%I', n.nspname, c.relname)          AS name,
           pg_get_userbyid(c.relowner)                    AS owner,
           c.relowner                                     AS ownerid
      FROM pg_class c
      JOIN pg_namespace n ON n.oid = c.relnamespace
     WHERE n.nspname = $1
       AND c.relkind IN ('r', 'p', 'v', 'm', 'S', 'f')
       AND NOT EXISTS (
             SELECT 1 FROM pg_depend d
              WHERE d.classid = 'pg_class'::regclass
                AND d.objid = c.oid
                AND d.deptype = 'e')

    UNION ALL

    SELECT CASE p.prokind
               WHEN 'f' THEN 'function'
               WHEN 'p' THEN 'procedure'
               WHEN 'a' THEN 'aggregate'
               WHEN 'w' THEN 'function'
           END,
           -- NOT regprocedure: it drops the schema whenever that schema is on
           -- the caller's search_path, and the runner always puts trakrf there.
           -- The repair line is pasted into a session with a different path, so
           -- the qualification has to be unconditional.
           format('%I.%I(%s)', n.nspname, p.proname,
                  pg_get_function_identity_arguments(p.oid)),
           pg_get_userbyid(p.proowner),
           p.proowner
      FROM pg_proc p
      JOIN pg_namespace n ON n.oid = p.pronamespace
     WHERE n.nspname = $1
       AND NOT EXISTS (
             SELECT 1 FROM pg_depend d
              WHERE d.classid = 'pg_proc'::regclass
                AND d.objid = p.oid
                AND d.deptype = 'e')

    UNION ALL

    SELECT 'type',
           format('%I.%I', n.nspname, t.typname),
           pg_get_userbyid(t.typowner),
           t.typowner
      FROM pg_type t
      JOIN pg_namespace n ON n.oid = t.typnamespace
     WHERE n.nspname = $1
       AND t.typtype IN ('e', 'd', 'c')
       AND NOT EXISTS (
             SELECT 1 FROM pg_depend d
              WHERE d.classid = 'pg_type'::regclass
                AND d.objid = t.oid
                AND d.deptype = 'e')
       AND NOT EXISTS (
             SELECT 1 FROM pg_class c
              WHERE c.reltype = t.oid
                AND c.relkind IN ('r', 'p', 'v', 'm', 'S', 'f'))

    UNION ALL

    SELECT 'schema',
           format('%I', n.nspname),
           pg_get_userbyid(n.nspowner),
           n.nspowner
      FROM pg_namespace n
     WHERE n.nspname = $1
) objects
WHERE NOT pg_has_role(CURRENT_USER, ownerid, 'USAGE')
ORDER BY kind, name`

	rows, err := pool.Query(ctx, q, appSchema)
	if err != nil {
		return nil, fmt.Errorf("failed to audit object ownership in schema %s: %w", appSchema, err)
	}
	defer rows.Close()

	var drifts []ownershipDrift
	for rows.Next() {
		var d ownershipDrift
		if err := rows.Scan(&d.kind, &d.name, &d.owner); err != nil {
			return nil, fmt.Errorf("failed to read object ownership audit: %w", err)
		}
		drifts = append(drifts, d)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to audit object ownership in schema %s: %w", appSchema, err)
	}
	return drifts, nil
}

// currentRole reports the role the migrations will actually run as, for the
// refusal message. Falls back to a placeholder rather than failing the preflight
// on a cosmetic lookup.
func currentRole(ctx context.Context, pool *pgxpool.Pool) string {
	var role string
	if err := pool.QueryRow(ctx, "SELECT CURRENT_USER").Scan(&role); err != nil {
		return "the migrating role"
	}
	return role
}
