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
	Verified          bool
}

// DirectoryRow is the trimmed shape returned by ListByStage (public directory).
// The avatar media bucket/path fields come from LEFT JOINs on
// user_profiles.avatar_media_id — when present the service re-signs a
// display-sized derivative instead of emitting the stored 640px URL.
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
	Bio               *string
	Profession        *string
	Interests         []string
	Verified          bool
	CoverURL          *string
	LastActiveAt      pgtype.Timestamptz
	AvatarMediaBucket *string
	AvatarMediaPath   *string
}

// GetUserProfile fetches the user_profiles row (with japa_stage columns)
// for a given user_id, joining users + spaces for display fields.
func (s *Store) GetUserProfile(ctx context.Context, userID pgtype.UUID) (ProfileRow, error) {
	const q = `
SELECT up.user_id, u.username, up.display_name, up.avatar_url,
       up.japa_stage, up.japa_intent, up.japa_stage_dest_space_id,
       ds.name AS dest_space_name, ds.geo_code AS dest_space_code,
       up.japa_stage_is_public, up.japa_stage_history, up.japa_stage_updated_at,
       u.verified
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
		&r.Verified,
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
       up.japa_stage_is_public, up.japa_stage_history, up.japa_stage_updated_at,
       u.verified
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
		&r.Verified,
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
func (s *Store) UpdateStage(ctx context.Context, userID pgtype.UUID, stage, intent *string, destSpaceID pgtype.UUID, isPublic *bool) error {
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

// stageValidCase is the SQL expression for "<stage expr> is valid for
// the effective intent". %s is substituted with a column/expr name; the
// effective intent is the second %s (a COALESCE expr or NULLIF param).
const stageValidCase = `(%[2]s IS NULL OR %[2]s IN ('relocation','study','work','family'))
           AND %[1]s IN ('considering','planning','preparing','applied','approved','relocated','settled')
        OR %[2]s IN ('visit','business')
           AND %[1]s IN ('considering','planning','preparing','applied','approved','traveled','returned')`

// UpsertJourneyClass sets japa_intent and/or japa_stage without touching
// the destination or visibility fields — and without a read: the ADR-122
// intent×stage rule is resolved atomically inside the upsert so
// concurrent writers can't produce a rejected row or a stale cleared
// stage (ADR-128). The contract:
//
//   - a submitted stage wins when it is valid for the effective intent
//     (submitted intent, else stored, else relocation class);
//   - a stored stage is kept only while it remains valid for the
//     effective intent — an incompatible stage on class switch clears;
//   - otherwise japa_stage becomes NULL.
//
// The history entry records the effective post-write pair so the log
// stays self-describing.
func (s *Store) UpsertJourneyClass(ctx context.Context, userID pgtype.UUID, intent, stage, source string) error {
	q := fmt.Sprintf(`
INSERT INTO user_profiles (user_id, japa_intent, japa_stage, japa_stage_updated_at, japa_stage_history)
VALUES (
    $1, NULLIF($2::text, ''),
    CASE WHEN %[1]s THEN NULLIF($3::text, '') ELSE NULL END,
    now(),
    jsonb_build_array(
        jsonb_build_object(
            'intent', to_jsonb(NULLIF($2::text, '')),
            'stage', to_jsonb(CASE WHEN %[1]s THEN NULLIF($3::text, '') ELSE NULL END),
            'source', to_jsonb($4::text),
            'set_at', to_char(now() AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"')
        )
    )
)
ON CONFLICT (user_id) DO UPDATE SET
    japa_intent = COALESCE(NULLIF($2::text, ''), user_profiles.japa_intent),
    japa_stage = CASE
        WHEN %[2]s THEN NULLIF($3::text, '')
        WHEN %[3]s THEN user_profiles.japa_stage
        ELSE NULL
    END,
    japa_stage_updated_at = now(),
    japa_stage_history = user_profiles.japa_stage_history || jsonb_build_array(
        jsonb_build_object(
            'intent', to_jsonb(COALESCE(NULLIF($2::text, ''), user_profiles.japa_intent)),
            'stage', to_jsonb(CASE
                WHEN %[2]s THEN NULLIF($3::text, '')
                WHEN %[3]s THEN user_profiles.japa_stage
                ELSE NULL
            END),
            'source', to_jsonb($4::text),
            'set_at', to_char(now() AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"')
        )
    )
`,
		// %[1]s — submitted stage valid for the submitted intent (INSERT).
		fmt.Sprintf(stageValidCase, "NULLIF($3::text, '')", "NULLIF($2::text, '')"),
		// %[2]s — submitted stage valid for the effective intent (UPDATE).
		fmt.Sprintf(stageValidCase, "NULLIF($3::text, '')", "COALESCE(NULLIF($2::text, ''), user_profiles.japa_intent)"),
		// %[3]s — stored stage valid for the effective intent (UPDATE).
		fmt.Sprintf(stageValidCase, "user_profiles.japa_stage", "COALESCE(NULLIF($2::text, ''), user_profiles.japa_intent)"),
	)

	cmd, err := s.pool.Writer.Exec(ctx, q, userID, intent, stage, source)
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

// DirectorySort orders the public member directory (§87 discovery hub).
// "recent" is the default — most recently active members first.
type DirectorySort string

const (
	DirectorySortRecent       DirectorySort = "recent"
	DirectorySortStageUpdated DirectorySort = "stage_updated"
	DirectorySortName         DirectorySort = "name"
)

// ValidDirectorySort reports whether s is a recognized sort value.
func ValidDirectorySort(s string) bool {
	switch DirectorySort(s) {
	case DirectorySortRecent, DirectorySortStageUpdated, DirectorySortName:
		return true
	}
	return false
}

// validDirectoryGenders is the directory-filterable subset of the
// user_profiles gender vocabulary (the values the profile settings UI
// writes). The column is free text, so the filter list stays whitelisted
// rather than reflecting arbitrary stored values.
var validDirectoryGenders = []string{"male", "female", "non_binary", "other"}

// ValidDirectoryGender reports whether s is a recognized gender filter.
func ValidDirectoryGender(s string) bool {
	for _, v := range validDirectoryGenders {
		if v == s {
			return true
		}
	}
	return false
}

// JoinDirectoryGenders returns the valid gender filters as a
// comma-separated string, useful for error messages.
func JoinDirectoryGenders() string {
	return strings.Join(validDirectoryGenders, ", ")
}

// directoryOrderBy maps a sort value to its ORDER BY clause. The user_id
// tiebreaker keeps pagination stable across all orderings.
func directoryOrderBy(sort string) string {
	switch DirectorySort(sort) {
	case DirectorySortName:
		return "up.display_name ASC NULLS LAST, u.username, up.user_id"
	case DirectorySortStageUpdated:
		return "up.japa_stage_updated_at DESC NULLS LAST, up.user_id"
	default:
		// §19/ADR-014: members who opted out of online status never expose
		// activity — they sort last rather than leaking it via position.
		return "CASE WHEN COALESCE(ups.show_online_status, true) THEN u.last_login_at END DESC NULLS LAST, up.user_id"
	}
}

// likeEscape escapes %, _ and \ so a user-supplied search term cannot inject
// LIKE wildcards (paired with ESCAPE '\' in the query).
func likeEscape(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return r.Replace(s)
}

// ListByStage returns a paginated directory of public japa stage profiles.
// stage, when non-empty, filters to that stage. intent, when non-empty,
// filters to that journey class (ADR-122) — including stage-less members
// for non-relocation intents. When both are empty, all public profiles with
// a set japa_stage are returned. destSpaceID, when valid, further filters
// by destination (geo-aware ?dest= filter, §87). q is a substring match over
// username, display_name, profession and interests (§87 discovery hub).
// sort selects the ORDER BY (recent|stage_updated|name; empty = recent).
func (s *Store) ListByStage(ctx context.Context, stage, intent, q, sort string, destSpaceID pgtype.UUID, gender string, limit, offset int32) ([]DirectoryRow, int64, error) {
	if limit <= 0 {
		limit = 20
	}

	if limit > 100 {
		limit = 100
	}

	if offset < 0 {
		offset = 0
	}

	// §19/§50: the directory is anonymous — members/followers profiles are
	// excluded regardless of japa_stage_is_public (that flag governs the
	// stage payload; profile_visibility governs profile presence).
	listPredicates := []string{
		"up.japa_stage_is_public = true",
		"NOT EXISTS (SELECT 1 FROM user_privacy_settings pv WHERE pv.user_id = u.id AND pv.profile_visibility <> 'public')",
	}
	countPredicates := []string{
		"up.japa_stage_is_public = true",
		"NOT EXISTS (SELECT 1 FROM user_privacy_settings pv WHERE pv.user_id = u.id AND pv.profile_visibility <> 'public')",
	}
	listArgs := []any{}
	countArgs := []any{}

	if stage != "" {
		listPredicates = append(listPredicates, fmt.Sprintf("up.japa_stage = $%d", len(listArgs)+1))
		countPredicates = append(countPredicates, fmt.Sprintf("up.japa_stage = $%d", len(countArgs)+1))
		listArgs = append(listArgs, stage)
		countArgs = append(countArgs, stage)
	}

	if intent != "" {
		listPredicates = append(listPredicates, fmt.Sprintf("up.japa_intent = $%d", len(listArgs)+1))
		countPredicates = append(countPredicates, fmt.Sprintf("up.japa_intent = $%d", len(countArgs)+1))
		listArgs = append(listArgs, intent)
		countArgs = append(countArgs, intent)
	} else if stage == "" {
		// Unfiltered directory keeps the ADR-095 shape: members who set a
		// stage. An intent filter bypasses this so stage-less visit/business
		// members are listable (ADR-122).
		listPredicates = append(listPredicates, "up.japa_stage IS NOT NULL")
		countPredicates = append(countPredicates, "up.japa_stage IS NOT NULL")
	}

	if destSpaceID.Valid {
		listPredicates = append(listPredicates, fmt.Sprintf("up.japa_stage_dest_space_id = $%d", len(listArgs)+1))
		countPredicates = append(countPredicates, fmt.Sprintf("up.japa_stage_dest_space_id = $%d", len(countArgs)+1))
		listArgs = append(listArgs, destSpaceID)
		countArgs = append(countArgs, destSpaceID)
	}

	if gender != "" {
		// Free-text column — case-insensitive exact match so legacy casing
		// still filters.
		listPredicates = append(listPredicates, fmt.Sprintf("LOWER(up.gender) = $%d", len(listArgs)+1))
		countPredicates = append(countPredicates, fmt.Sprintf("LOWER(up.gender) = $%d", len(countArgs)+1))
		listArgs = append(listArgs, gender)
		countArgs = append(countArgs, gender)
	}

	if term := strings.TrimSpace(q); term != "" {
		needle := "%" + likeEscape(term) + "%"
		listPredicates = append(listPredicates, fmt.Sprintf(
			`(u.username ILIKE $%d ESCAPE '\' OR up.display_name ILIKE $%d ESCAPE '\' OR up.profession ILIKE $%d ESCAPE '\' OR EXISTS (SELECT 1 FROM unnest(up.interests) i WHERE i ILIKE $%d ESCAPE '\'))`,
			len(listArgs)+1, len(listArgs)+1, len(listArgs)+1, len(listArgs)+1))
		countPredicates = append(countPredicates, fmt.Sprintf(
			`(u.username ILIKE $%d ESCAPE '\' OR up.display_name ILIKE $%d ESCAPE '\' OR up.profession ILIKE $%d ESCAPE '\' OR EXISTS (SELECT 1 FROM unnest(up.interests) i WHERE i ILIKE $%d ESCAPE '\'))`,
			len(countArgs)+1, len(countArgs)+1, len(countArgs)+1, len(countArgs)+1))
		listArgs = append(listArgs, needle)
		countArgs = append(countArgs, needle)
	}

	listQuery := fmt.Sprintf(`
SELECT up.user_id, u.username, up.display_name, up.avatar_url,
       up.japa_stage, up.japa_intent, up.japa_stage_dest_space_id,
       ds.name AS dest_space_name, ds.geo_code AS dest_space_code,
       up.japa_stage_updated_at,
       up.bio, up.profession, up.interests, u.verified, up.cover_url,
       CASE WHEN COALESCE(ups.show_online_status, true) THEN u.last_login_at END AS last_active_at,
       am.storage_bucket AS avatar_media_bucket, am.storage_path AS avatar_media_path
FROM user_profiles up
JOIN users u ON u.id = up.user_id
LEFT JOIN spaces ds ON ds.id = up.japa_stage_dest_space_id
LEFT JOIN user_privacy_settings ups ON ups.user_id = up.user_id
LEFT JOIN media am ON am.id = up.avatar_media_id AND am.deleted_at IS NULL
WHERE %s
ORDER BY %s
LIMIT $%d OFFSET $%d
`, strings.Join(listPredicates, "\n  AND "), directoryOrderBy(sort), len(listArgs)+1, len(listArgs)+2)

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

	// The count query joins users so the q predicate (u.username) resolves
	// identically to the list query.
	countQuery := "SELECT COUNT(*) FROM user_profiles up\nJOIN users u ON u.id = up.user_id\nWHERE " + strings.Join(countPredicates, "\n  AND ")

	var total int64
	if err := s.pool.Reader.QueryRow(ctx, countQuery, countArgs...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("japastage: count by stage: %w", err)
	}

	return out, total, nil
}

