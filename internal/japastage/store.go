// Package japastage implements the Japa Stage Progress Profiles (§87,
// ADR-095).
// // Each user_profile row carries a japa_stage column tracking where the
// member is in their Japan relocation journey (considering → settled), an
// optional destination space, a visibility toggle, and a jsonb history of
// stage transitions. The package provides profile reads, stage updates,
// visibility toggles, and a public directory filtered by stage.
// // Ref: japaconnect-engineering/sections/87-japa-stage-progress-profiles.md
// //	japaconnect-engineering/adr/ADR-095-japa-stage-progress-profiles.md
package japastage

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/db"
)

// Store is the data-access layer for the japa_stage columns on user_profiles.
// It uses direct pgx queries because the japa_stage fields are new columns
// without sqlc-generated queries yet. No business logic here.
type Store struct {
	pool *db.DB
}

// NewStore creates a japa stage store backed by the given database.
func NewStore(database *db.DB) *Store {
	return &Store{pool: database}
}

// ProfileRow is the joined result of user_profiles + users + spaces,
// returned by get-by-user and get-by-username queries.
type ProfileRow struct {
	UserID            pgtype.UUID
	TenantID          pgtype.UUID
	Username          *string
	DisplayName       *string
	AvatarURL         *string
	JapaStage         *string
	JapaStageDestID   pgtype.UUID
	JapaStageDestName *string
	JapaStageDestCode *string
	JapaStagePublic   bool
	JapaStageHistory  []byte // raw jsonb
	JapaStageUpdated  pgtype.Timestamptz
}

// DirectoryRow is the trimmed shape returned by ListByStage (public directory).
type DirectoryRow struct {
	UserID            pgtype.UUID
	Username          *string
	DisplayName       *string
	AvatarURL         *string
	JapaStage         *string
	JapaStageDestID   pgtype.UUID
	JapaStageDestName *string
	JapaStageDestCode *string
	JapaStageUpdated  pgtype.Timestamptz
}

// GetUserProfile fetches the user_profiles row (with japa_stage columns)
// for a given user_id, joining users + spaces for display fields.
func (s *Store) GetUserProfile(ctx context.Context, userID, tenantID pgtype.UUID) (ProfileRow, error) {
	const q = `
SELECT up.user_id, u.username, up.display_name, up.avatar_url,
       up.japa_stage, up.japa_stage_dest_space_id,
       ds.name AS dest_space_name, ds.geo_code AS dest_space_code,
       up.japa_stage_is_public, up.japa_stage_history, up.japa_stage_updated_at
FROM user_profiles up
JOIN users u ON u.id = up.user_id
LEFT JOIN spaces ds ON ds.id = up.japa_stage_dest_space_id
WHERE up.user_id = $1
LIMIT 1
`

	var r ProfileRow

	err := s.pool.Reader.QueryRow(ctx, q, userID, tenantID).Scan(
		&r.UserID, &r.Username, &r.DisplayName, &r.AvatarURL,
		&r.JapaStage, &r.JapaStageDestID,
		&r.JapaStageDestName, &r.JapaStageDestCode,
		&r.JapaStagePublic, &r.JapaStageHistory, &r.JapaStageUpdated,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ProfileRow{}, pgx.ErrNoRows
		}

		return ProfileRow{}, fmt.Errorf("japastage: get user profile: %w", err)
	}

	return r, nil
}

// GetProfileByUsername fetches the japa_stage profile by username (for the
// public view). Joins users + spaces.
func (s *Store) GetProfileByUsername(ctx context.Context, username string, tenantID pgtype.UUID) (ProfileRow, error) {
	const q = `
SELECT up.user_id, u.username, up.display_name, up.avatar_url,
       up.japa_stage, up.japa_stage_dest_space_id,
       ds.name AS dest_space_name, ds.geo_code AS dest_space_code,
       up.japa_stage_is_public, up.japa_stage_history, up.japa_stage_updated_at
FROM user_profiles up
JOIN users u ON u.id = up.user_id
LEFT JOIN spaces ds ON ds.id = up.japa_stage_dest_space_id
WHERE u.username = $1
LIMIT 1
`

	var r ProfileRow

	err := s.pool.Reader.QueryRow(ctx, q, username, tenantID).Scan(
		&r.UserID, &r.Username, &r.DisplayName, &r.AvatarURL,
		&r.JapaStage, &r.JapaStageDestID,
		&r.JapaStageDestName, &r.JapaStageDestCode,
		&r.JapaStagePublic, &r.JapaStageHistory, &r.JapaStageUpdated,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ProfileRow{}, pgx.ErrNoRows
		}

		return ProfileRow{}, fmt.Errorf("japastage: get profile by username: %w", err)
	}

	return r, nil
}

