package promoter

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/db/gen"
	"github.com/jedafe/japa/internal/intelligence/extractor"
	"github.com/jedafe/japa/internal/intelligence/extractor/schemas"
	"github.com/jedafe/japa/internal/intelligence/reviewerctx"
)

// CredentialStore is the persistence surface the CredentialPromoter needs.
// *stores.Store satisfies this interface; tests substitute a mock.
type CredentialStore interface {
	PersonaResolver
	GetIntelRecord(ctx context.Context, tenantID, id pgtype.UUID) (gen.IntelRecord, error)
	GetIntelSource(ctx context.Context, tenantID, id pgtype.UUID) (gen.IntelSource, error)
	GetCredentialPathwayByIntelRecord(ctx context.Context, tenantID, intelRecordID pgtype.UUID) (gen.CredentialPathway, error)
	CreateCredentialPathwayIntel(ctx context.Context, tenantID pgtype.UUID, arg gen.CreateCredentialPathwayIntelParams) (gen.CredentialPathway, error)
	UpdateCredentialPathwayIntel(ctx context.Context, tenantID pgtype.UUID, arg gen.UpdateCredentialPathwayIntelParams) error
	UpdateIntelRecordStatus(ctx context.Context, tenantID pgtype.UUID, arg gen.UpdateIntelRecordStatusParams) error
	CreateIntelReviewLog(ctx context.Context, tenantID pgtype.UUID, arg gen.CreateIntelReviewLogParams) error
	GetCountrySpace(ctx context.Context, tenantID pgtype.UUID, countryCode string) (pgtype.UUID, error)
	GetCredentialProfessionBySlug(ctx context.Context, slug string) (gen.CredentialProfession, error)
	GetCredentialProfessionByName(ctx context.Context, name string) (gen.CredentialProfession, error)
	WithTx(ctx context.Context, fn func(CredentialTxStore) error) error
}

// CredentialTxStore is the subset of CredentialStore needed inside a transaction.
type CredentialTxStore interface {
	CreateCredentialPathwayIntel(ctx context.Context, tenantID pgtype.UUID, arg gen.CreateCredentialPathwayIntelParams) (gen.CredentialPathway, error)
	UpdateIntelRecordStatus(ctx context.Context, tenantID pgtype.UUID, arg gen.UpdateIntelRecordStatusParams) error
	UpdateCredentialPathwayIntel(ctx context.Context, tenantID pgtype.UUID, arg gen.UpdateCredentialPathwayIntelParams) error
	CreateIntelReviewLog(ctx context.Context, tenantID pgtype.UUID, arg gen.CreateIntelReviewLogParams) error
}

// CredentialPromoter promotes credential pathway intel_records into the
// credential_pathways projection table (§94, ADR-102, ADR-076 Amendment 1).
type CredentialPromoter struct {
	store    CredentialStore
	tenantID pgtype.UUID
	authorID pgtype.UUID
	aiModel  *string
	now      func() time.Time
}

// CredentialPromoterOption configures a CredentialPromoter.
type CredentialPromoterOption func(*CredentialPromoter)

// WithCredentialClock overrides the clock used for timestamps (tests).
func WithCredentialClock(fn func() time.Time) CredentialPromoterOption {
	return func(p *CredentialPromoter) { p.now = fn }
}

// WithCredentialAIModel sets the AI model name stamped on created rows.
func WithCredentialAIModel(model string) CredentialPromoterOption {
	return func(p *CredentialPromoter) { p.aiModel = &model }
}

// NewCredentialPromoter creates a credential promoter backed by the given store.
func NewCredentialPromoter(store CredentialStore, tenantID, authorID pgtype.UUID, opts ...CredentialPromoterOption) *CredentialPromoter {
	p := &CredentialPromoter{store: store, tenantID: tenantID, authorID: authorID, aiModel: ptr("gpt-4o-mini"), now: time.Now}
	for _, opt := range opts {
		opt(p)
	}
	return p
}

// Entity returns the entity_type this promoter handles.
func (p *CredentialPromoter) Entity() string { return schemas.EntityTypeCredentialPathway }

