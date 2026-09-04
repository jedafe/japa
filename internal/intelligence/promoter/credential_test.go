package promoter

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/db/gen"
	"github.com/jedafe/japa/internal/intelligence/extractor"
)

// mockCredentialStore is an in-memory implementation of CredentialStore used
// to assert on the credential promoter's write sequence.
type mockCredentialStore struct {
	record         gen.IntelRecord
	source         gen.IntelSource
	existing       gen.CredentialPathway
	hasExisting    bool
	professionID   pgtype.UUID
	issuingSpaceID pgtype.UUID
	destSpaceID    pgtype.UUID
	createCalls    []gen.CreateCredentialPathwayIntelParams
	updateCalls    []gen.UpdateCredentialPathwayIntelParams
	reviewLogCalls []gen.CreateIntelReviewLogParams
	statusCalls    []gen.UpdateIntelRecordStatusParams
}

func (m *mockCredentialStore) GetIntelRecord(_ context.Context, _, _ pgtype.UUID) (gen.IntelRecord, error) {
	return m.record, nil
}

func (m *mockCredentialStore) GetIntelSource(_ context.Context, _, _ pgtype.UUID) (gen.IntelSource, error) {
	return m.source, nil
}

func (m *mockCredentialStore) GetPersonaUserID(_ context.Context, _, _ pgtype.UUID) (pgtype.UUID, error) {
	return pgtype.UUID{}, nil
}

func (m *mockCredentialStore) GetCredentialPathwayByIntelRecord(_ context.Context, _, _ pgtype.UUID) (gen.CredentialPathway, error) {
	if !m.hasExisting {
		return gen.CredentialPathway{}, errors.New("pgx: no rows in result set")
	}
	return m.existing, nil
}

func (m *mockCredentialStore) GetCredentialProfessionBySlug(_ context.Context, slug string) (gen.CredentialProfession, error) {
	if !m.professionID.Valid {
		return gen.CredentialProfession{}, errors.New("pgx: no rows in result set")
	}
	return gen.CredentialProfession{ID: m.professionID, Slug: slug, Name: "Nurse"}, nil
}

func (m *mockCredentialStore) GetCredentialProfessionByName(_ context.Context, name string) (gen.CredentialProfession, error) {
	if !m.professionID.Valid {
		return gen.CredentialProfession{}, errors.New("pgx: no rows in result set")
	}
	return gen.CredentialProfession{ID: m.professionID, Slug: "nurse", Name: name}, nil
}

func (m *mockCredentialStore) GetCountrySpace(_ context.Context, _ pgtype.UUID, countryCode string) (pgtype.UUID, error) {
	switch countryCode {
	case "USA":
		return m.issuingSpaceID, nil
	case "CAN":
		return m.destSpaceID, nil
	}
	return pgtype.UUID{}, nil
}

func (m *mockCredentialStore) CreateCredentialPathwayIntel(_ context.Context, _ pgtype.UUID, arg gen.CreateCredentialPathwayIntelParams) (gen.CredentialPathway, error) {
	m.createCalls = append(m.createCalls, arg)
	return gen.CredentialPathway{
		ID:                 pgtype.UUID{Bytes: uuid.New(), Valid: true},
		IntelRecordID:      arg.IntelRecordID,
		ProfessionID:       arg.ProfessionID,
		IssuingSpaceID:     arg.IssuingSpaceID,
		DestinationSpaceID: arg.DestinationSpaceID,
		RegulatoryBody:     arg.RegulatoryBody,
	}, nil
}

func (m *mockCredentialStore) UpdateCredentialPathwayIntel(_ context.Context, _ pgtype.UUID, arg gen.UpdateCredentialPathwayIntelParams) error {
	m.updateCalls = append(m.updateCalls, arg)
	return nil
}

func (m *mockCredentialStore) UpdateIntelRecordStatus(_ context.Context, _ pgtype.UUID, arg gen.UpdateIntelRecordStatusParams) error {
	m.statusCalls = append(m.statusCalls, arg)
	return nil
}