// UpdateStage upserts japa_stage, japa_stage_dest_space_id,
// japa_stage_is_public (when isPublic is non-nil), and
// japa_stage_updated_at, and appends a transition record to the
// japa_stage_history jsonb array. destSpaceID may be NULL to clear it.
// // The upsert (INSERT ... ON CONFLICT DO UPDATE) ensures a user without an
// existing user_profiles row — e.g. a newly registered member who has not
// yet visited profile settings — can still set their japa stage. CreateUser
// (auth.sql) only inserts into `users`; no trigger auto-creates the profile
// row, so a plain UPDATE would affect 0 rows and 404.
// // The history record uses the spec key `set_at` (§87:
// `japa_stage_history: [{stage, dest_space_id, set_at}]`); the destination
// is stored as a `spaces` row (country-level space per ADR-053) because no
// `geo_countries` table exists.
func (s *Store) UpdateStage(ctx context.Context, userID, tenantID pgtype.UUID, stage string, destSpaceID pgtype.UUID, isPublic *bool) error {
	const q = `
INSERT INTO user_profiles (user_id, japa_stage, japa_stage_dest_space_id, japa_stage_updated_at, japa_stage_is_public, japa_stage_history)
VALUES (
    $2, $1, $3, $4, now(),
    COALESCE($5::bool, true),
    jsonb_build_array(
        jsonb_build_object(
            'stage', $3::text,
            'dest_space_id', CASE WHEN $4::uuid IS NULL THEN 'null'::jsonb ELSE to_jsonb($4::uuid) END,
            'set_at', to_char(now() AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"')
        )
    )
)
ON CONFLICT (user_id) DO UPDATE SET
    japa_stage = EXCLUDED.japa_stage,
    japa_stage_dest_space_id = EXCLUDED.japa_stage_dest_space_id,
    japa_stage_updated_at = now(),
    japa_stage_is_public = CASE WHEN $5::bool IS NULL THEN user_profiles.japa_stage_is_public ELSE $5::bool END,
    japa_stage_history = user_profiles.japa_stage_history || jsonb_build_array(
        jsonb_build_object(
            'stage', $3::text,
            'dest_space_id', CASE WHEN $4::uuid IS NULL THEN 'null'::jsonb ELSE to_jsonb($4::uuid) END,
            'set_at', to_char(now() AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"')
        )
    )
`

	cmd, err := s.pool.Writer.Exec(ctx, q, userID, tenantID, stage, destSpaceID, isPublic)
	if err != nil {
		return fmt.Errorf("japastage: update stage: %w", err)
	}

	if cmd.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}

	return nil
}

// SetPublic updates the japa_stage_is_public visibility toggle.
func (s *Store) SetPublic(ctx context.Context, userID, tenantID pgtype.UUID, isPublic bool) error {
	const q = `
UPDATE user_profiles
SET japa_stage_is_public = $2
WHERE user_id = $1
`

	cmd, err := s.pool.Writer.Exec(ctx, q, userID, tenantID, isPublic)
	if err != nil {
		return fmt.Errorf("japastage: set public: %w", err)
	}

	if cmd.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}

	return nil
}