// ErrNotCredentialPathway is returned when Promote is called on an intel record
// whose entity_type is not "credential_pathway".
var ErrNotCredentialPathway = errors.New("promoter: intel record is not a credential_pathway")

// Promote maps the intel record identified by recordID to a credential_pathways
// projection row, creating or updating it while respecting editorial overrides.
//
//nolint:dupl // standard promoter dispatch pattern shared across domains
func (p *CredentialPromoter) Promote(ctx context.Context, recordID uuid.UUID) (PromoteResult, error) {
	pgID := uuidToPg(recordID)

	record, err := p.store.GetIntelRecord(ctx, p.tenantID, pgID)
	if err != nil {
		return PromoteResult{}, fmt.Errorf("promoter: load intel record: %w", err)
	}
	if record.EntityType != p.Entity() {
		return PromoteResult{}, ErrNotCredentialPathway
	}

	ext, err := parseExtraction(record.Extracted)
	if err != nil {
		return PromoteResult{}, fmt.Errorf("promoter: parse extracted: %w", err)
	}

	source, err := p.store.GetIntelSource(ctx, p.tenantID, record.SourceID)
	if err != nil {
		return PromoteResult{}, fmt.Errorf("promoter: load intel source: %w", err)
	}

	existing, err := p.store.GetCredentialPathwayByIntelRecord(ctx, p.tenantID, pgID)
	if err != nil {
		if isNotFound(err) {
			return p.credentialCreatePath(ctx, record, source, ext)
		}
		return PromoteResult{}, fmt.Errorf("promoter: load existing credential pathway: %w", err)
	}

	return p.credentialUpdatePath(ctx, record, existing, ext)
}

// credentialCreatePath inserts a credential_pathways row and flips the intel
// record to 'extracted'.
func (p *CredentialPromoter) credentialCreatePath(ctx context.Context, record gen.IntelRecord, _ gen.IntelSource, ext extractor.Extraction) (PromoteResult, error) {
	professionID, issuingSpaceID, destinationSpaceID, err := p.resolveCredentialKeys(ctx, ext)
	if err != nil {
		return PromoteResult{}, err
	}

	params := p.buildCredentialPathwayCreateParams(ctx, record, ext)
	params.ProfessionID = professionID
	params.IssuingSpaceID = issuingSpaceID
	params.DestinationSpaceID = destinationSpaceID

	var result PromoteResult
	err = p.store.WithTx(ctx, func(tx CredentialTxStore) error {
		pathway, err := tx.CreateCredentialPathwayIntel(ctx, p.tenantID, params)
		if err != nil {
			return fmt.Errorf("promoter: create credential pathway: %w", err)
		}
		if err := tx.UpdateIntelRecordStatus(ctx, p.tenantID, gen.UpdateIntelRecordStatusParams{
			ID:     record.ID,
			Status: "extracted",
		}); err != nil {
			return fmt.Errorf("promoter: mark intel record extracted: %w", err)
		}
		result = PromoteResult{
			DomainID:  pgToUUID(pathway.ID),
			ContentID: uuid.Nil,
			Action:    ActionCreated,
		}
		return nil
	})
	return result, err
}

