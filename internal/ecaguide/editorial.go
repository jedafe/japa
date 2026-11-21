package ecaguide

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

// guideFieldValues mirrors the guide_eca_fields custom-field group keys
// the publish hook reads (§109 editorial integration). Scalar fields are
// typed `any` — a single corrupt value must not fail the entire unmarshal
// and skip provisioning for the healthy keys (attestation precedent).
type guideFieldValues struct {
	GuideID            any             `json:"guide_id"`
	Provider           any             `json:"provider"`
	DestinationCountry any             `json:"destination_country"`
	Purpose            any             `json:"purpose"`
	PathwaySteps       json.RawMessage `json:"pathway_steps"`
	DocumentsRequired  any             `json:"documents_required"`
	AcceptedFor        any             `json:"accepted_for"`
	FeeEstimate        any             `json:"fee_estimate"`
	Currency           any             `json:"currency"`
	TimelineEstimate   any             `json:"timeline_estimate"`
	OfficialURL        any             `json:"official_url"`
	LastVerified       any             `json:"last_verified"`
}

// guideString coerces a scalar field value to string; non-strings yield
// "" (which the combination check treats as missing).
func guideString(v any) string {
	s, _ := v.(string)
	return s
}

// guideStringList converts the one-per-line textarea fields
// (documents_required, accepted_for) into the jsonb string-array shape
// eca_guides stores. Already-JSON values (an editor pasted an array)
// pass through when they unmarshal as a string list.
func guideStringList(v any) []byte {
	var arr []string
	switch raw := v.(type) {
	case []any:
		for _, item := range raw {
			if s := strings.TrimSpace(guideString(item)); s != "" {
				arr = append(arr, s)
			}
		}
	case string:
		if parsed := tryJSONStringArray(raw); parsed != nil {
			return []byte(strings.TrimSpace(raw))
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

	b, err := json.Marshal(arr)
	if err != nil {
		return nil
	}

	return b
}

// tryJSONStringArray parses a raw string as a JSON array of strings;
// returns nil when the input isn't a parseable string array.
func tryJSONStringArray(raw string) []string {
	trimmed := strings.TrimSpace(raw)
	if !strings.HasPrefix(trimmed, "[") {
		return nil
	}

	var arr []string
	if err := json.Unmarshal([]byte(trimmed), &arr); err != nil {
		return nil
	}

	return arr
}

// OnGuidePublished implements content.GuidePublishHook (wired in
// server.go — the interface lives in the content package). When a
// published eca-category guide has no eca_guides row for its
// provider × destination × purpose combination — the guide's natural
// key — one is auto-created as a 'draft' seeded from the guide's
// structured fields, and both link columns are stamped so the
// cross-link becomes explicit rather than computed. Created rows land
// as drafts: publication stays a human decision (ADR-088 HITL).
//
// Idempotent — an existing row (explicit guide_id claim or a
// combination match in any status) is linked instead of duplicated, so
// republishing a guide is a no-op. Invocations for non-eca categories
// or incomplete field combinations return nil.
func (s *Service) OnGuidePublished(ctx context.Context, contentID pgtype.UUID, guideCategory string, fieldValues []byte) error {
	if guideCategory != "eca" {
		return nil
	}
	// The tool being flagged off must not accumulate provisioned rows.
	if !s.Enabled(ctx) {
		return nil
	}

	var fv guideFieldValues
	if len(fieldValues) > 0 {
		if err := json.Unmarshal(fieldValues, &fv); err != nil {
			return fmt.Errorf("ecaguide: guide field_values parse: %w", err)
		}
	}

	// An explicit guide_id is authoritative when it resolves — the
	// editor chose that row. A stale/unresolvable id falls through to
	// combination resolution exactly like an empty field.
	if gid := strings.TrimSpace(guideString(fv.GuideID)); gid != "" {
		var id pgtype.UUID
		if err := id.Scan(gid); err == nil {
			if _, err := s.store.GetGuide(ctx, id); err == nil {
				return s.store.LinkGuideToContent(ctx, id, contentID)
			}
		}
	}

	destGeo := strings.ToUpper(strings.TrimSpace(guideString(fv.DestinationCountry)))
	providerSlug := strings.ToLower(strings.TrimSpace(guideString(fv.Provider)))
	purpose := strings.ToLower(strings.TrimSpace(guideString(fv.Purpose)))
	if providerSlug == "" || destGeo == "" || !validPurpose(purpose) {
		return nil
	}

	destSpace, err := s.store.SpaceIDByCountryGeoCode(ctx, destGeo)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil // unknown destination geo — nothing resolvable
		}
		return fmt.Errorf("ecaguide: destination geo lookup: %w", err)
	}

	providerID, err := s.store.ProviderIDBySlug(ctx, providerSlug)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil // unknown/inactive provider slug — nothing resolvable
		}
		return fmt.Errorf("ecaguide: provider slug lookup: %w", err)
	}

	// Existing row for the natural key — link it instead of duplicating.
	// Any status counts: a draft still owns the combination.
	if id, err := s.store.GuideIDForCombination(ctx, destSpace, providerID, purpose); err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("ecaguide: combination lookup: %w", err)
		}
	} else if id.Valid {
		return s.store.LinkGuideToContent(ctx, id, contentID)
	}

	// A malformed steps field must not fail provisioning — a draft with
	// empty steps is still claimable by the link resolver (attestation
	// precedent); steps can be backfilled later.
	steps := normalizeEditorialSteps(fv.PathwaySteps)
	if !isJSONArray(steps) {
		steps = nil
	}

	row := GuideRow{
		DestinationSpaceID: destSpace,
		ProviderID:         providerID,
		Purpose:            purpose,
		PathwaySteps:       steps,
		DocumentsRequired:  guideStringList(fv.DocumentsRequired),
		AcceptedFor:        guideStringList(fv.AcceptedFor),
		Status:             StatusDraft,
		GuideID:            contentID,
	}
	if v := strings.TrimSpace(guideString(fv.FeeEstimate)); v != "" {
		row.FeeEstimate = &v
	}
	if v := strings.ToUpper(strings.TrimSpace(guideString(fv.Currency))); len(v) == 3 {
		row.Currency = &v
	}
	if v := strings.TrimSpace(guideString(fv.TimelineEstimate)); v != "" {
		row.TimelineEstimate = &v
	}
	if v := strings.TrimSpace(guideString(fv.OfficialURL)); v != "" {
		row.OfficialURL = &v
	}
	// Only forward a parseable date — free-text last_verified must not
	// fail provisioning.
	if v := strings.TrimSpace(guideString(fv.LastVerified)); v != "" {
		if t, err := time.Parse("2006-01-02", v); err == nil {
			row.SourceURLLastChecked = pgtype.Date{Time: t, Valid: true}
		}
	}

	created, err := s.store.CreateGuide(ctx, row)
	if err != nil {
		// The (dest × provider × purpose) unique index makes a concurrent
		// auto-create lose with a 23505 — link the winner's row instead
		// of reporting a failure for a settled outcome.
		if id, lerr := s.store.GuideIDForCombination(ctx, destSpace, providerID, purpose); lerr == nil && id.Valid {
			return s.store.LinkGuideToContent(ctx, id, contentID)
		}
		return fmt.Errorf("ecaguide: auto-create guide for content %s: %w", contentID, err)
	}

	return s.store.LinkGuideToContent(ctx, created, contentID)
}

// normalizeEditorialSteps converts the guide's pathway_steps repeater
// rows into the eca_guides storage shape: official_url is renamed to
// url (attestation precedent — the editorial item_fields key is
// official_url while the public step renderer reads url); every other
// key passes through verbatim.
func normalizeEditorialSteps(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 {
		return raw
	}

	var items []json.RawMessage
	if err := json.Unmarshal(raw, &items); err != nil || len(items) == 0 {
		return raw
	}

	out := make([]json.RawMessage, 0, len(items))
	changed := false
	for _, item := range items {
		var obj map[string]json.RawMessage
		if err := json.Unmarshal(item, &obj); err != nil {
			out = append(out, item)
			continue
		}
		if v, ok := obj["official_url"]; ok {
			if _, hasURL := obj["url"]; !hasURL {
				obj["url"] = v
			}
			delete(obj, "official_url")
			changed = true
		}
		b, err := json.Marshal(obj)
		if err != nil {
			out = append(out, item)
			continue
		}
		out = append(out, b)
	}
	if !changed {
		return raw
	}

	res, err := json.Marshal(out)
	if err != nil {
		return raw
	}
	return res
}