// StageCountRow is one stage→count bucket for the discovery hub stats.
type StageCountRow struct {
	Stage string
	Count int64
}

// DestinationStatRow is one top-destination bucket for the stats endpoint.
type DestinationStatRow struct {
	SpaceID pgtype.UUID
	Name    *string
	GeoCode *string
	Count   int64
}

// StageCounts aggregates public staged profiles per stage (§87 discovery
// hub). intent, when non-empty, scopes the buckets to that journey class.
// Also returns the total count of staged public profiles.
func (s *Store) StageCounts(ctx context.Context, intent string) ([]StageCountRow, int64, error) {
	q := `
SELECT up.japa_stage, COUNT(*)
FROM user_profiles up
WHERE up.japa_stage_is_public = true
  AND up.japa_stage IS NOT NULL`
	args := []any{}
	if intent != "" {
		q += fmt.Sprintf(" AND up.japa_intent = $%d", len(args)+1)
		args = append(args, intent)
	}
	q += " GROUP BY up.japa_stage"

	rows, err := s.pool.Reader.Query(ctx, q, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("japastage: stage counts: %w", err)
	}
	defer rows.Close()

	out := []StageCountRow{}
	var total int64
	for rows.Next() {
		var r StageCountRow
		if err := rows.Scan(&r.Stage, &r.Count); err != nil {
			return nil, 0, fmt.Errorf("japastage: scan stage count: %w", err)
		}
		total += r.Count
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("japastage: iterate stage counts: %w", err)
	}

	return out, total, nil
}