// credentialUpdatePath applies the new extraction to an existing
// credential_pathways row, skipping editorial overrides and recording an
// auto_update log entry when fields change.
func (p *CredentialPromoter) credentialUpdatePath(ctx context.Context, record gen.IntelRecord, existing gen.CredentialPathway, ext extractor.Extraction) (PromoteResult, error) {
	overrides, err := parseOverrideKeys(existing.EditorialOverrides)
	if err != nil {
		return PromoteResult{}, fmt.Errorf("promoter: parse editorial overrides: %w", err)
	}

	professionID, issuingSpaceID, destinationSpaceID, err := p.resolveCredentialKeys(ctx, ext)
	if err != nil {
		return PromoteResult{}, err
	}

	params, changed := p.buildCredentialPathwayUpdateParams(existing, ext, overrides)
	p.applyResolvedKeys(&params, overrides, professionID, issuingSpaceID, destinationSpaceID)

	if len(changed) == 0 {
		params.FreshnessScore = freshScore
		if err := p.store.WithTx(ctx, func(tx CredentialTxStore) error {
			return tx.UpdateCredentialPathwayIntel(ctx, p.tenantID, params)
		}); err != nil {
			return PromoteResult{}, fmt.Errorf("promoter: bump credential pathway freshness: %w", err)
		}
		return PromoteResult{
			DomainID: pgToUUID(existing.ID),
			Action:   ActionVerified,
		}, nil
	}

	params.FreshnessScore = freshScore
	changesJSON, err := marshalCredentialPathwayFieldChanges(existing, params, changed)
	if err != nil {
		return PromoteResult{}, fmt.Errorf("promoter: marshal field changes: %w", err)
	}
	note := "automatic re-crawl update"
	reviewLogParams := gen.CreateIntelReviewLogParams{
		IntelRecordID: record.ID,
		ActorUserID:   p.authorID,
		Action:        "auto_update",
		FieldChanges:  changesJSON,
		Note:          &note,
	}

	var result PromoteResult
	err = p.store.WithTx(ctx, func(tx CredentialTxStore) error {
		if err := tx.UpdateCredentialPathwayIntel(ctx, p.tenantID, params); err != nil {
			return fmt.Errorf("promoter: update credential pathway: %w", err)
		}
		if err := tx.CreateIntelReviewLog(ctx, p.tenantID, reviewLogParams); err != nil {
			return fmt.Errorf("promoter: write auto_update review log: %w", err)
		}
		result = PromoteResult{
			DomainID:      pgToUUID(existing.ID),
			Action:        ActionUpdated,
			ChangedFields: changed,
		}
		return nil
	})
	return result, err
}

// resolveCredentialKeys resolves the three foreign keys a credential pathway
// needs from an extraction: profession, issuing country, and destination country.
func (p *CredentialPromoter) resolveCredentialKeys(ctx context.Context, ext extractor.Extraction) (pgtype.UUID, pgtype.UUID, pgtype.UUID, error) {
	professionID, err := p.resolveProfession(ctx, ext)
	if err != nil {
		return pgtype.UUID{}, pgtype.UUID{}, pgtype.UUID{}, fmt.Errorf("promoter: resolve profession: %w", err)
	}

	issuingSpaceID, err := p.resolveCountrySpace(ctx, ext, "issuing_country")
	if err != nil {
		return pgtype.UUID{}, pgtype.UUID{}, pgtype.UUID{}, fmt.Errorf("promoter: resolve issuing country: %w", err)
	}

	destinationSpaceID, err := p.resolveCountrySpace(ctx, ext, "destination_country")
	if err != nil {
		return pgtype.UUID{}, pgtype.UUID{}, pgtype.UUID{}, fmt.Errorf("promoter: resolve destination country: %w", err)
	}

	return professionID, issuingSpaceID, destinationSpaceID, nil
}

// applyResolvedKeys stamps resolved FKs into the update params unless the
// corresponding field is under an editorial override.
func (p *CredentialPromoter) applyResolvedKeys(params *gen.UpdateCredentialPathwayIntelParams, overrides map[string]struct{}, professionID, issuingSpaceID, destinationSpaceID pgtype.UUID) {
	if _, ov := overrides["profession_id"]; !ov && professionID.Valid {
		params.ProfessionID = professionID
	}
	if _, ov := overrides["issuing_space_id"]; !ov && issuingSpaceID.Valid {
		params.IssuingSpaceID = issuingSpaceID
	}
	if _, ov := overrides["destination_space_id"]; !ov && destinationSpaceID.Valid {
		params.DestinationSpaceID = destinationSpaceID
	}
}

