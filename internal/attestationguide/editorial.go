package attestationguide

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

// guideFieldValues mirrors the guide_attestation_fields custom-field
// group keys the publish hook reads (§97 editorial integration).
type guideFieldValues struct {
	PathwayID          string          `json:"pathway_id"`
	DocumentType       string          `json:"document_type"`
	IssuingCountry     string          `json:"issuing_country"`
	DestinationCountry string          `json:"destination_country"`
	ApostilleRequired  *bool           `json:"apostille_required"`
	LastVerified       string          `json:"last_verified"`
	PathwaySteps       json.RawMessage `json:"pathway_steps"`
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

	var fv guideFieldValues
	if len(fieldValues) > 0 {
		if err := json.Unmarshal(fieldValues, &fv); err != nil {
			return fmt.Errorf("attestationguide: guide field_values parse: %w", err)
		}
	}

	// An explicit pathway_id is authoritative when it resolves — the
	// editor chose that pathway. A stale/unresolvable id falls through
	// to combination resolution exactly like an empty field.
	if pid := strings.TrimSpace(fv.PathwayID); pid != "" {
		var id pgtype.UUID
		if err := id.Scan(pid); err == nil {
			if _, err := s.store.GetPathwayByID(ctx, id); err == nil {
				return nil
			}
		}
	}

	docType := strings.TrimSpace(fv.DocumentType)
	issGeo := strings.ToUpper(strings.TrimSpace(fv.IssuingCountry))
	destGeo := strings.ToUpper(strings.TrimSpace(fv.DestinationCountry))
	if !isValidDocumentType(docType) || issGeo == "" || destGeo == "" {
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

	input := CreatePathwayInput{
		DocumentType:       docType,
		IssuingSpaceID:     issSpace.String(),
		DestinationSpaceID: destSpace.String(),
		ApostilleRequired:  fv.ApostilleRequired,
		PathwaySteps:       normalizeEditorialSteps(fv.PathwaySteps),
		Status:             PathwayStatusDraft,
	}
	// Only forward a parseable date — a free-text last_verified must not
	// fail pathway creation.
	if _, err := time.Parse("2006-01-02", strings.TrimSpace(fv.LastVerified)); err == nil {
		input.CommunityLastVerified = strings.TrimSpace(fv.LastVerified)
	}

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
