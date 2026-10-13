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
	UserID pgtype.UUID

	Username          *string
	DisplayName       *string
	AvatarURL         *string
	JapaStage         *string
	JapaIntent        *string
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
	JapaIntent        *string
	JapaStageDestID   pgtype.UUID
	JapaStageDestName *string
	JapaStageDestCode *string
	JapaStageUpdated  pgtype.Timestamptz
}

// GetUserProfile fetches the user_profiles row (with japa_stage columns)
// for a given user_id, joining users + spaces for display fields.
func (s *Store) GetUserProfile(ctx context.Context, userID pgtype.UUID) (ProfileRow, error) {
	const q = `
SELECT up.user_id, u.username, up.display_name, up.avatar_url,
       up.japa_stage, up.japa_intent, up.japa_stage_dest_space_id,
       ds.name AS dest_space_name, ds.geo_code AS dest_space_code,
       up.japa_stage_is_public, up.japa_stage_history, up.japa_stage_updated_at
FROM user_profiles up
JOIN users u ON u.id = up.user_id
LEFT JOIN spaces ds ON ds.id = up.japa_stage_dest_space_id
WHERE up.user_id = $1
LIMIT 1
`

	var r ProfileRow

	err := s.pool.Reader.QueryRow(ctx, q, userID).Scan(
		&r.UserID, &r.Username, &r.DisplayName, &r.AvatarURL,
		&r.JapaStage, &r.JapaIntent, &r.JapaStageDestID,
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
func (s *Store) GetProfileByUsername(ctx context.Context, username string) (ProfileRow, error) {
	const q = `
SELECT up.user_id, u.username, up.display_name, up.avatar_url,
       up.japa_stage, up.japa_intent, up.japa_stage_dest_space_id,
       ds.name AS dest_space_name, ds.geo_code AS dest_space_code,
       up.japa_stage_is_public, up.japa_stage_history, up.japa_stage_updated_at
FROM user_profiles up
JOIN users u ON u.id = up.user_id
LEFT JOIN spaces ds ON ds.id = up.japa_stage_dest_space_id
WHERE u.username = $1
LIMIT 1
`

	var r ProfileRow

	err := s.pool.Reader.QueryRow(ctx, q, username).Scan(
		&r.UserID, &r.Username, &r.DisplayName, &r.AvatarURL,
		&r.JapaStage, &r.JapaIntent, &r.JapaStageDestID,
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

// UpdateStage upserts japa_stage, japa_intent, japa_stage_dest_space_id,
// japa_stage_is_public (when isPublic is non-nil), and
// japa_stage_updated_at, and appends a transition record to the
// japa_stage_history jsonb array. destSpaceID may be NULL to clear it.
// // stage is a pointer: nil leaves japa_stage unchanged, a non-nil empty
// string clears it to NULL (used when switching to a non-relocation intent
// per ADR-122), and a non-empty value sets it. intent follows the same
// contract: nil leaves japa_intent unchanged, a non-empty value sets it.
// // The upsert (INSERT ... ON CONFLICT DO UPDATE) ensures a user without an
// existing user_profiles row — e.g. a newly registered member who has not
// yet visited profile settings — can still set their japa stage. CreateUser
// (auth.sql) only inserts into `users`; no trigger auto-creates the profile
// row, so a plain UPDATE would affect 0 rows and 404.
// // The history record uses the spec keys (§87:
// `japa_stage_history: [{intent, stage, dest_space_id, set_at}]`); each entry
// records the effective intent+stage pair after the update so the log stays
// self-describing (ADR-122). The destination is stored as a `spaces` row
// (country-level space per ADR-053) because no `geo_countries` table exists.
func (s *Store) UpdateStage(ctx context.Context, userID pgtype.UUID, stage *string, intent *string, destSpaceID pgtype.UUID, isPublic *bool) error {
	const q = `
INSERT INTO user_profiles (user_id, japa_stage, japa_intent, japa_stage_dest_space_id, japa_stage_updated_at, japa_stage_is_public, japa_stage_history)
VALUES (
    $1, NULLIF($2::text, ''), $3::text, $4, now(),
    COALESCE($5::bool, true),
    jsonb_build_array(
        jsonb_build_object(
            'intent', to_jsonb($3::text),
            'stage', to_jsonb(NULLIF($2::text, '')),
            'dest_space_id', to_jsonb($4::uuid),
            'set_at', to_char(now() AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"')
        )
    )
)
ON CONFLICT (user_id) DO UPDATE SET
    japa_stage = CASE WHEN $2::text IS NULL THEN user_profiles.japa_stage ELSE NULLIF($2::text, '') END,
    japa_intent = CASE WHEN $3::text IS NULL THEN user_profiles.japa_intent ELSE $3::text END,
    japa_stage_dest_space_id = EXCLUDED.japa_stage_dest_space_id,
    japa_stage_updated_at = now(),
    japa_stage_is_public = CASE WHEN $5::bool IS NULL THEN user_profiles.japa_stage_is_public ELSE $5::bool END,
    japa_stage_history = user_profiles.japa_stage_history || jsonb_build_array(
        jsonb_build_object(
            'intent', to_jsonb(COALESCE($3::text, user_profiles.japa_intent)),
            'stage', to_jsonb(CASE WHEN $2::text IS NULL THEN user_profiles.japa_stage ELSE NULLIF($2::text, '') END),
            'dest_space_id', to_jsonb($4::uuid),
            'set_at', to_char(now() AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"')
        )
    )
`

	cmd, err := s.pool.Writer.Exec(ctx, q, userID, stage, intent, destSpaceID, isPublic)
	if err != nil {
		return fmt.Errorf("japastage: update stage: %w", err)
	}

	if cmd.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}

	return nil
}

// UpsertJourneyClass sets japa_intent and/or japa_stage without touching
// the destination or visibility fields. Unlike UpdateStage, both values are
// "set only" — an empty value leaves the column unchanged — except when
// clearStage is true, which clears japa_stage to NULL (the ADR-122
// class-switch rule: the stored stage is invalid for the new intent).
// Used by surfaces that learn the journey class indirectly (onboarding
// primary_goal / japa_status, the purpose custom field) so they can merge
// into the canonical columns without an intent×stage error path (ADR-128).
// A history entry tagged `source` records the effective pair after the
// write so the log stays self-describing.
func (s *Store) UpsertJourneyClass(ctx context.Context, userID pgtype.UUID, intent, stage, source string, clearStage bool) error {
	const q = `
INSERT INTO user_profiles (user_id, japa_intent, japa_stage, japa_stage_updated_at, japa_stage_history)
VALUES (
    $1, NULLIF($2::text, ''), NULLIF($3::text, ''), now(),
    jsonb_build_array(
        jsonb_build_object(
            'intent', to_jsonb(NULLIF($2::text, '')),
            'stage', to_jsonb(NULLIF($3::text, '')),
            'source', to_jsonb($4::text),
            'set_at', to_char(now() AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"')
        )
    )
)
ON CONFLICT (user_id) DO UPDATE SET
    japa_intent = COALESCE(NULLIF($2::text, ''), user_profiles.japa_intent),
    japa_stage = CASE WHEN $5::bool THEN NULL ELSE COALESCE(NULLIF($3::text, ''), user_profiles.japa_stage) END,
    japa_stage_updated_at = now(),
    japa_stage_history = user_profiles.japa_stage_history || jsonb_build_array(
        jsonb_build_object(
            'intent', to_jsonb(COALESCE(NULLIF($2::text, ''), user_profiles.japa_intent)),
            'stage', to_jsonb(CASE WHEN $5::bool THEN NULL ELSE COALESCE(NULLIF($3::text, ''), user_profiles.japa_stage) END),
            'source', to_jsonb($4::text),
            'set_at', to_char(now() AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"')
        )
    )
`

	cmd, err := s.pool.Writer.Exec(ctx, q, userID, intent, stage, source, clearStage)
	if err != nil {
		return fmt.Errorf("japastage: upsert journey class: %w", err)
	}

	if cmd.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}

	return nil
}

// SetPublic updates the japa_stage_is_public visibility toggle.
func (s *Store) SetPublic(ctx context.Context, userID pgtype.UUID, isPublic bool) error {
	const q = `
UPDATE user_profiles
SET japa_stage_is_public = $2
WHERE user_id = $1
`

	cmd, err := s.pool.Writer.Exec(ctx, q, userID, isPublic)
	if err != nil {
		return fmt.Errorf("japastage: set public: %w", err)
	}

	if cmd.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}

	return nil
}

// ListByStage returns a paginated directory of public japa stage profiles.
// stage, when non-empty, filters to that stage. intent, when non-empty,
// filters to that journey class (ADR-122) — including stage-less members
// for non-relocation intents. When both are empty, all public profiles with
// a set japa_stage are returned. destSpaceID, when valid, further filters
// by destination (geo-aware ?dest= filter, §87).
func (s *Store) ListByStage(ctx context.Context, stage, intent string, destSpaceID pgtype.UUID, limit, offset int32) ([]DirectoryRow, int64, error) {
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
	}

	if intent != "" {
		listPredicates = append(listPredicates, fmt.Sprintf("up.japa_intent = $%d", len(listArgs)+1))
		countPredicates = append(countPredicates, fmt.Sprintf("japa_intent = $%d", len(countArgs)+1))
		listArgs = append(listArgs, intent)
		countArgs = append(countArgs, intent)
	} else if stage == "" {
		// Unfiltered directory keeps the ADR-095 shape: members who set a
		// stage. An intent filter bypasses this so stage-less visit/business
		// members are listable (ADR-122).
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
       up.japa_stage, up.japa_intent, up.japa_stage_dest_space_id,
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
			&r.JapaStage, &r.JapaIntent, &r.JapaStageDestID,
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

// validStages is the canonical nine-stage vocabulary (ADR-122 amendment):
// a shared five-stage visa arc plus two class-scoped terminal pairs.
// Ordering is canonical progression order — the filtered per-class subsets
// preserve it.
var validStages = []string{
	"considering",
	"planning",
	"preparing",
	"applied",
	"approved",
	"relocated",
	"settled",
	"traveled",
	"returned",
}

// relocationStages and tripStages are the class-scoped stage sets enforced
// by user_profiles_intent_stage_chk (ADR-122). The shared arc is identical;
// only the terminal pair differs per journey class.
var relocationStages = []string{
	"considering", "planning", "preparing", "applied", "approved",
	"relocated", "settled",
}

var tripStages = []string{
	"considering", "planning", "preparing", "applied", "approved",
	"traveled", "returned",
}

// StagesForIntent returns the stage values applicable to a journey class,
// in progression order. An empty or unrecognized intent resolves to the
// relocation class — NULL intent is the legacy undeclared state and every
// pre-ADR-122 stage was relocation-class.
func StagesForIntent(intent string) []string {
	if intent == "visit" || intent == "business" {
		return tripStages
	}

	return relocationStages
}

// ValidStageForIntent reports whether stage is within the intent's
// applicable set — the service-layer mirror of user_profiles_intent_stage_chk.
func ValidStageForIntent(stage, intent string) bool {
	for _, s := range StagesForIntent(intent) {
		if s == stage {
			return true
		}
	}

	return false
}

// JoinStages returns the valid stages as a comma-separated string, useful
// for error messages.
func JoinStages() string {
	return strings.Join(validStages, ", ")
}

// ValidIntent reports whether intent is one of the allowed japa_intent
// values enforced by the user_profiles CHECK constraint (ADR-122).
func ValidIntent(intent string) bool {
	for _, v := range validIntents {
		if v == intent {
			return true
		}
	}

	return false
}

var validIntents = []string{
	"relocation",
	"study",
	"work",
	"family",
	"visit",
	"business",
}

// IsRelocationIntent reports whether the intent belongs to the relocation
// journey class — the class whose terminal stages are relocated/settled
// (ADR-122 composite CHECK user_profiles_intent_stage_chk).
func IsRelocationIntent(intent string) bool {
	switch intent {
	case "relocation", "study", "work", "family":
		return true
	}

	return false
}

// JoinIntents returns the valid intents as a comma-separated string, useful
// for error messages.
func JoinIntents() string {
	return strings.Join(validIntents, ", ")
}
