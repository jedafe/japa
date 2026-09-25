#!/usr/bin/env python3
"""Regenerate the squashed baseline migration pair.

Pipeline:
  1. pg_dump the fully-migrated dev database (schema + data)
  2. Restore it into a scratch database (japa_baseline_build)
  3. Prune the scratch copy: wipe every table in
     baseline_seed_exclusions.txt, trim users to the system set
     (intel bots, GDPR deleted-user placeholder, seeded super admin),
     and repair foreign keys — nullable dangling refs are NULLed,
     authorship columns are remapped to the super admin, ownership-bound
     rows are deleted
  4. Emit migrations/<N>_baseline_schema.up.sql (schema only) and
     migrations/<N>_baseline_seed.up.sql (pruned seed data, loaded with
     session_replication_role=replica for the circular FK graph)
  5. Verify: apply both files to a throwaway database and diff the
     resulting schema against the source

Usage:  JC_DATABASE_URL=postgresql://... python3 scripts/gen_baseline.py
        (or run from the repo root with .env present — it is read as a
        fallback; PG* tooling must be on PATH)

After regenerating, INSERT the two new filenames into schema_migrations
on every already-migrated database so the runner skips them.
"""

import os
import re
import subprocess
import sys
import tempfile

REPO = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
MANIFEST = os.path.join(REPO, "scripts", "baseline_seed_exclusions.txt")
MIGRATIONS = os.path.join(REPO, "migrations")
SCRATCH_DB = "japa_baseline_build"
VERIFY_DB = "japa_baseline_verify"

# Columns whose values describe WHO created/reviewed a row, not WHO owns it.
# Dangling values are remapped to the super admin rather than deleting the row.
AUTHORSHIP_COLS = {
    "author_user_id", "author_id", "created_by", "submitted_by", "edited_by",
    "reviewed_by", "reviewer_id", "reviewer_user_id", "hitl_reviewed_by",
    "sent_by", "uploaded_by", "updated_by", "curated_by", "added_by",
    "moderated_by", "approved_by",
}

SUPER_ADMIN_EMAIL = "jedafeglobal@gmail.com"
# bcrypt hash documented in archived migration 000124 (seed credentials).
SUPER_ADMIN_SEED_HASH = "$2a$12$4jeCRM3aq48/gvkFFvDkeOdylJbEQB8S8Nmii0OxWupn8DEWog3x6"
GDPR_PLACEHOLDER_ID = "ffffffff-0000-0000-0000-000000000001"


def sh(args, **kw):
    kw.setdefault("capture_output", True)
    kw.setdefault("text", True)
    return subprocess.run(args, check=True, **kw)


def psql(url, sql):
    return sh(["psql", url, "-v", "ON_ERROR_STOP=1", "-tAc", sql]).stdout.strip()


def load_db_url():
    url = os.environ.get("JC_DATABASE_URL")
    if url:
        return url
    env = os.path.join(REPO, ".env")
    for line in open(env):
        m = re.match(r"JC_DATABASE_URL=['\"]?([^'\"\n]+)", line)
        if m:
            return m.group(1)
    sys.exit("JC_DATABASE_URL not set and not found in .env")


def db_url_for(base, name):
    return re.sub(r"/[^/?]+\?", f"/{name}?", base)


def load_manifest():
    out = set()
    for line in open(MANIFEST):
        line = line.strip()
        if line and not line.startswith("#"):
            out.add(line)
    out.add("schema_migrations")
    return out


def fetch_fks(url):
    rows = psql(url, """
        SELECT conrelid::regclass::text || '|' || confrelid::regclass::text || '|' ||
               (SELECT string_agg(a.attname||':'||(a.attnotnull::text), ',' ORDER BY u.ord)
                FROM unnest(con.conkey) WITH ORDINALITY u(attnum,ord)
                JOIN pg_attribute a ON a.attrelid=con.conrelid AND a.attnum=u.attnum) || '|' ||
               (SELECT string_agg(a.attname, ',' ORDER BY u.ord)
                FROM unnest(con.confkey) WITH ORDINALITY u(attnum,ord)
                JOIN pg_attribute a ON a.attrelid=con.confrelid AND a.attnum=u.attnum)
        FROM pg_constraint con WHERE con.contype='f'
        AND con.connamespace = 'public'::regnamespace ORDER BY 1
    """).splitlines()
    return [tuple(r.split("|")) for r in rows if r]