func (m *mockCredentialStore) CreateIntelReviewLog(_ context.Context, _ pgtype.UUID, arg gen.CreateIntelReviewLogParams) error {
	m.reviewLogCalls = append(m.reviewLogCalls, arg)
	return nil
}

func (m *mockCredentialStore) WithTx(_ context.Context, fn func(CredentialTxStore) error) error {
	return fn(m)
}

func TestCredentialPromoter_CreatePath(t *testing.T) {
	t.Parallel()
	recordID := pgtype.UUID{Bytes: uuid.New(), Valid: true}
	sourceID := pgtype.UUID{Bytes: uuid.New(), Valid: true}
	professionID := pgtype.UUID{Bytes: uuid.New(), Valid: true}
	issuingID := pgtype.UUID{Bytes: uuid.New(), Valid: true}
	destID := pgtype.UUID{Bytes: uuid.New(), Valid: true}

	ext := extractor.Extraction{
		"profession_slug":          {Value: "nurse"},
		"issuing_country":          {Value: []any{"USA"}},
		"destination_country":      {Value: []any{"CAN"}},
		"regulatory_body":          {Value: "College of Nurses"},
		"regulatory_body_url":      {Value: "https://example.com"},
		"pathway_type":             {Value: "bridging_exam"},
		"timeline_estimate":        {Value: "6-12 months"},
		"cost_estimate_usd":        {Value: "$500-$2000"},
		"pathway_description":      {Value: "Apply for recognition"},
		"official_application_url": {Value: "https://apply.example.com"},
		"source_url":               {Value: "https://example.com/source"},
	}

	store := &mockCredentialStore{
		record: gen.IntelRecord{
			ID:                recordID,
			SourceID:          sourceID,
			EntityType:        "credential_pathway",
			ConfidenceOverall: 0.92,
			Extracted:         extractionJSON(t, ext),
		},
		source: gen.IntelSource{
			ID:         sourceID,
			SourceUrl:  "https://example.com/source",
			SourceType: "official",
			TrustLevel: 1,
		},
		professionID:   professionID,
		issuingSpaceID: issuingID,
		destSpaceID:    destID,
	}

	p := NewCredentialPromoter(
		store,
		pgtype.UUID{Bytes: uuid.New(), Valid: true},
		pgtype.UUID{Bytes: uuid.New(), Valid: true},
		WithCredentialClock(func() time.Time { return time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC) }),
	)

	res, err := p.Promote(t.Context(), uuid.UUID(recordID.Bytes))
	if err != nil {
		t.Fatalf("Promote returned error: %v", err)
	}
	if res.Action != ActionCreated {
		t.Fatalf("Action = %q, want %q", res.Action, ActionCreated)
	}
	if len(store.createCalls) != 1 {
		t.Fatalf("expected 1 create call, got %d", len(store.createCalls))
	}
	if len(store.statusCalls) != 1 {
		t.Fatalf("expected 1 status update call, got %d", len(store.statusCalls))
	}
	create := store.createCalls[0]
	if create.ProfessionID != professionID {
		t.Errorf("ProfessionID mismatch")
	}
	if create.IssuingSpaceID != issuingID {
		t.Errorf("IssuingSpaceID mismatch")
	}
	if create.DestinationSpaceID != destID {
		t.Errorf("DestinationSpaceID mismatch")
	}
	if create.RegulatoryBody != "College of Nurses" {
		t.Errorf("RegulatoryBody = %q, want %q", create.RegulatoryBody, "College of Nurses")
	}
	if create.Status != "published" {
		t.Errorf("Status = %q, want %q", create.Status, "published")
	}
	if create.FreshnessScore != freshScore {
		t.Errorf("FreshnessScore = %d, want %d", create.FreshnessScore, freshScore)
	}
	if store.statusCalls[0].Status != "extracted" {
		t.Errorf("intel record status = %q, want %q", store.statusCalls[0].Status, "extracted")
	}
}

