package attestationguide

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// guideFieldValues mirrors the guide_attestation_fields custom-field
// group keys the publish hook reads (§97 editorial integration).
// Scalar fields are typed `any` — a single corrupt value (a number where
// a string was expected) must not fail the entire unmarshal and skip
// provisioning for the healthy keys.
type guideFieldValues struct {
	PathwayID          any             `json:"pathway_id"`
	DocumentType       any             `json:"document_type"`
	IssuingCountry     any             `json:"issuing_country"`
	DestinationCountry any             `json:"destination_country"`
	ApostilleRequired  any             `json:"apostille_required"`
	LastVerified       any             `json:"last_verified"`
	PathwaySteps       json.RawMessage `json:"pathway_steps"`
}

// guideString coerces a scalar field value to string; non-strings yield
// "" (which the combination check treats as missing).
func guideString(v any) string {
	s, _ := v.(string)
	return s
}

// guideBool coerces a field value that the composer may have stored as
// either a bool or a boolish string into *bool. nil input or an
// unparseable value yields nil — the field simply isn't forwarded.
func guideBool(v any) *bool {
	switch b := v.(type) {
	case bool:
		return &b
	case string:
		if p, err := strconv.ParseBool(strings.TrimSpace(b)); err == nil {
			return &p
		}
	}
	return nil
}

// OnGuidePublished implements content.GuidePublishHook (wired in
// server.go — the interface lives in the content package). When a
// published attestation-category guide has no pathway for its
// document_type + issuing/destination corridor — the pathway's natural
// key — one is auto-created as a 'draft' seeded from the guide's
// structured fields, and field_values.pathway_id is backfilled so the
// cross-link becomes explicit rather than computed. Created rows land
// as drafts: publication stays a human decision (ADR-088 HITL).
//
// Idempotent — an existing pathway (explicit pathway_id claim or a
// combination match in any status) is claimed instead of duplicated,
// so republishing a guide is a no-op. Invocations for non-attestation
// categories or incomplete field combinations return nil.
func (s *Service) OnGuidePublished(ctx context.Context, contentID pgtype.UUID, guideCategory string, fieldValues []byte) error {
	if guideCategory != "attestation" {
		return nil
	}
	// The tool being flagged off must not accumulate provisioned rows.
	if !s.Enabled(ctx) {
		return nil
	}

	var fv guideFieldValues
	if len(fieldValues) > 0 {
		if err := json.Unmarshal(fieldValues, &fv); err != nil {
			return fmt.Errorf("attestationguide: guide field_values parse: %w", err)
		}
	}

	// An explicit pathway_id is authoritative when it resolves — the
	// editor chose that pathway. A stale/unresolvable id falls through
	// to combination resolution exactly like an empty field.
	if pid := strings.TrimSpace(guideString(fv.PathwayID)); pid != "" {
		var id pgtype.UUID
		if err := id.Scan(pid); err == nil {
			if _, err := s.store.GetPathwayByID(ctx, id); err == nil {
				return nil
			}
		}
	}

	docType := strings.ToLower(strings.TrimSpace(guideString(fv.DocumentType)))
	issGeo := strings.ToUpper(strings.TrimSpace(guideString(fv.IssuingCountry)))
	destGeo := strings.ToUpper(strings.TrimSpace(guideString(fv.DestinationCountry)))
	if !isValidDocumentType(docType) || issGeo == "" || destGeo == "" || issGeo == destGeo {
		return nil
	}

	issSpace, err := s.store.SpaceIDByCountryGeoCode(ctx, issGeo)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil // unknown issuing geo — nothing resolvable
		}
		return fmt.Errorf("attestationguide: issuing geo lookup: %w", err)
	}
	destSpace, err := s.store.SpaceIDByCountryGeoCode(ctx, destGeo)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		return fmt.Errorf("attestationguide: destination geo lookup: %w", err)
	}
	if issSpace == destSpace {
		return nil
	}

	// Existing pathway for the natural key — claim it instead of
	// duplicating. Any status counts: a draft still owns the corridor.
	if id, err := s.store.PathwayIDForCombination(ctx, docType, issSpace, destSpace); err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("attestationguide: combination lookup: %w", err)
		}
	} else if id.Valid {
		return s.store.BackfillGuidePathwayID(ctx, contentID, id)
	}

	// A malformed steps field (null, object, garbage) must not fail
	// provisioning — a draft with empty steps is still claimable by the
	// link resolver; the steps can be backfilled later.
	steps := normalizeEditorialSteps(fv.PathwaySteps)
	if !isJSONArray(steps) {
		steps = nil
	}

	input := CreatePathwayInput{
		DocumentType:       docType,
		IssuingSpaceID:     issSpace.String(),
		DestinationSpaceID: destSpace.String(),
		ApostilleRequired:  guideBool(fv.ApostilleRequired),
		PathwaySteps:       steps,
		Status:             PathwayStatusDraft,
	}
	// Only forward a parseable date — a free-text last_verified must not
	// fail pathway creation.
	if _, err := time.Parse("2006-01-02", strings.TrimSpace(guideString(fv.LastVerified))); err == nil {
		input.CommunityLastVerified = strings.TrimSpace(guideString(fv.LastVerified))
	}
	// Provenance for the reviewer — an auto-provisioned draft is otherwise
	// indistinguishable from a hand-created one.
	input.HITLReviewNotes = fmt.Sprintf("auto-provisioned from editorial guide %s", contentID.String())

	created, err := s.Create(ctx, input)
	if err != nil {
		// The (document_type, issuing, destination) unique index makes a
		// concurrent auto-create lose with a 23505 — claim the winner's
		// row instead of reporting a failure for a settled outcome.
		if id, lerr := s.store.PathwayIDForCombination(ctx, docType, issSpace, destSpace); lerr == nil && id.Valid {
			return s.store.BackfillGuidePathwayID(ctx, contentID, id)
		}
		return fmt.Errorf("attestationguide: auto-create pathway for guide %s: %w", contentID, err)
	}

	var createdID pgtype.UUID
	if err := createdID.Scan(created.ID); err != nil {
		return fmt.Errorf("attestationguide: parse created pathway id: %w", err)
	}

	return s.store.BackfillGuidePathwayID(ctx, contentID, createdID)
}

// normalizeEditorialSteps converts the guide's pathway_steps field
// (editorial keys) into the pathway-steps storage shape: the only
// rename is official_url → url; every other key — including the
// min/max objects and what_to_bring arrays the repeater now writes —
// passes through verbatim. Non-object items are kept as-is so scalar
// steps survive the round trip.
func normalizeEditorialSteps(raw json.RawMessage) json.RawMessage {
	// pathway_steps is NOT NULL — absent or unparseable input must yield
	// '[]', not nil, or provisioning hits a 23502.
	if len(raw) == 0 {
		return json.RawMessage("[]")
	}

	var items []json.RawMessage
	if err := json.Unmarshal(raw, &items); err != nil || len(items) == 0 {
		return json.RawMessage("[]")
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
