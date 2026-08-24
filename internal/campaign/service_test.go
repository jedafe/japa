// Package campaign — service_test.go validates service-level validation and
// planning helpers.
package campaign

import (
	"context"
	"errors"
	"fmt"
	"testing"

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
	ctx := context.Background()

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
		ID:             id,
		TenantID:       id,
		Name:           "Test",
		Prompt:         "prompt",
		Status:         CampaignStatusPlanned,
		RequestedCount: 2,
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
