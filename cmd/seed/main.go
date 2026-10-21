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
	"errors"
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
	superAdminGroupID = "00000000-0000-0000-0000-000000000010"
	membersGroupID    = "00000000-0000-0000-0000-000000000020"
	countrySpaceType  = "00000000-0000-0000-0000-000000000060"
)

// Default admin display values (non-sensitive).
const (
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

	// Source admin credentials from environment variables (AUDIT-BE-017).
	adminEmail := os.Getenv("JC_SEED_ADMIN_EMAIL")
	adminPassword := os.Getenv("JC_SEED_ADMIN_PASSWORD")

	if adminEmail == "" {
		adminEmail = "admin@japa.to"
	}

	if adminPassword == "" {
		if cfg.IsProduction() {
			return errors.New("JC_SEED_ADMIN_PASSWORD must be set in production")
		}

		slog.Warn("JC_SEED_ADMIN_PASSWORD not set, using default dev password")

		adminPassword = "Admin123!"
	}

	ctx := context.Background()

	database, err := db.New(ctx, cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("connecting to database: %w", err)
	}

	defer database.Close()

	// ── Check if admin already exists (idempotent) ─────────────────────
	var existingID pgtype.UUID

	err = database.Reader.QueryRow(ctx, `SELECT id FROM users WHERE email = $1`, adminEmail).Scan(&existingID)

	var userID pgtype.UUID

	if err == nil {
		fmt.Println("seed: admin user already exists, skipping admin creation")

		userID = existingID
	} else {
		// ── Create admin user ──────────────────────────────────────────
		hash, err := auth.HashPassword(adminPassword)
		if err != nil {
			return fmt.Errorf("hashing password: %w", err)
		}

		err = database.Writer.QueryRow(ctx, `INSERT INTO users (email, username, password_hash, profile_type, status) VALUES ($1, $2, $3, 'regular', 'active')
			 RETURNING id`, adminEmail, adminUsername, hash).Scan(&userID)
		if err != nil {
			return fmt.Errorf("creating admin user: %w", err)
		}

		fmt.Printf("seed: created admin user %s (%s)\n", adminEmail, userID.String())

		// ── Create user profile ────────────────────────────────────────
		_, err = database.Writer.Exec(ctx, `INSERT INTO user_profiles (user_id, display_name) VALUES ($1, $2)`, userID, adminDisplayName)
		if err != nil {
			return fmt.Errorf("creating admin profile: %w", err)
		}

		fmt.Println("seed: created admin profile")
	}

	// ── Add admin to system groups ─────────────────────────────────────
	for _, groupPair := range []struct {
		name string
		id   string
	}{
		{name: "super_admin", id: superAdminGroupID},
		{name: "members", id: membersGroupID},
	} {
		gid := parseUUID(groupPair.id)

		_, err = database.Writer.Exec(ctx, `INSERT INTO user_group_memberships (user_id, group_id) VALUES ($1, $2)
			 ON CONFLICT (user_id, group_id) DO NOTHING`, userID, gid)
		if err != nil {
			return fmt.Errorf("adding admin to %s group: %w", groupPair.name, err)
		}

		fmt.Printf("seed: added admin to %s group\n", groupPair.name)
	}

	// ── Create default spaces ──────────────────────────────────────────
	stID := parseUUID(countrySpaceType)

	for _, s := range defaultSpaces {
		var spaceID pgtype.UUID

		err = database.Writer.QueryRow(ctx, `INSERT INTO spaces (
				space_type_id, name, slug, description, level,
				visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status
			) VALUES ($1, $2, $3, $4, 'country',
				'public', 'members', 'members', 'members', 'open', 'active')
			ON CONFLICT (slug) WHERE deleted_at IS NULL DO UPDATE SET slug = EXCLUDED.slug
			RETURNING id`, stID, s.Name, s.Slug, s.Description).Scan(&spaceID)
		if err != nil {
			return fmt.Errorf("creating space %s: %w", s.Slug, err)
		}

		// Add admin as member of the space.
		_, err = database.Writer.Exec(ctx, `INSERT INTO space_members (space_id, user_id, role) VALUES ($1, $2, 'admin')
			 ON CONFLICT (space_id, user_id) DO NOTHING`, spaceID, userID)
		if err != nil {
			return fmt.Errorf("adding admin to space %s: %w", s.Slug, err)
		}

		fmt.Printf("seed: created space %q (%s)\n", s.Name, spaceID.String())
	}

	// ── Create user preferences ────────────────────────────────────────
	_, err = database.Writer.Exec(ctx, `INSERT INTO user_preferences (user_id, dark_mode, language) VALUES ($1, 'system', 'en-NG')
		 ON CONFLICT (user_id) DO NOTHING`, userID)
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