// resolveProfession looks up credential_professions by slug, then by name.
func (p *CredentialPromoter) resolveProfession(ctx context.Context, ext extractor.Extraction) (pgtype.UUID, error) {
	if slug := stringField(ext, "profession_slug"); slug != "" {
		prof, err := p.store.GetCredentialProfessionBySlug(ctx, slug)
		if err == nil {
			return prof.ID, nil
		}
		if !isNotFound(err) {
			return pgtype.UUID{}, err
		}
	}
	if name := stringField(ext, "profession_name"); name != "" {
		prof, err := p.store.GetCredentialProfessionByName(ctx, name)
		if err == nil {
			return prof.ID, nil
		}
		if !isNotFound(err) {
			return pgtype.UUID{}, err
		}
	}
	return pgtype.UUID{}, errors.New("profession not found")
}

// resolveCountrySpace resolves an ISO-3 country code to a spaces.id.
func (p *CredentialPromoter) resolveCountrySpace(ctx context.Context, ext extractor.Extraction, field string) (pgtype.UUID, error) {
	code := firstCountry(ext, field)
	if code == "" {
		return pgtype.UUID{}, fmt.Errorf("%s not provided", field)
	}
	id, err := p.store.GetCountrySpace(ctx, p.tenantID, code)
	if err != nil {
		return pgtype.UUID{}, err
	}
	if !id.Valid {
		return pgtype.UUID{}, fmt.Errorf("%s space not found for %s", field, code)
	}
	return id, nil
}

// buildCredentialPathwayCreateParams maps a validated extraction to a
// CreateCredentialPathwayIntelParams. Foreign keys are resolved by the caller.
func (p *CredentialPromoter) buildCredentialPathwayCreateParams(ctx context.Context, record gen.IntelRecord, ext extractor.Extraction) gen.CreateCredentialPathwayIntelParams {
	now := p.now()
	params := gen.CreateCredentialPathwayIntelParams{
		IntelRecordID:          record.ID,
		RegulatoryBody:         stringField(ext, "regulatory_body"),
		RegulatoryBodyUrl:      stringPtrField(ext, "regulatory_body_url"),
		PathwayType:            stringField(ext, "pathway_type"),
		TimelineEstimate:       stringPtrField(ext, "timeline_estimate"),
		CostEstimateUsd:        stringPtrField(ext, "cost_estimate_usd"),
		PathwayDescription:     stringField(ext, "pathway_description"),
		OfficialApplicationUrl: stringPtrField(ext, "official_application_url"),
		SourceUrlLastChecked:   pgtype.Date{Time: now, Valid: true},
		Status:                 "published",
		IsActive:               true,
		AiGenerated:            true,
		ConfidenceOverall:      record.ConfidenceOverall,
		VerificationStatus:     "verified",
		FreshnessScore:         freshScore,
		OverrideSource:         "ingestion",
		EditorialOverrides:     emptyJSONObject(),
	}
	if p.aiModel != nil {
		params.AiModel = p.aiModel
	}
	if reviewerID, ok := reviewerctx.ReviewerFromContext(ctx); ok {
		params.HitlReviewedBy = reviewerID
		params.HitlReviewedAt = pgtype.Timestamptz{Time: now, Valid: true}
	}
	return params
}