// ListByStage returns a paginated directory of public japa stage profiles.
// stage, when non-empty, filters to that stage; when empty, all public
// profiles with a set japa_stage are returned. destSpaceID, when valid,
// further filters by destination (geo-aware ?dest= filter, §87).
func (s *Store) ListByStage(ctx context.Context, stage string, destSpaceID pgtype.UUID, limit, offset int32) ([]DirectoryRow, int64, error) {
	if limit <= 0 {
		limit = 20
	}

	if limit > 100 {
		limit = 100
	}

	if offset < 0 {
		offset = 0
	}

	listPredicates := []string{
		"up.japa_stage_is_public = true",
	}
	countPredicates := []string{
		"japa_stage_is_public = true",
	}
	listArgs := []any{}
	countArgs := []any{}

	if stage != "" {
		listPredicates = append(listPredicates, fmt.Sprintf("up.japa_stage = $%d", len(listArgs)+1))
		countPredicates = append(countPredicates, fmt.Sprintf("japa_stage = $%d", len(countArgs)+1))
		listArgs = append(listArgs, stage)
		countArgs = append(countArgs, stage)
	} else {
		listPredicates = append(listPredicates, "up.japa_stage IS NOT NULL")
		countPredicates = append(countPredicates, "japa_stage IS NOT NULL")
	}

	if destSpaceID.Valid {
		listPredicates = append(listPredicates, fmt.Sprintf("up.japa_stage_dest_space_id = $%d", len(listArgs)+1))
		countPredicates = append(countPredicates, fmt.Sprintf("japa_stage_dest_space_id = $%d", len(countArgs)+1))
		listArgs = append(listArgs, destSpaceID)
		countArgs = append(countArgs, destSpaceID)
	}

	listQuery := fmt.Sprintf(`
SELECT up.user_id, u.username, up.display_name, up.avatar_url,
       up.japa_stage, up.japa_stage_dest_space_id,
       ds.name AS dest_space_name, ds.geo_code AS dest_space_code,
       up.japa_stage_updated_at
FROM user_profiles up
JOIN users u ON u.id = up.user_id
LEFT JOIN spaces ds ON ds.id = up.japa_stage_dest_space_id
WHERE %s
ORDER BY up.japa_stage_updated_at DESC NULLS LAST, up.user_id
LIMIT $%d OFFSET $%d
`, strings.Join(listPredicates, "\n  AND "), len(listArgs)+1, len(listArgs)+2)
	listArgs = append(listArgs, limit, offset)

	rows, err := s.pool.Reader.Query(ctx, listQuery, listArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("japastage: list by stage: %w", err)
	}

	defer rows.Close()

	out, err := scanDirectoryRows(rows)
	if err != nil {
		return nil, 0, err
	}

	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM user_profiles\nWHERE %s", strings.Join(countPredicates, "\n  AND "))

	var total int64
	if err := s.pool.Reader.QueryRow(ctx, countQuery, countArgs...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("japastage: count by stage: %w", err)
	}

	return out, total, nil
}

// scanDirectoryRows scans a DirectoryRow set from an open pgx.Rows cursor.
func scanDirectoryRows(rows pgx.Rows) ([]DirectoryRow, error) {
	var out []DirectoryRow

	for rows.Next() {
		var r DirectoryRow
		if err := rows.Scan(
			&r.UserID, &r.Username, &r.DisplayName, &r.AvatarURL,
			&r.JapaStage, &r.JapaStageDestID,
			&r.JapaStageDestName, &r.JapaStageDestCode,
			&r.JapaStageUpdated,
		); err != nil {
			return nil, fmt.Errorf("japastage: scan directory row: %w", err)
		}

		out = append(out, r)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("japastage: iterate directory rows: %w", err)
	}

	return out, nil
}

// ValidStage reports whether stage is one of the allowed japa_stage values
// enforced by the user_profiles CHECK constraint.
func ValidStage(stage string) bool {
	for _, s := range validStages {
		if s == stage {
			return true
		}
	}

	return false
}

var validStages = []string{
	"considering",
	"planning",
	"preparing",
	"applied",
	"approved",
	"relocated",
	"settled",
}

// JoinStages returns the valid stages as a comma-separated string, useful
// for error messages.
func JoinStages() string {
	return strings.Join(validStages, ", ")
}
