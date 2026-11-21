package credentialchecker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// guideFieldValues mirrors the guide_credential_fields custom-field
// group keys the publish hook reads (§94 editorial integration).
// Scalar fields are typed `any` — a single corrupt value must not fail
// the entire unmarshal and skip provisioning for the healthy keys.
type guideFieldValues struct {
	PathwayID              any             `json:"pathway_id"`
	Profession             any             `json:"profession"`
	IssuingCountry         any             `json:"issuing_country"`
	DestinationCountry     any             `json:"destination_country"`
	RegulatoryBody         any             `json:"regulatory_body"`
	PathwayType            any             `json:"pathway_type"`
	PathwaySteps           json.RawMessage `json:"pathway_steps"`
	TimelineEstimate       any             `json:"timeline_estimate"`
	CostEstimate           any             `json:"cost_estimate"`
	OfficialApplicationURL any             `json:"official_application_url"`
}

// guideString coerces a scalar field value to string; non-strings yield
// "" (which the combination check treats as missing).
func guideString(v any) string {
	s, _ := v.(string)
	return s
}

// validPathwayTypes is the §94 pathway_type enum mirrored by the
// editorial select options — anything else means the combination is
// incomplete rather than wrong.
var validPathwayTypes = map[string]bool{
	PathwayDirect:              true,
	PathwayBridgingExam:        true,
	PathwaySupervisedPractice:  true,
	PathwayFullRequalification: true,
}

// OnGuidePublished implements content.GuidePublishHook (wired in
// server.go — the interface lives in the content package). When a
// published credential-category guide has no credential_pathways row
// for its profession × issuing × destination corridor — the pathway's
// natural key — one is auto-created as a 'draft' seeded from the
// guide's structured fields, and both link columns are stamped so the
// cross-link becomes explicit rather than computed. Created rows land
// as drafts: publication stays a human decision (ADR-088 HITL).
//
// Idempotent — an existing pathway (explicit pathway_id claim or a
// combination match in any status) is linked instead of duplicated, so
// republishing a guide is a no-op. Invocations for non-credential
// categories or incomplete field combinations return nil.
func (s *Service) OnGuidePublished(ctx context.Context, contentID pgtype.UUID, guideCategory string, fieldValues []byte) error {
	if guideCategory != "credential" {
		return nil
	}
	// The tool being flagged off must not accumulate provisioned rows.
	if !s.Enabled(ctx) {
		return nil
	}

	var fv guideFieldValues
	if len(fieldValues) > 0 {
		if err := json.Unmarshal(fieldValues, &fv); err != nil {
			return fmt.Errorf("credentialchecker: guide field_values parse: %w", err)
		}
	}

	// An explicit pathway_id is authoritative when it resolves — the
	// editor chose that pathway. A stale/unresolvable id falls through
	// to combination resolution exactly like an empty field.
	if pid := strings.TrimSpace(guideString(fv.PathwayID)); pid != "" {
		var id pgtype.UUID
		if err := id.Scan(pid); err == nil {
			if _, err := s.store.GetPathwayByID(ctx, id); err == nil {
				return s.store.LinkPathwayToContent(ctx, id, contentID)
			}
		}
	}

	profSlug := strings.ToLower(strings.TrimSpace(guideString(fv.Profession)))
	issGeo := strings.ToUpper(strings.TrimSpace(guideString(fv.IssuingCountry)))
	destGeo := strings.ToUpper(strings.TrimSpace(guideString(fv.DestinationCountry)))
	if profSlug == "" || issGeo == "" || destGeo == "" || issGeo == destGeo {
		return nil
	}

	profID, err := s.store.ProfessionIDBySlug(ctx, profSlug)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil // unknown profession slug — nothing resolvable
		}
		return fmt.Errorf("credentialchecker: profession slug lookup: %w", err)
	}

	issSpace, err := s.store.SpaceIDByCountryGeoCode(ctx, issGeo)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil // unknown issuing geo — nothing resolvable
		}
		return fmt.Errorf("credentialchecker: issuing geo lookup: %w", err)
	}
	destSpace, err := s.store.SpaceIDByCountryGeoCode(ctx, destGeo)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		return fmt.Errorf("credentialchecker: destination geo lookup: %w", err)
	}
	if issSpace == destSpace {
		return nil
	}

	// Existing pathway for the natural key — link it instead of
	// duplicating. Any status counts: a draft still owns the corridor.
	if id, err := s.store.PathwayIDForCombination(ctx, profID, issSpace, destSpace); err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("credentialchecker: combination lookup: %w", err)
		}
	} else if id.Valid {
		return s.store.LinkPathwayToContent(ctx, id, contentID)
	}

	// A malformed steps field must not fail provisioning — a draft with
	// empty steps is still claimable by the link resolver; steps can be
	// backfilled later.
	steps := normalizeEditorialSteps(fv.PathwaySteps)
	if !isJSONArray(steps) {
		steps = nil
	}

	pathwayType := strings.ToLower(strings.TrimSpace(guideString(fv.PathwayType)))
	if !validPathwayTypes[pathwayType] {
		pathwayType = PathwayDirect
	}

	input := CreatePathwayInput{
		ProfessionID:       profID,
		IssuingSpaceID:     issSpace,
		DestinationSpaceID: destSpace,
		PathwayType:        pathwayType,
		PathwaySteps:       steps,
		Status:             StatusDraft,
		IsActive:           true,
		GuideID:            contentID,
	}
	// regulatory_body is NOT NULL — an empty draft still needs a
	// placeholder the reviewer can replace rather than a NULL violation.
	if v := strings.TrimSpace(guideString(fv.RegulatoryBody)); v != "" {
		input.RegulatoryBody = v
	}
	if v := strings.TrimSpace(guideString(fv.TimelineEstimate)); v != "" {
		input.TimelineEstimate = &v
	}
	if v := strings.TrimSpace(guideString(fv.CostEstimate)); v != "" {
		input.CostEstimateUSD = &v
	}
	if v := strings.TrimSpace(guideString(fv.OfficialApplicationURL)); v != "" {
		input.OfficialApplicationURL = &v
	}

	created, err := s.store.CreatePathway(ctx, input)
	if err != nil {
		// The (profession × issuing × destination) unique index makes a
		// concurrent auto-create lose with a 23505 — link the winner's
		// row instead of reporting a failure for a settled outcome.
		if id, lerr := s.store.PathwayIDForCombination(ctx, profID, issSpace, destSpace); lerr == nil && id.Valid {
			return s.store.LinkPathwayToContent(ctx, id, contentID)
		}
		return fmt.Errorf("credentialchecker: auto-create pathway for guide %s: %w", contentID, err)
	}

	return s.store.LinkPathwayToContent(ctx, created, contentID)
}

// normalizeEditorialSteps converts the guide's pathway_steps repeater
// rows into the credential_pathways storage shape: official_url is
// renamed to url (attestation precedent); every other key passes
// through verbatim.
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
