package housingguide

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// guideFieldValues mirrors the guide_accommodation_fields custom-field
// group keys the publish hook reads (§96 editorial integration). Scalar
// fields are typed `any` — a single corrupt value must not fail the
// entire unmarshal and skip provisioning for the healthy keys
// (attestation precedent).
type guideFieldValues struct {
	GuideID              any             `json:"guide_id"`
	DestinationCountry   any             `json:"destination_country"`
	City                 any             `json:"city"`
	AccommodationType    any             `json:"accommodation_type"`
	SearchPlatforms      json.RawMessage `json:"search_platforms"`
	RentRanges           json.RawMessage `json:"rent_ranges"`
	LandlordRequirements any             `json:"landlord_requirements"`
	ScamCategories       any             `json:"scam_categories"`
	Tips                 any             `json:"tips"`
	LastVerified         any             `json:"last_verified"`
}

// guideString coerces a scalar field value to string; non-strings yield
// "" (which the combination check treats as missing).
func guideString(v any) string {
	s, _ := v.(string)
	return s
}

// guideStringList converts the one-per-line scam_categories textarea into
// the text[] shape housing_guides stores. Already-JSON values (an editor
// pasted an array) pass through when they unmarshal as a string list.
func guideStringList(v any) []string {
	var arr []string
	switch raw := v.(type) {
	case []any:
		for _, item := range raw {
			if s := strings.TrimSpace(guideString(item)); s != "" {
				arr = append(arr, s)
			}
		}
	case string:
		trimmed := strings.TrimSpace(raw)
		if strings.HasPrefix(trimmed, "[") {
			var parsed []string
			if err := json.Unmarshal([]byte(trimmed), &parsed); err == nil {
				return parsed
			}
		}
		for _, line := range strings.Split(raw, "\n") {
			if s := strings.TrimSpace(line); s != "" {
				arr = append(arr, s)
			}
		}
	default:
		return nil
	}

	if len(arr) == 0 {
		return nil
	}

	return arr
}

// normalizeRentRanges converts the guide's rent_ranges repeater rows
// ({tier, studio, one_bed, two_bed, currency}) into the §96 object shape
// housing_guides stores: {tier: {studio, 1bed, 2bed, currency}}. Rows
// without a tier key are skipped; a malformed field yields nil so a bad
// repeater never fails provisioning (attestation precedent).
func normalizeRentRanges(raw json.RawMessage) []byte {
	if len(raw) == 0 {
		return nil
	}

	var rows []map[string]any
	if err := json.Unmarshal(raw, &rows); err != nil || len(rows) == 0 {
		return nil
	}

	out := map[string]map[string]any{}
	for _, row := range rows {
		tier := strings.ToLower(strings.TrimSpace(guideString(row["tier"])))
		if tier == "" {
			continue
		}
		entry := map[string]any{}
		if v := strings.TrimSpace(guideString(row["studio"])); v != "" {
			entry["studio"] = v
		}
		if v := strings.TrimSpace(guideString(row["one_bed"])); v != "" {
			entry["1bed"] = v
		}
		if v := strings.TrimSpace(guideString(row["two_bed"])); v != "" {
			entry["2bed"] = v
		}
		if v := strings.ToUpper(strings.TrimSpace(guideString(row["currency"]))); len(v) == 3 {
			entry["currency"] = v
		}
		if len(entry) > 0 {
			out[tier] = entry
		}
	}

	if len(out) == 0 {
		return nil
	}

	b, err := json.Marshal(out)
	if err != nil {
		return nil
	}

	return b
}

// isJSONArray reports whether b holds a JSON array (the
// housing_guides.search_platforms shape). A malformed platforms field
// must not fail provisioning — the draft is still claimable and the
// platform list can be backfilled later.
func isJSONArray(b []byte) bool {
	var arr []any
	return json.Unmarshal(b, &arr) == nil
}

