// Package campaign — service_test.go validates service-level validation and
// planning helpers.
package campaign

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db/gen"
	errpkg "github.com/jedafe/japa/internal/errors"
)

func TestValidatePlan_Success(t *testing.T) {
	s := &Service{}
	plan := CampaignPlanResult{
		Items: []CampaignItemPlan{
			{PersonaID: "p1", ContentType: "blog", Title: "A", TopicPrompt: "write a"},
			{PersonaID: "p2", ContentType: "story", Title: "B", TopicPrompt: "write b"},
		},
	}
	pool := []personaSummary{{ID: "p1"}, {ID: "p2"}}
	allowed := []string{"blog", "story"}

	if err := s.validatePlan(plan, 2, pool, allowed); err != nil {
		t.Fatalf("validatePlan failed: %v", err)
	}
}

func TestValidatePlan_WrongCount(t *testing.T) {
	s := &Service{}
	plan := CampaignPlanResult{Items: []CampaignItemPlan{{PersonaID: "p1", ContentType: "blog", TopicPrompt: "x"}}}
	pool := []personaSummary{{ID: "p1"}}
	allowed := []string{"blog"}

	err := s.validatePlan(plan, 2, pool, allowed)
	if err == nil || !errors.Is(err, errpkg.ErrInvalid) {
		t.Fatalf("expected invalid error, got: %v", err)
	}
}

func TestValidatePlan_DuplicatePersona(t *testing.T) {
	s := &Service{}
	plan := CampaignPlanResult{
		Items: []CampaignItemPlan{
			{PersonaID: "p1", ContentType: "blog", TopicPrompt: "a"},
			{PersonaID: "p1", ContentType: "story", TopicPrompt: "b"},
		},
	}
	pool := []personaSummary{{ID: "p1"}}
	allowed := []string{"blog", "story"}

	err := s.validatePlan(plan, 2, pool, allowed)
	if err == nil || !errors.Is(err, errpkg.ErrDuplicate) {
		t.Fatalf("expected duplicate error, got: %v", err)
	}
}

func TestValidatePlan_UnknownPersona(t *testing.T) {
	s := &Service{}
	plan := CampaignPlanResult{
		Items: []CampaignItemPlan{
			{PersonaID: "p99", ContentType: "blog", TopicPrompt: "x"},
		},
	}
	pool := []personaSummary{{ID: "p1"}}
	allowed := []string{"blog"}

	err := s.validatePlan(plan, 1, pool, allowed)
	if err == nil || !errors.Is(err, errpkg.ErrInvalid) {
		t.Fatalf("expected invalid error, got: %v", err)
	}
}

func TestValidatePlan_DisallowedContentType(t *testing.T) {
	s := &Service{}
	plan := CampaignPlanResult{
		Items: []CampaignItemPlan{
			{PersonaID: "p1", ContentType: "review", TopicPrompt: "x"},
		},
	}
	pool := []personaSummary{{ID: "p1"}}
	allowed := []string{"blog"}

	err := s.validatePlan(plan, 1, pool, allowed)
	if err == nil || !errors.Is(err, errpkg.ErrInvalid) {
		t.Fatalf("expected invalid error, got: %v", err)
	}
}