// buildCredentialPathwayUpdateParams produces an UpdateCredentialPathwayIntelParams
// seeded from the existing row, applying each extraction field that is not in
// overrides and has changed.
func (p *CredentialPromoter) buildCredentialPathwayUpdateParams(existing gen.CredentialPathway, ext extractor.Extraction, overrides map[string]struct{}) (gen.UpdateCredentialPathwayIntelParams, []string) {
	params := gen.UpdateCredentialPathwayIntelParams{
		ID:                     existing.ID,
		ProfessionID:           existing.ProfessionID,
		IssuingSpaceID:         existing.IssuingSpaceID,
		DestinationSpaceID:     existing.DestinationSpaceID,
		RegulatoryBody:         existing.RegulatoryBody,
		RegulatoryBodyUrl:      existing.RegulatoryBodyUrl,
		PathwayType:            existing.PathwayType,
		TimelineEstimate:       existing.TimelineEstimate,
		CostEstimateUsd:        existing.CostEstimateUsd,
		PathwayDescription:     existing.PathwayDescription,
		OfficialApplicationUrl: existing.OfficialApplicationUrl,
		SourceUrlLastChecked:   existing.SourceUrlLastChecked,
		Status:                 existing.Status,
		IsActive:               existing.IsActive,
		AiGenerated:            existing.AiGenerated,
		AiModel:                existing.AiModel,
		AiPromptHash:           existing.AiPromptHash,
		HitlReviewedBy:         existing.HitlReviewedBy,
		HitlReviewedAt:         existing.HitlReviewedAt,
		HitlReviewNotes:        existing.HitlReviewNotes,
		ConfidenceOverall:      existing.ConfidenceOverall,
		VerificationStatus:     existing.VerificationStatus,
		FreshnessScore:         existing.FreshnessScore,
		OverrideSource:         existing.OverrideSource,
		EditorialOverrides:     existing.EditorialOverrides,
	}

	var changed []string

	applyString := func(field, current string) {
		if _, ov := overrides[field]; ov {
			return
		}
		v := stringField(ext, field)
		if v != "" && v != current {
			setCredentialStringField(&params, field, v)
			changed = append(changed, field)
		}
	}

	applyPtr := func(field string, current *string) {
		if _, ov := overrides[field]; ov {
			return
		}
		v := stringPtrField(ext, field)
		if !strPtrEqual(v, current) {
			setCredentialStringPtrField(&params, field, v)
			changed = append(changed, field)
		}
	}

	applyString("regulatory_body", existing.RegulatoryBody)
	applyString("pathway_type", existing.PathwayType)
	applyString("pathway_description", existing.PathwayDescription)
	applyPtr("regulatory_body_url", existing.RegulatoryBodyUrl)
	applyPtr("timeline_estimate", existing.TimelineEstimate)
	applyPtr("cost_estimate_usd", existing.CostEstimateUsd)
	applyPtr("official_application_url", existing.OfficialApplicationUrl)

	params.SourceUrlLastChecked = pgtype.Date{Time: p.now(), Valid: true}

	return params, changed
}

func setCredentialStringField(p *gen.UpdateCredentialPathwayIntelParams, field, value string) {
	switch field {
	case "regulatory_body":
		p.RegulatoryBody = value
	case "pathway_type":
		p.PathwayType = value
	case "pathway_description":
		p.PathwayDescription = value
	}
}

func setCredentialStringPtrField(p *gen.UpdateCredentialPathwayIntelParams, field string, value *string) {
	switch field {
	case "regulatory_body_url":
		p.RegulatoryBodyUrl = value
	case "timeline_estimate":
		p.TimelineEstimate = value
	case "cost_estimate_usd":
		p.CostEstimateUsd = value
	case "official_application_url":
		p.OfficialApplicationUrl = value
	}
}

func marshalCredentialPathwayFieldChanges(existing gen.CredentialPathway, params gen.UpdateCredentialPathwayIntelParams, changed []string) ([]byte, error) {
	changes := make([]fieldChange, 0, len(changed))
	for _, c := range changed {
		fc := fieldChange{Field: c}
		switch c {
		case "regulatory_body":
			fc.From, fc.To = existing.RegulatoryBody, params.RegulatoryBody
		case "regulatory_body_url":
			fc.From, fc.To = derefStr(existing.RegulatoryBodyUrl), derefStr(params.RegulatoryBodyUrl)
		case "pathway_type":
			fc.From, fc.To = existing.PathwayType, params.PathwayType
		case "timeline_estimate":
			fc.From, fc.To = derefStr(existing.TimelineEstimate), derefStr(params.TimelineEstimate)
		case "cost_estimate_usd":
			fc.From, fc.To = derefStr(existing.CostEstimateUsd), derefStr(params.CostEstimateUsd)
		case "pathway_description":
			fc.From, fc.To = existing.PathwayDescription, params.PathwayDescription
		case "official_application_url":
			fc.From, fc.To = derefStr(existing.OfficialApplicationUrl), derefStr(params.OfficialApplicationUrl)
		}
		changes = append(changes, fc)
	}
	return json.Marshal(changes)
}
