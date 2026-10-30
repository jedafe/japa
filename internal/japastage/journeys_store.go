// Package japastage — journeys_store.go is the data-access layer for
// user_journeys (ADR-136 Concurrent Journeys). Raw pgx queries following
// the package convention — the table is new and has no sqlc-generated
// queries. No business logic here; lifecycle and class-rule validation
// live in journeys.go.
//
// Ref: japaconnect-engineering/sections/87-japa-stage-profiles.md
//
//	japaconnect-engineering/adr/ADR-136-concurrent-journeys.md
package japastage

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// JourneyRow is the user_journeys row shape, joined to spaces for the
// destination display fields.
type JourneyRow struct {
	ID            pgtype.UUID
	UserID        pgtype.UUID
	Intent        *string
	Stage         *string
	DestSpaceID   pgtype.UUID
	DestSpaceName *string
	DestSpaceCode *string
	Label         *string
	TravelStart   pgtype.Date
	TravelEnd     pgtype.Date
	Status        *string
	IsPublic      bool
	History       []byte // raw jsonb
	CreatedAt     pgtype.Timestamptz
	UpdatedAt     pgtype.Timestamptz
}

// journeyColumns is the shared SELECT list for journey queries — the
// destination join resolves display names the same way the profile
// queries resolve japa_stage_dest_space_id (ADR-053 country spaces).
const journeyColumns = `
SELECT j.id, j.user_id, j.intent, j.stage, j.destination_space_id,
       ds.name AS dest_space_name, ds.geo_code AS dest_space_code,
       j.label, j.travel_start, j.travel_end, j.status, j.is_public,
       j.history, j.created_at, j.updated_at
FROM user_journeys j
LEFT JOIN spaces ds ON ds.id = j.destination_space_id
`

func scanJourneyRows(rows pgx.Rows) ([]JourneyRow, error) {
	var out []JourneyRow

	for rows.Next() {
		var r JourneyRow
		if err := rows.Scan(
			&r.ID, &r.UserID, &r.Intent, &r.Stage, &r.DestSpaceID,
			&r.DestSpaceName, &r.DestSpaceCode,
			&r.Label, &r.TravelStart, &r.TravelEnd, &r.Status, &r.IsPublic,
			&r.History, &r.CreatedAt, &r.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("japastage: scan journey row: %w", err)
		}

		out = append(out, r)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("japastage: iterate journey rows: %w", err)
	}

	return out, nil
}

// ListJourneys returns the member's non-deleted journeys (including
// cancelled — ADR-136 keeps cancelled trips visible) ordered by recency.
func (s *Store) ListJourneys(ctx context.Context, userID pgtype.UUID) ([]JourneyRow, error) {
	const q = journeyColumns + `
WHERE j.user_id = $1 AND j.deleted_at IS NULL
ORDER BY j.updated_at DESC, j.id
`

	rows, err := s.pool.Reader.Query(ctx, q, userID)
	if err != nil {
		return nil, fmt.Errorf("japastage: list journeys: %w", err)
	}

	defer rows.Close()

	return scanJourneyRows(rows)
}

// GetJourney returns a single journey owned by the member, or
// pgx.ErrNoRows when absent/deleted/foreign.
func (s *Store) GetJourney(ctx context.Context, userID, journeyID pgtype.UUID) (JourneyRow, error) {
	const q = journeyColumns + `
WHERE j.id = $2 AND j.user_id = $1 AND j.deleted_at IS NULL
LIMIT 1
`

	rows, err := s.pool.Reader.Query(ctx, q, userID, journeyID)
	if err != nil {
		return JourneyRow{}, fmt.Errorf("japastage: get journey: %w", err)
	}

	defer rows.Close()

	found, err := scanJourneyRows(rows)
	if err != nil {
		return JourneyRow{}, err
	}

	if len(found) == 0 {
		return JourneyRow{}, pgx.ErrNoRows
	}

	return found[0], nil
}

// CountLiveJourneys counts the member's non-deleted journeys in a live
// lifecycle state (planned|active) — the input to the
// japa_stage.max_journeys cap (ADR-136). Completed and cancelled journeys
// are history and do not count against the cap.
func (s *Store) CountLiveJourneys(ctx context.Context, userID pgtype.UUID) (int64, error) {
	const q = `SELECT COUNT(*) FROM user_journeys WHERE user_id = $1 AND deleted_at IS NULL AND status IN ('planned','active')`

	var n int64
	if err := s.pool.Reader.QueryRow(ctx, q, userID).Scan(&n); err != nil {
		return 0, fmt.Errorf("japastage: count journeys: %w", err)
	}

	return n, nil
}

// CreateJourney inserts a journey row and seeds history with the initial
// state (source 'journey_ui' — members create journeys from the
// /japa-stage/me surface). Returns the full row via RETURNING.
func (s *Store) CreateJourney(ctx context.Context, userID pgtype.UUID, intent, stage, destSpaceID, label, travelStart, travelEnd, status string, isPublic *bool) (JourneyRow, error) {
	const q = `
INSERT INTO user_journeys (user_id, intent, stage, destination_space_id, label, travel_start, travel_end, status, is_public, history)
VALUES (
    $1, $2, $3, $4, NULLIF(BTRIM($5::text), ''), $6, $7, $8, COALESCE($9::bool, false),
    jsonb_build_array(
        jsonb_build_object(
            'intent', to_jsonb($2::text),
            'stage', to_jsonb($3::text),
            'source', to_jsonb('journey_ui'::text),
            'set_at', to_char(now() AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"')
        )
    )
)
RETURNING id, user_id, intent, stage, destination_space_id, label,
          travel_start, travel_end, status, is_public, history,
          created_at, updated_at
`

	row, err := s.pool.Writer.Query(ctx, q,
		userID, intent, stage, uuidOrNil(destSpaceID), label,
		dateOrNil(travelStart), dateOrNil(travelEnd), status, isPublic,
	)
	if err != nil {
		return JourneyRow{}, fmt.Errorf("japastage: create journey: %w", err)
	}

	defer row.Close()

	var r JourneyRow

	if !row.Next() {
		return JourneyRow{}, errors.New("japastage: create journey: no row returned")
	}

	if err := row.Scan(&r.ID, &r.UserID, &r.Intent, &r.Stage, &r.DestSpaceID, &r.Label,
		&r.TravelStart, &r.TravelEnd, &r.Status, &r.IsPublic, &r.History,
		&r.CreatedAt, &r.UpdatedAt); err != nil {
		return JourneyRow{}, fmt.Errorf("japastage: scan created journey: %w", err)
	}

	return r, nil
}