func TestValidateCount(t *testing.T) {
	s := &Service{}
	ctx := t.Context()

	tests := []struct {
		count   int
		wantErr bool
	}{
		{count: 1, wantErr: false},
		{count: 0, wantErr: true},
		{count: 101, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprintf("count=%d", tt.count), func(t *testing.T) {
			err := s.validateCount(ctx, pgtype.UUID{}, tt.count)
			if tt.wantErr && err == nil {
				t.Fatalf("expected error for count %d", tt.count)
			}

			if !tt.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestCampaignFromGen(t *testing.T) {
	var id pgtype.UUID
	_ = id.Scan("550e8400-e29b-41d4-a716-446655440000")

	c := gen.ContentCampaign{
		ID: id,

		Name:                  "Test",
		Prompt:                "prompt",
		Status:                CampaignStatusPlanned,
		RequestedCount:        2,
		AutoProvisionPersonas: true,
		AutoAssign:            true,
	}

	camp := campaignFromGen(c)
	if camp.ID != id.String() {
		t.Fatalf("unexpected ID: %s", camp.ID)
	}

	if camp.RequestedCount != 2 {
		t.Fatalf("unexpected requested count: %d", camp.RequestedCount)
	}
}

func TestCampaignFromGen_ProvisionedPersonas(t *testing.T) {
	var id pgtype.UUID
	_ = id.Scan("550e8400-e29b-41d4-a716-446655440000")

	plan := CampaignPlanResult{
		NewPersonas: []PersonaSpec{{Name: "P1"}, {Name: "P2"}},
		Items:       []CampaignItemPlan{{PersonaID: id.String(), ContentType: "blog"}},
	}
	planJSON, _ := json.Marshal(plan)

	c := gen.ContentCampaign{
		ID: id,

		Name:           "Test",
		Prompt:         "prompt",
		Status:         CampaignStatusPlanned,
		RequestedCount: 1,
		PlanJson:       planJSON,
	}

	camp := campaignFromGen(c)
	if camp.ProvisionedPersonas != 2 {
		t.Fatalf("expected 2 provisioned personas, got %d", camp.ProvisionedPersonas)
	}
}

func TestCountNewPersonas(t *testing.T) {
	if got := countNewPersonas(nil); got != 0 {
		t.Fatalf("expected 0 for nil plan_json, got %d", got)
	}

	if got := countNewPersonas([]byte(`{"error":"boom"}`)); got != 0 {
		t.Fatalf("expected 0 for error plan_json, got %d", got)
	}

	planJSON, _ := json.Marshal(CampaignPlanResult{NewPersonas: []PersonaSpec{{Name: "A"}, {Name: "B"}, {Name: "C"}}})
	if got := countNewPersonas(planJSON); got != 3 {
		t.Fatalf("expected 3 provisioned personas, got %d", got)
	}
}

func TestCampaignNameFromPrompt(t *testing.T) {
	if got := campaignNameFromPrompt("short"); got != "short" {
		t.Fatalf("expected unchanged short prompt, got %q", got)
	}

	longASCII := strings.Repeat("a", 120)
	if got := campaignNameFromPrompt(longASCII); got != strings.Repeat("a", 80) {
		t.Fatalf("expected 80 ascii runes, got %d", len(got))
	}

	longEmoji := strings.Repeat("🎉", 120)
	if got := campaignNameFromPrompt(longEmoji); len([]rune(got)) != 80 {
		t.Fatalf("expected 80 emoji runes, got %d", len([]rune(got)))
	}
}

func TestValidateTargetSpace(t *testing.T) {
	validSpace := pgtype.UUID{Bytes: [16]byte{0x55, 0x0e, 0x84, 0x00, 0xe2, 0x9b, 0x41, 0xd4, 0xa7, 0x16, 0x44, 0x66, 0x55, 0x44, 0x00, 0x00}, Valid: true}
	store := &fakeSpaceStore{exists: map[string]bool{validSpace.String(): true}}
	s := &Service{store: store}

	if err := s.validateTargetSpace(t.Context(), validSpace.String()); err != nil {
		t.Fatalf("expected valid space, got: %v", err)
	}

	if err := s.validateTargetSpace(t.Context(), "not-a-uuid"); err == nil {
		t.Fatalf("expected error for invalid UUID")
	}

	if err := s.validateTargetSpace(t.Context(), "11111111-1111-1111-1111-111111111111"); err == nil {
		t.Fatalf("expected error for missing space")
	}
}

func TestValidatePlanTargetSpaces(t *testing.T) {
	validSpace := pgtype.UUID{Bytes: [16]byte{0x55, 0x0e, 0x84, 0x00, 0xe2, 0x9b, 0x41, 0xd4, 0xa7, 0x16, 0x44, 0x66, 0x55, 0x44, 0x00, 0x00}, Valid: true}
	store := &fakeSpaceStore{exists: map[string]bool{validSpace.String(): true}}
	s := &Service{store: store}

	campaign := gen.ContentCampaign{
		TargetSpaceID: pgtype.UUID{Valid: true, Bytes: validSpace.Bytes},
	}
	items := []CampaignItemPlan{{TargetSpaceID: validSpace.String()}}

	if err := s.validatePlanTargetSpaces(t.Context(), campaign, items); err != nil {
		t.Fatalf("expected valid target spaces, got: %v", err)
	}

	items = []CampaignItemPlan{{TargetSpaceID: "not-a-uuid"}}
	if err := s.validatePlanTargetSpaces(t.Context(), gen.ContentCampaign{}, items); err == nil {
		t.Fatalf("expected error for invalid item target space")
	}

	items = []CampaignItemPlan{{TargetSpaceID: "11111111-1111-1111-1111-111111111111"}}
	if err := s.validatePlanTargetSpaces(t.Context(), gen.ContentCampaign{}, items); err == nil {
		t.Fatalf("expected error for missing item target space")
	}
}

func TestCampaignItemFromGen(t *testing.T) {
	var id pgtype.UUID
	_ = id.Scan("550e8400-e29b-41d4-a716-446655440000")

	i := gen.ContentCampaignItem{
		ID:          id,
		CampaignID:  id,
		PersonaID:   id,
		ContentType: "blog",
		TopicPrompt: "write",
		Status:      ItemStatusPlanned,
	}

	item := campaignItemFromGen(i, map[string]string{id.String(): "Persona A"})
	if item.PersonaName != "Persona A" {
		t.Fatalf("unexpected persona name: %s", item.PersonaName)
	}
}

// fakeSpaceStore is a test double that only implements SpaceExists.
type fakeSpaceStore struct {
	storer
	exists map[string]bool
}

func (f *fakeSpaceStore) SpaceExists(ctx context.Context, id pgtype.UUID) (bool, error) {
	return f.exists[id.String()], nil
}

// fakeReviewSyncStore is a test double that supports the review lifecycle hooks.
type fakeReviewSyncStore struct {
	storer
	item       gen.ContentCampaignItem
	lastStatus string
}

func (f *fakeReviewSyncStore) GetCampaignItemByReviewID(ctx context.Context, reviewID pgtype.UUID) (gen.ContentCampaignItem, error) {
	if f.item.ReviewID == reviewID {
		return f.item, nil
	}

	return gen.ContentCampaignItem{}, pgx.ErrNoRows
}

func (f *fakeReviewSyncStore) UpdateCampaignItem(ctx context.Context, params gen.UpdateContentCampaignItemParams) (gen.ContentCampaignItem, error) {
	f.lastStatus = params.Status
	f.item.Status = params.Status

	return f.item, nil
}

func (f *fakeReviewSyncStore) GetCampaign(ctx context.Context, id pgtype.UUID) (gen.ContentCampaign, error) {
	return gen.ContentCampaign{ID: id, Status: CampaignStatusGenerating}, nil
}

func (f *fakeReviewSyncStore) ListCampaignItemsByCampaign(ctx context.Context, campaignID pgtype.UUID) ([]gen.ContentCampaignItem, error) {
	return []gen.ContentCampaignItem{f.item}, nil
}

func (f *fakeReviewSyncStore) UpdateCampaignStatus(ctx context.Context, id pgtype.UUID, status string, planJSON []byte) (gen.ContentCampaign, error) {
	return gen.ContentCampaign{ID: id, Status: status}, nil
}

func TestOnStudioReviewApproved(t *testing.T) {
	var id, reviewID pgtype.UUID
	_ = id.Scan("550e8400-e29b-41d4-a716-446655440000")
	_ = reviewID.Scan("660e8400-e29b-41d4-a716-446655440000")

	store := &fakeReviewSyncStore{
		item: gen.ContentCampaignItem{
			ID:         id,
			CampaignID: id,

			ReviewID: reviewID,
			Status:   ItemStatusInReview,
		},
	}
	s := &Service{store: store}

	if err := s.OnStudioReviewApproved(t.Context(), id, reviewID, id); err != nil {
		t.Fatalf("OnStudioReviewApproved failed: %v", err)
	}

	if store.lastStatus != ItemStatusPublished {
		t.Fatalf("expected item status %q, got %q", ItemStatusPublished, store.lastStatus)
	}
}

func TestOnStudioReviewRejected(t *testing.T) {
	var id, reviewID pgtype.UUID
	_ = id.Scan("550e8400-e29b-41d4-a716-446655440000")
	_ = reviewID.Scan("660e8400-e29b-41d4-a716-446655440000")

	store := &fakeReviewSyncStore{
		item: gen.ContentCampaignItem{
			ID:         id,
			CampaignID: id,

			ReviewID: reviewID,
			Status:   ItemStatusInReview,
		},
	}
	s := &Service{store: store}

	if err := s.OnStudioReviewRejected(t.Context(), id, reviewID, id); err != nil {
		t.Fatalf("OnStudioReviewRejected failed: %v", err)
	}

	if store.lastStatus != ItemStatusRejected {
		t.Fatalf("expected item status %q, got %q", ItemStatusRejected, store.lastStatus)
	}
}