def gen_prune_sql(excl, fks):
    out = ["SET session_replication_role='replica';", ""]
    for t in sorted(excl):
        out.append(f'DELETE FROM "{t}";')
    out += [
        "",
        "-- trim users to the system set and restore documented seed credentials",
        f"DELETE FROM users WHERE NOT (is_bot OR status='deleted' OR email='{SUPER_ADMIN_EMAIL}');",
        f"UPDATE users SET password_hash='{SUPER_ADMIN_SEED_HASH}' WHERE email='{SUPER_ADMIN_EMAIL}';",
        "-- ensure the GDPR deleted-user placeholder exists",
        "INSERT INTO users (id, email, username, status, profile_type) VALUES "
        f"('{GDPR_PLACEHOLDER_ID}','deleted-user@system.japaconnect.local','Deleted User','deleted','regular') "
        "ON CONFLICT (id) DO NOTHING;",
        "",
    ]
    for child, parent, cspec, pspec in fks:
        if child in excl:
            continue
        cols = [tuple(c.split(":")) for c in cspec.split(",")]
        pcols = pspec.split(",")
        all_nullable = all(nn == "false" for _, nn in cols)
        cnames = [c for c, _ in cols]
        if len(cols) == 1:
            dangle = (f'c."{cnames[0]}" IS NOT NULL AND NOT EXISTS '
                      f'(SELECT 1 FROM "{parent}" p WHERE p."{pcols[0]}" = c."{cnames[0]}")')
        else:
            nonnull = " AND ".join(f'c."{c}" IS NOT NULL' for c in cnames)
            match = " AND ".join(f'p."{p}" = c."{c}"' for c, p in zip(cnames, pcols))
            dangle = f'{nonnull} AND NOT EXISTS (SELECT 1 FROM "{parent}" p WHERE {match})'
        if all_nullable:
            setnull = ", ".join(f'"{c}" = NULL' for c in cnames)
            out.append(f'UPDATE "{child}" c SET {setnull} WHERE {dangle};')
        elif len(cols) == 1 and parent == "users" and cnames[0] in AUTHORSHIP_COLS:
            out.append(f'UPDATE "{child}" c SET "{cnames[0]}" = (SELECT id FROM users '
                       f"WHERE email='{SUPER_ADMIN_EMAIL}') WHERE {dangle};")
        else:
            out.append(f'DELETE FROM "{child}" c WHERE {dangle};')
    out += ["", "SET session_replication_role='origin';"]
    return "\n".join(out) + "\n"


def strip_psql_meta(text):
    """Remove \\restrict/\\unrestrict psql meta-commands (pg_dump 15.17+)."""
    return "\n".join(l for l in text.split("\n")
                     if not l.startswith("\\restrict") and not l.startswith("\\unrestrict"))


def strip_dump_object(text, name):
    """Remove every dumped block for object `name` (comment header + statement,
    or an empty TABLE DATA comment section) from a pg_dump plain-text dump."""
    lines = text.split("\n")
    out = []
    i = 0
    while i < len(lines):
        l = lines[i]
        is_obj = (l == f"CREATE TABLE public.{name} ("
                  or l == f"ALTER TABLE ONLY public.{name}"
                  or l == f"-- Data for Name: {name}; Type: TABLE DATA; Schema: public; Owner: -")
        if not is_obj:
            out.append(l)
            i += 1
            continue
        # statement block: consume to the terminating ";" line
        if not l.startswith("-- Data for Name:"):
            while i < len(lines) and not lines[i].rstrip().endswith(";"):
                i += 1
            i += 1  # skip the ";" line
        else:
            i += 1  # comment line only
        # strip trailing comment closer ("--") and blanks after the block
        while i < len(lines) and lines[i] in ("--", ""):
            i += 1
        # strip the comment header we already emitted ("--", "-- Name:", "--")
        while out and out[-1] in ("--", "") or (out and out[-1].startswith("-- Name:")):
            out.pop()
    return "\n".join(out)


def baseline_number():
    """Reuse the existing baseline prefix if present, else max+1 across
    migrations/ and migrations/archive/."""
    for f in os.listdir(MIGRATIONS):
        m = re.match(r"(\d{6})_baseline_schema\.up\.sql$", f)
        if m:
            return int(m.group(1))
    nums = []
    for d in (MIGRATIONS, os.path.join(MIGRATIONS, "archive")):
        if os.path.isdir(d):
            nums += [int(f.split("_")[0]) for f in os.listdir(d) if f[:6].isdigit()]
    return max(nums) + 1