// TopDestinations returns the destination spaces with the most staged
// public members, ordered by member count (§87 discovery hub).
func (s *Store) TopDestinations(ctx context.Context, limit int32) ([]DestinationStatRow, error) {
	if limit <= 0 {
		limit = 5
	}
	const q = `
SELECT ds.id, ds.name, ds.geo_code, COUNT(*) AS members
FROM user_profiles up
JOIN spaces ds ON ds.id = up.japa_stage_dest_space_id
WHERE up.japa_stage_is_public = true
  AND up.japa_stage IS NOT NULL
GROUP BY ds.id, ds.name, ds.geo_code
ORDER BY members DESC, ds.name
LIMIT $1`

	rows, err := s.pool.Reader.Query(ctx, q, limit)
	if err != nil {
		return nil, fmt.Errorf("japastage: top destinations: %w", err)
	}
	defer rows.Close()

	out := []DestinationStatRow{}
	for rows.Next() {
		var r DestinationStatRow
		if err := rows.Scan(&r.SpaceID, &r.Name, &r.GeoCode, &r.Count); err != nil {
			return nil, fmt.Errorf("japastage: scan destination stat: %w", err)
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("japastage: iterate destination stats: %w", err)
	}

	return out, nil
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
			&r.Bio, &r.Profession, &r.Interests, &r.Verified, &r.CoverURL,
			&r.LastActiveAt, &r.AvatarMediaBucket, &r.AvatarMediaPath,
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