func TestCredentialPromoter_UpdateRespectsEditorialOverrides(t *testing.T) {
	t.Parallel()
	recordID := pgtype.UUID{Bytes: uuid.New(), Valid: true}
	sourceID := pgtype.UUID{Bytes: uuid.New(), Valid: true}
	pathwayID := pgtype.UUID{Bytes: uuid.New(), Valid: true}
	professionID := pgtype.UUID{Bytes: uuid.New(), Valid: true}
	issuingID := pgtype.UUID{Bytes: uuid.New(), Valid: true}
	destID := pgtype.UUID{Bytes: uuid.New(), Valid: true}

	newExt := extractor.Extraction{
		"profession_slug":     {Value: "nurse"},
		"issuing_country":     {Value: []any{"USA"}},
		"destination_country": {Value: []any{"CAN"}},
		"regulatory_body":     {Value: "New College of Nurses"},
		"pathway_type":        {Value: "direct"},
		"pathway_description": {Value: "Updated description"},
		"source_url":          {Value: "https://example.com/source"},
	}

	store := &mockCredentialStore{
		record: gen.IntelRecord{
			ID:                recordID,
			SourceID:          sourceID,
			EntityType:        "credential_pathway",
			ConfidenceOverall: 0.88,
			Extracted:         extractionJSON(t, newExt),
		},
		source: gen.IntelSource{
			ID:        sourceID,
			SourceUrl: "https://example.com/source",
		},
		hasExisting: true,
		existing: gen.CredentialPathway{
			ID:                 pathwayID,
			IntelRecordID:      recordID,
			ProfessionID:       professionID,
			IssuingSpaceID:     issuingID,
			DestinationSpaceID: destID,
			RegulatoryBody:     "Old College of Nurses",
			PathwayType:        "bridging_exam",
			PathwayDescription: "Old description",
			EditorialOverrides: []byte(`{"regulatory_body":"2026-09-01"}`),
		},
		professionID:   professionID,
		issuingSpaceID: issuingID,
		destSpaceID:    destID,
	}

	p := NewCredentialPromoter(
		store,
		pgtype.UUID{Bytes: uuid.New(), Valid: true},
		pgtype.UUID{Bytes: uuid.New(), Valid: true},
		WithCredentialClock(func() time.Time { return time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC) }),
	)

	res, err := p.Promote(t.Context(), uuid.UUID(recordID.Bytes))
	if err != nil {
		t.Fatalf("Promote returned error: %v", err)
	}
	if res.Action != ActionUpdated {
		t.Fatalf("Action = %q, want %q", res.Action, ActionUpdated)
	}
	if len(store.updateCalls) != 1 {
		t.Fatalf("expected 1 update call, got %d", len(store.updateCalls))
	}
	if len(store.reviewLogCalls) != 1 {
		t.Fatalf("expected 1 review log call, got %d", len(store.reviewLogCalls))
	}
	upd := store.updateCalls[0]
	// regulatory_body is overridden, so it must stay the old value.
	if upd.RegulatoryBody != "Old College of Nurses" {
		t.Errorf("RegulatoryBody = %q, want %q (editorial override must win)", upd.RegulatoryBody, "Old College of Nurses")
	}
	// pathway_type and pathway_description are NOT overridden, so they update.
	if upd.PathwayType != "direct" {
		t.Errorf("PathwayType = %q, want %q", upd.PathwayType, "direct")
	}
	if upd.PathwayDescription != "Updated description" {
		t.Errorf("PathwayDescription = %q, want %q", upd.PathwayDescription, "Updated description")
	}
	if !contains(res.ChangedFields, "pathway_type") {
		t.Errorf("ChangedFields = %v, want to contain %q", res.ChangedFields, "pathway_type")
	}
	if contains(res.ChangedFields, "regulatory_body") {
		t.Errorf("ChangedFields = %v, must NOT contain %q (overridden)", res.ChangedFields, "regulatory_body")
	}
}
