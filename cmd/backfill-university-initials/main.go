// Command backfill-university-initials recomputes the initials custom field
// value for every existing university space using the latest DeriveInitials
// logic. It reads the official_website_url custom field (when present) and the
// university name, then upserts the initials value.
//
// Run as a one-off after updating the curated override maps or the derivation
// algorithm. It intentionally overwrites existing initials so the improved
// derivation takes effect; admin-entered values should be preserved by running
// this only before production launch or after a deliberate override refresh.
// Ref: japaconnect-engineering/sections/39-space-types-university.md
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/universities"
)

const batchSize = 500

type backfillEntry struct {
	SpaceID  pgtype.UUID
	TenantID pgtype.UUID
	Name     string
	Website  string
}

func main() {
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	dbURL := resolveDatabaseURL()

	conn, err := pgx.Connect(ctx, dbURL)
	if err != nil {
		slog.Error("failed to connect to database", "error", err)
		os.Exit(1)
	}
	defer conn.Close(ctx)

	// Find the deterministic initials field ID. We look it up by key for safety.
	var initialsFieldID pgtype.UUID
	if err := conn.QueryRow(ctx, `
		SELECT id FROM custom_fields WHERE key = 'initials' LIMIT 1
	`).Scan(&initialsFieldID); err != nil {
		// Fall back to the migration 000288 deterministic UUID.
		if serr := initialsFieldID.Scan("00000000-0000-0000-0000-000000000090"); serr != nil {
			slog.Error("failed to resolve initials field id", "error", serr)
			os.Exit(1)
		}
	}

	var websiteFieldID pgtype.UUID
	if err := conn.QueryRow(ctx, `
		SELECT id FROM custom_fields WHERE key = 'official_website_url' LIMIT 1
	`).Scan(&websiteFieldID); err != nil {
		if serr := websiteFieldID.Scan("00000000-0000-0000-0000-000000000091"); serr != nil {
			slog.Error("failed to resolve website field id", "error", serr)
			os.Exit(1)
		}
	}

	rows, err := conn.Query(ctx, `
		SELECT s.id, s.tenant_id, s.name, COALESCE(cfv.value_text, '') AS website
		FROM spaces s
		JOIN space_types st ON st.id = s.space_type_id
		LEFT JOIN custom_field_values cfv ON cfv.entity_id = s.id
		  AND cfv.entity_type = 'space'
		  AND cfv.field_id = $1
		WHERE st.slug = 'university'
		ORDER BY s.id
	`, websiteFieldID)
	if err != nil {
		slog.Error("failed to query university spaces", "error", err)
		os.Exit(1)
	}
	defer rows.Close()

	var entries []backfillEntry

	for rows.Next() {
		var e backfillEntry

		var website *string
		if err := rows.Scan(&e.SpaceID, &e.TenantID, &e.Name, &website); err != nil {
			slog.Warn("skipping row: scan error", "error", err)
			continue
		}

		if website != nil {
			e.Website = *website
		}

		entries = append(entries, e)
	}

	if err := rows.Err(); err != nil {
		slog.Error("row iteration error", "error", err)
		os.Exit(1)
	}

	slog.Info("found university spaces", "count", len(entries))

	updated := 0

	for i := 0; i < len(entries); i += batchSize {
		end := i + batchSize
		if end > len(entries) {
			end = len(entries)
		}

		batch := entries[i:end]

		var sb strings.Builder

		sb.WriteString(`
			INSERT INTO custom_field_values (field_id, entity_type, entity_id, value_text)
			SELECT $1, 'space', s.id, v.initials
			FROM spaces s
			JOIN (VALUES
		`)

		args := make([]interface{}, 0, len(batch)*3+1)
		args = append(args, initialsFieldID)

		for j, e := range batch {
			if j > 0 {
				sb.WriteString(", ")
			}

			base := j*3 + 2
			fmt.Fprintf(&sb, "($%d::uuid, $%d::uuid, $%d::text)", base, base+1, base+2)

			initials := universities.DeriveInitials(e.Name, nil, []string{e.Website})
			args = append(args, e.SpaceID, e.TenantID, initials)
		}

		sb.WriteString(`) AS v(space_id, tenant_id, initials)
			ON s.id = v.space_id AND s.tenant_id = v.tenant_id
		`)
		sb.WriteString(`
			ON CONFLICT (field_id, entity_type, entity_id) DO UPDATE
			SET value_text = EXCLUDED.value_text, updated_at = now()
		`)

		ct, err := conn.Exec(ctx, sb.String(), args...)
		if err != nil {
			slog.Error("batch upsert failed", "batch_start", i, "error", err)
			os.Exit(1)
		}

		updated += int(ct.RowsAffected())
	}

	slog.Info("backfill complete", "updated", updated)
	fmt.Printf("\nBackfill complete: %d university initials updated\n", updated)
}

// resolveDatabaseURL reads DATABASE_URL from the environment, falling back to
// reading JC_DATABASE_URL from a .env file or DB_PASSWORD for local dev.
func resolveDatabaseURL() string {
	if url := os.Getenv("DATABASE_URL"); url != "" {
		slog.Info("using DATABASE_URL from environment")
		return url
	}

	if url := os.Getenv("JC_DATABASE_URL"); url != "" {
		slog.Info("using JC_DATABASE_URL from environment")
		return url
	}

	if url := readEnvFile("JC_DATABASE_URL"); url != "" {
		slog.Info("using JC_DATABASE_URL from .env")
		return url
	}

	slog.Info("no database URL found, reading DB_PASSWORD from .env")

	password := readEnvFile("DB_PASSWORD")
	if password == "" {
		slog.Error("no DATABASE_URL, JC_DATABASE_URL, or DB_PASSWORD found")
		os.Exit(1)
	}

	url := fmt.Sprintf("postgres://japa:%s@localhost:5432/japa?sslmode=disable", password)

	slog.Info("constructed DATABASE_URL from .env", "host", "localhost", "db", "japa")

	return url
}

func readEnvFile(key string) string {
	data, err := os.ReadFile(".env")
	if err != nil {
		return ""
	}

	prefix := key + "="

	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, prefix) {
			return strings.TrimPrefix(line, prefix)
		}
	}

	return ""
}