def main():
    url = load_db_url()
    scratch = db_url_for(url, SCRATCH_DB)
    verify = db_url_for(url, VERIFY_DB)
    excl = load_manifest()
    n = baseline_number()

    other = [f for f in os.listdir(MIGRATIONS)
             if f.endswith(".sql") and "_baseline_" not in f]
    if other:
        print(f"NOTE: {len(other)} non-baseline migration files are present in "
              f"migrations/. Move them to migrations/archive/ before or after "
              f"regenerating — the runner applies them before the baseline.")

    with tempfile.TemporaryDirectory() as td:
        full = f"{td}/full.sql"
        schema_f = f"{td}/schema.sql"
        seed_f = f"{td}/seed.sql"

        print("== dumping source database")
        sh(["pg_dump", url, "--no-owner", "--no-privileges", "--no-tablespaces", "-f", full])

        print("== restoring to scratch db")
        sh(["dropdb", "--if-exists", SCRATCH_DB])
        sh(["createdb", SCRATCH_DB])
        sh(["psql", scratch, "-v", "ON_ERROR_STOP=1", "-q", "-f", full])

        print("== pruning scratch db")
        fks = fetch_fks(url)
        sh(["psql", scratch, "-v", "ON_ERROR_STOP=1", "-q"],
           input=gen_prune_sql(excl, fks))

        print("== dumping schema and pruned seed data")
        sh(["pg_dump", url, "--schema-only", "--no-owner", "--no-privileges",
            "--no-tablespaces", "-f", schema_f])
        sh(["pg_dump", scratch, "--data-only", "--inserts", "--no-owner",
            "--no-privileges", "--no-blobs", "-f", seed_f])
        schema = strip_dump_object(strip_psql_meta(open(schema_f).read()), "schema_migrations")
        seed = strip_psql_meta(open(seed_f).read())

        schema_name = f"{n:06d}_baseline_schema"
        seed_name = f"{n:06d}_baseline_seed"
        today = os.popen("date +%F").read().strip()

        open(f"{MIGRATIONS}/{schema_name}.up.sql", "w").write(
            f"""-- {schema_name}.up.sql
-- Squashed schema baseline (regenerated {today} by scripts/gen_baseline.py).
-- Replaces the historical migration chain (archived in migrations/archive/).
-- Pair with {seed_name}.up.sql, which loads the platform reference/seed data.

""" + schema + "\nSET search_path = public;\n")

        seeded_tables = sorted({m.group(1) for m in
                                (re.match(r"INSERT INTO public\.(\w+)", l) for l in seed.split("\n"))
                                if m})
        open(f"{MIGRATIONS}/{seed_name}.up.sql", "w").write(
            f"""-- {seed_name}.up.sql
-- Squashed seed baseline. Applies AFTER {schema_name}.up.sql.
-- Platform-owned reference data only; user-generated/session/analytics
-- tables are intentionally empty. See scripts/baseline_seed_exclusions.txt.
-- session_replication_role=replica defers FK checks for the circular
-- users<->spaces / ai_personas constraint graph; rows are FK-consistent.

SET session_replication_role = 'replica';

""" + seed + "\nSET session_replication_role = 'origin';\nSET search_path = public;\n")

        open(f"{MIGRATIONS}/{schema_name}.down.sql", "w").write(
            f"""-- {schema_name}.down.sql
-- Reverts the squashed baseline by dropping the entire public schema.

BEGIN;
DROP SCHEMA public CASCADE;
CREATE SCHEMA public;
GRANT ALL ON SCHEMA public TO public;
COMMIT;
""")
        open(f"{MIGRATIONS}/{seed_name}.down.sql", "w").write(
            f"""-- {seed_name}.down.sql
-- Removes the baseline seed data (schema objects remain).

BEGIN;
SET session_replication_role = 'replica';
""" + "\n".join(f'DELETE FROM "{t}";' for t in seeded_tables) +
            "\nSET session_replication_role = 'origin';\nCOMMIT;\n")

        print("== verifying on throwaway db via the real migrator")
        sh(["dropdb", "--if-exists", VERIFY_DB])
        sh(["createdb", VERIFY_DB])
        env = dict(os.environ, JC_DATABASE_URL=verify)
        r = subprocess.run(["go", "run", "./cmd/migrate"], cwd=REPO, env=env,
                           capture_output=True, text=True)
        if r.returncode != 0:
            print(r.stderr[-3000:])
            sys.exit("migrator failed against the fresh database")
        vs = f"{td}/verify_schema.sql"
        sh(["pg_dump", verify, "--schema-only", "--no-owner", "--no-privileges",
            "--no-tablespaces", "-f", vs])
        vs_clean = strip_dump_object(strip_psql_meta(open(vs).read()), "schema_migrations")
        if schema == vs_clean:
            print("schema diff: IDENTICAL")
        else:
            open(f"{td}/a.sql", "w").write(schema)
            open(f"{td}/b.sql", "w").write(vs_clean)
            print(subprocess.run(["diff", f"{td}/a.sql", f"{td}/b.sql"],
                                 capture_output=True, text=True).stdout[:4000])

        sh(["dropdb", "--if-exists", VERIFY_DB])
        sh(["dropdb", "--if-exists", SCRATCH_DB])

    print(f"""
Done: migrations/{schema_name}.up.sql + {seed_name}.up.sql (+ .down pair).

Next steps on every already-migrated database:
    INSERT INTO schema_migrations (filename) VALUES
        ('{schema_name}.up.sql'), ('{seed_name}.up.sql')
    ON CONFLICT (filename) DO NOTHING;
""")


if __name__ == "__main__":
    main()
