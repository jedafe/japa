// Command seed creates the default admin user, profile, group memberships,
// spaces, and preferences for a fresh JapaConnect database.
//
// It is idempotent: if the admin user already exists, it prints a message
// and exits cleanly. Run it after migrations:
//
//	source .env && go run ./cmd/seed/
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/auth"
	"github.com/jedafe/japa/internal/config"
	"github.com/jedafe/japa/internal/db"
)

// Seeded UUIDs — must match the values in the migration seed data.
const (
	tenantID          = "00000000-0000-0000-0000-000000000001"
	superAdminGroupID = "00000000-0000-0000-0000-000000000010"
	membersGroupID    = "00000000-0000-0000-0000-000000000020"
	countrySpaceType  = "00000000-0000-0000-0000-000000000060"
)

// Default admin credentials.
const (
	adminEmail       = "admin@japaconnect.com"
	adminPassword    = "Admin123!"
	adminUsername    = "admin"
	adminDisplayName = "Platform Admin"
)

// defaultSpace describes a space to create during seeding.
type defaultSpace struct {
	Name        string
	Slug        string
	Description string
}

var defaultSpaces = []defaultSpace{
	{Name: "Nigeria", Slug: "nigeria", Description: "Community for Nigerians at home and abroad"},
	{Name: "United Kingdom", Slug: "united-kingdom", Description: "Nigerians in the UK"},
	{Name: "United States", Slug: "united-states", Description: "Nigerians in the US"},
	{Name: "Canada", Slug: "canada", Description: "Nigerians in Canada"},
}

func main() {
	if err := run(); err != nil {
		slog.Error("seed failed", "error", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	ctx := context.Background()

	database, err := db.New(ctx, cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("connecting to database: %w", err)
	}

	defer database.Close()

	// Parse the tenant ID once.
	tid := parseUUID(tenantID)

	// ── Check if admin already exists (idempotent) ─────────────────────
	var existingID pgtype.UUID

	err = database.Reader.QueryRow(
		ctx,
		`SELECT id FROM users WHERE tenant_id = $1 AND email = $2`, tid, adminEmail,
	).Scan(&existingID)
	if err == nil {
		fmt.Println("seed: admin user already exists, skipping")
		return nil
	}

	// ── Create admin user ──────────────────────────────────────────────
	hash, err := auth.HashPassword(adminPassword)
	if err != nil {
		return fmt.Errorf("hashing password: %w", err)
	}

	var userID pgtype.UUID

	err = database.Writer.QueryRow(
		ctx,
		`INSERT INTO users (tenant_id, email, username, password_hash, profile_type, status)
		 VALUES ($1, $2, $3, $4, 'personal', 'active')
		 RETURNING id`, tid, adminEmail, adminUsername, hash,
	).Scan(&userID)
	if err != nil {
		return fmt.Errorf("creating admin user: %w", err)
	}

	fmt.Printf("seed: created admin user %s (%s)\n", adminEmail, userID.String())

	// ── Create user profile ────────────────────────────────────────────
	_, err = database.Writer.Exec(
		ctx,
		`INSERT INTO user_profiles (tenant_id, user_id, display_name)
		 VALUES ($1, $2, $3)`, tid, userID, adminDisplayName,
	)
	if err != nil {
		return fmt.Errorf("creating admin profile: %w", err)
	}

	fmt.Println("seed: created admin profile")

	// ── Add admin to system groups ─────────────────────────────────────
	for _, groupPair := range []struct {
		name string
		id   string
	}{
		{name: "super_admin", id: superAdminGroupID},
		{name: "members", id: membersGroupID},
	} {
		gid := parseUUID(groupPair.id)

		_, err = database.Writer.Exec(
			ctx,
			`INSERT INTO user_group_memberships (tenant_id, user_id, group_id)
			 VALUES ($1, $2, $3)
			 ON CONFLICT (tenant_id, user_id, group_id) DO NOTHING`, tid, userID, gid,
		)
		if err != nil {
			return fmt.Errorf("adding admin to %s group: %w", groupPair.name, err)
		}

		fmt.Printf("seed: added admin to %s group\n", groupPair.name)
	}

	// ── Create default spaces ──────────────────────────────────────────
	stID := parseUUID(countrySpaceType)

	for _, s := range defaultSpaces {
		var spaceID pgtype.UUID

		err = database.Writer.QueryRow(
			ctx,
			`INSERT INTO spaces (
				tenant_id, space_type_id, name, slug, description, level,
				visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status
			) VALUES ($1, $2, $3, $4, $5, 'country',
				'public', 'members', 'members', 'members', 'open', 'active')
			ON CONFLICT (tenant_id, slug) DO UPDATE SET slug = EXCLUDED.slug
			RETURNING id`, tid, stID, s.Name, s.Slug, s.Description,
		).Scan(&spaceID)
		if err != nil {
			return fmt.Errorf("creating space %s: %w", s.Slug, err)
		}

		// Add admin as member of the space.
		_, err = database.Writer.Exec(
			ctx,
			`INSERT INTO space_members (space_id, user_id, tenant_id, role)
			 VALUES ($1, $2, $3, 'admin')
			 ON CONFLICT (space_id, user_id) DO NOTHING`, spaceID, userID, tid,
		)
		if err != nil {
			return fmt.Errorf("adding admin to space %s: %w", s.Slug, err)
		}

		fmt.Printf("seed: created space %q (%s)\n", s.Name, spaceID.String())
	}

	// ── Create user preferences ────────────────────────────────────────
	_, err = database.Writer.Exec(
		ctx,
		`INSERT INTO user_preferences (tenant_id, user_id, dark_mode, language)
		 VALUES ($1, $2, 'system', 'en-NG')
		 ON CONFLICT (tenant_id, user_id) DO NOTHING`, tid, userID,
	)
	if err != nil {
		return fmt.Errorf("creating user preferences: %w", err)
	}

	fmt.Println("seed: created user preferences")

	fmt.Println("\nseed: complete ✓")

	return nil
}

// parseUUID wraps a hardcoded UUID string into pgtype.UUID.
// Panics on bad input — these are compile-time constants that must be valid.
func parseUUID(s string) pgtype.UUID {
	var u pgtype.UUID
	if err := u.Scan(s); err != nil {
		panic("seed: invalid UUID constant " + s + ": " + err.Error())
	}

	return u
}