// UpdateJourney applies a partial update validated by the service. The
// effective intent+stage (already class-checked) are written. A submitted
// value always wins over its Clear* flag; the flag only nulls the column
// when no replacement was submitted. History appends only when
// recordHistory is set — i.e. intent/stage/status actually changed —
// recording the effective triple plus the effective destination
// (destSpaceID, resolved by the service).
func (s *Store) UpdateJourney(ctx context.Context, userID, journeyID pgtype.UUID, intent, stage string, destSpaceID pgtype.UUID, recordHistory bool, in JourneyPatchInput) (JourneyRow, error) {
	const q = `
UPDATE user_journeys SET
    intent = $3,
    stage = $4,
    destination_space_id = CASE
        WHEN NULLIF($5::text, '') IS NOT NULL THEN $5::uuid
        WHEN $6::bool THEN NULL
        ELSE destination_space_id
    END,
    label = CASE
        WHEN NULLIF(BTRIM($7::text), '') IS NOT NULL THEN BTRIM($7::text)
        WHEN $8::bool THEN NULL
        ELSE label
    END,
    travel_start = CASE
        WHEN NULLIF($9::text, '') IS NOT NULL THEN $9::date
        WHEN $10::bool THEN NULL
        ELSE travel_start
    END,
    travel_end = CASE
        WHEN NULLIF($11::text, '') IS NOT NULL THEN $11::date
        WHEN $10::bool THEN NULL
        ELSE travel_end
    END,
    status = COALESCE(NULLIF($12::text, ''), status),
    is_public = COALESCE($13::bool, is_public),
    updated_at = now(),
    history = CASE
        WHEN $14::bool THEN history || jsonb_build_array(
            jsonb_build_object(
                'intent', to_jsonb($3::text),
                'stage', to_jsonb($4::text),
                'status', to_jsonb(COALESCE(NULLIF($12::text, ''), status)),
                'dest_space_id', to_jsonb($15::uuid),
                'source', to_jsonb('journey_ui'::text),
                'set_at', to_char(now() AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"')
            )
        )
        ELSE history
    END
WHERE id = $1 AND user_id = $2 AND deleted_at IS NULL
RETURNING id, user_id, intent, stage, destination_space_id, label,
          travel_start, travel_end, status, is_public, history,
          created_at, updated_at
`

	row, err := s.pool.Writer.Query(ctx, q,
		journeyID, userID, intent, stage,
		in.DestSpaceID, in.ClearDest, in.Label, in.ClearLabel,
		in.TravelStart, in.ClearWindow, in.TravelEnd,
		in.Status, in.IsPublic, recordHistory, destSpaceID,
	)
	if err != nil {
		return JourneyRow{}, fmt.Errorf("japastage: update journey: %w", err)
	}

	defer row.Close()

	if !row.Next() {
		return JourneyRow{}, pgx.ErrNoRows
	}

	var r JourneyRow
	if err := row.Scan(&r.ID, &r.UserID, &r.Intent, &r.Stage, &r.DestSpaceID, &r.Label,
		&r.TravelStart, &r.TravelEnd, &r.Status, &r.IsPublic, &r.History,
		&r.CreatedAt, &r.UpdatedAt); err != nil {
		return JourneyRow{}, fmt.Errorf("japastage: scan updated journey: %w", err)
	}

	return r, nil
}

// DeleteJourney soft-deletes a journey owned by the member (Rule 11).
// Returns pgx.ErrNoRows when absent, already deleted, or foreign-owned.
func (s *Store) DeleteJourney(ctx context.Context, userID, journeyID pgtype.UUID) error {
	const q = `UPDATE user_journeys SET deleted_at = now(), updated_at = now() WHERE id = $2 AND user_id = $1 AND deleted_at IS NULL`

	cmd, err := s.pool.Writer.Exec(ctx, q, userID, journeyID)
	if err != nil {
		return fmt.Errorf("japastage: delete journey: %w", err)
	}

	if cmd.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}

	return nil
}

// uuidOrNil parses a UUID string into pgtype.UUID, or an invalid (NULL)
// UUID for empty input.
func uuidOrNil(v string) pgtype.UUID {
	if v == "" {
		return pgtype.UUID{}
	}

	var u pgtype.UUID
	if err := u.Scan(v); err != nil {
		return pgtype.UUID{}
	}

	return u
}

// dateOrNil parses a YYYY-MM-DD date string into pgtype.Date, or an
// invalid (NULL) date for empty/invalid input. The service validates
// before calling — a bad date reaching the store degrades to NULL rather
// than erroring mid-update.
func dateOrNil(v string) pgtype.Date {
	if v == "" {
		return pgtype.Date{}
	}

	t, err := time.Parse("2006-01-02", v)
	if err != nil {
		return pgtype.Date{}
	}

	return pgtype.Date{Time: t, Valid: true}
}