// OnGuidePublished implements content.GuidePublishHook (wired in
// server.go — the interface lives in the content package). When a
// published accommodation-category guide has no housing_guides row for
// its destination × city × accommodation_type combination — the guide's
// natural key — one is auto-created as a 'draft' seeded from the guide's
// structured fields, and both link columns are stamped so the cross-link
// becomes explicit rather than computed. Created rows land as drafts:
// publication stays a human decision (ADR-088 HITL).
//
// Idempotent — an existing row (explicit guide_id claim or a combination
// match in any status) is linked instead of duplicated, so republishing
// a guide is a no-op. Invocations for non-accommodation categories or
// missing destinations return nil.
func (s *Service) OnGuidePublished(ctx context.Context, contentID pgtype.UUID, guideCategory string, fieldValues []byte) error {
	if guideCategory != "accommodation" {
		return nil
	}
	// The tool being flagged off must not accumulate provisioned rows.
	if !s.Enabled(ctx) {
		return nil
	}

	var fv guideFieldValues
	if len(fieldValues) > 0 {
		if err := json.Unmarshal(fieldValues, &fv); err != nil {
			return fmt.Errorf("housingguide: guide field_values parse: %w", err)
		}
	}

	// An explicit guide_id is authoritative when it resolves — the
	// editor chose that row. A stale/unresolvable id falls through to
	// combination resolution exactly like an empty field.
	if gid := strings.TrimSpace(guideString(fv.GuideID)); gid != "" {
		var id pgtype.UUID
		if err := id.Scan(gid); err == nil {
			if _, err := s.store.GetGuideByID(ctx, id); err == nil {
				return s.store.LinkGuideToContent(ctx, id, contentID)
			}
		}
	}

	destGeo := strings.ToUpper(strings.TrimSpace(guideString(fv.DestinationCountry)))
	if destGeo == "" {
		return nil
	}

	destSpace, err := s.store.SpaceIDByCountryGeoCode(ctx, destGeo)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil // unknown destination geo — nothing resolvable
		}
		return fmt.Errorf("housingguide: destination geo lookup: %w", err)
	}

	// City is optional — §96 says NULL city_space_id is a country-level
	// guide. An unresolvable name falls back to the country-level row
	// rather than failing the publish.
	var citySpace pgtype.UUID
	if city := strings.TrimSpace(guideString(fv.City)); city != "" {
		if id, err := s.store.CitySpaceIDByName(ctx, destSpace, city); err == nil {
			citySpace = id
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("housingguide: city lookup: %w", err)
		}
	}

	accommodationType := strings.ToLower(strings.TrimSpace(guideString(fv.AccommodationType)))

	// Existing row for the natural key — link it instead of duplicating.
	// Any status counts: a draft still owns the combination.
	if id, err := s.store.GuideIDForCombination(ctx, destSpace, citySpace, accommodationType); err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("housingguide: combination lookup: %w", err)
		}
	} else if id.Valid {
		return s.store.LinkGuideToContent(ctx, id, contentID)
	}

	// Malformed structured fields must not fail provisioning — a sparse
	// draft is still claimable by the link resolver; content can be
	// backfilled later (attestation precedent).
	platforms := []byte(fv.SearchPlatforms)
	if !isJSONArray(platforms) {
		platforms = nil
	}

	notes := fmt.Sprintf("auto-provisioned from editorial guide %s", contentID.String())
	input := CreateGuideInput{
		DestinationSpaceID: destSpace,
		CitySpaceID:        citySpace,
		SearchPlatforms:    platforms,
		RentRanges:         normalizeRentRanges(fv.RentRanges),
		ScamCategories:     guideStringList(fv.ScamCategories),
		Status:             StatusDraft,
		IsActive:           true,
		GuideID:            contentID,
		HITLReviewNotes:    &notes,
	}
	if accommodationType != "" {
		input.AccommodationType = &accommodationType
	}
	if v := strings.TrimSpace(guideString(fv.LandlordRequirements)); v != "" {
		input.LandlordRequirements = &v
	}
	if v := strings.TrimSpace(guideString(fv.Tips)); v != "" {
		input.Tips = &v
	}
	// Only forward a parseable date — free-text last_verified must not
	// fail provisioning.
	if v := strings.TrimSpace(guideString(fv.LastVerified)); v != "" {
		if t, err := time.Parse("2006-01-02", v); err == nil {
			input.LastVerifiedAt = pgtype.Date{Time: t, Valid: true}
		}
	}

	created, err := s.store.CreateGuide(ctx, input)
	if err != nil {
		// A concurrent auto-create (or an admin-created row inserted
		// between the lookup above and this insert) hits the NULLS NOT
		// DISTINCT natural-key index and surfaces as ErrNoRows —
		// re-check the combination and link the winner instead of
		// reporting a failure for a settled outcome.
		if id, lerr := s.store.GuideIDForCombination(ctx, destSpace, citySpace, accommodationType); lerr == nil && id.Valid {
			return s.store.LinkGuideToContent(ctx, id, contentID)
		}
		return fmt.Errorf("housingguide: auto-create guide for content %s: %w", contentID, err)
	}

	return s.store.LinkGuideToContent(ctx, created, contentID)
}
