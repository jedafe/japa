package jce

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db/gen"
)

func TestMarshalProfileGoals(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   []ProfileGoal
		want string
	}{
		{name: "nil", in: nil, want: "[]"},
		{name: "empty", in: []ProfileGoal{}, want: "[]"},
		{name: "goals", in: []ProfileGoal{{Key: "low_cost", Weight: 0.25}}, want: `[{"key":"low_cost","weight":0.25}]`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := marshalProfileGoals(tt.in)
			if string(got) != tt.want {
				t.Fatalf("marshalProfileGoals(%v) = %s, want %s", tt.in, got, tt.want)
			}
		})
	}
}

func TestMarshalProfilePreferences(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   map[string]float64
		want string
	}{
		{name: "nil", in: nil, want: "{}"},
		{name: "empty", in: map[string]float64{}, want: "{}"},
		{name: "values", in: map[string]float64{"cost": 0.2}, want: `{"cost":0.2}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := marshalProfilePreferences(tt.in)
			if string(got) != tt.want {
				t.Fatalf("marshalProfilePreferences(%v) = %s, want %s", tt.in, got, tt.want)
			}
		})
	}
}

func TestMarshalProfileDemographics(t *testing.T) {
	t.Parallel()

	age := 30
	d := ProfileDemographics{Age: &age, Occupation: "engineer"}
	b := marshalProfileDemographics(d)

	var got ProfileDemographics
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("unmarshal demographics: %v", err)
	}

	if got.Age == nil || *got.Age != age {
		t.Fatalf("age = %v, want %d", got.Age, age)
	}

	if got.Occupation != d.Occupation {
		t.Fatalf("occupation = %q, want %q", got.Occupation, d.Occupation)
	}
}

func TestProfileFromRow(t *testing.T) {
	t.Parallel()

	uid, _ := uuid.Parse("00000000-0000-0000-0000-000000000001")
	tid, _ := uuid.Parse("00000000-0000-0000-0000-000000000002")
	now := time.Now().UTC()

	row := gen.ComparisonProfile{
		ID:       pgtype.UUID{Bytes: uid, Valid: true},
		TenantID: pgtype.UUID{Bytes: tid, Valid: true},
		UserID:   pgtype.UUID{Bytes: uid, Valid: true},
		Title:    "Test",
		Goals:    []byte(`[{"key":"pr_fast","weight":0.25}]`),
		TargetSpaceIds: []pgtype.UUID{
			{Bytes: tid, Valid: true},
			{Valid: false},
		},
		IsDefault:  true,
		IsTemplate: false,
		CreatedAt:  pgtype.Timestamptz{Time: now, Valid: true},
		UpdatedAt:  pgtype.Timestamptz{Time: now, Valid: true},
	}

	p, err := profileFromRow(row)
	if err != nil {
		t.Fatalf("profileFromRow: %v", err)
	}

	if p.ID != uid.String() {
		t.Fatalf("id = %s, want %s", p.ID, uid.String())
	}

	if p.UserID == nil || *p.UserID != uid.String() {
		t.Fatalf("user_id = %v, want %s", p.UserID, uid.String())
	}

	if len(p.Goals) != 1 || p.Goals[0].Key != "pr_fast" {
		t.Fatalf("goals = %v, want one pr_fast goal", p.Goals)
	}

	if len(p.TargetSpaceIDs) != 1 || p.TargetSpaceIDs[0] != tid.String() {
		t.Fatalf("target space ids = %v, want one valid id", p.TargetSpaceIDs)
	}

	if !p.IsDefault {
		t.Fatal("expected IsDefault true")
	}
}

func TestScoreFactorFromRow(t *testing.T) {
	t.Parallel()

	uid, _ := uuid.Parse("00000000-0000-0000-0000-000000000001")
	cid, _ := uuid.Parse("00000000-0000-0000-0000-000000000003")
	now := time.Now().UTC()

	row := gen.ComparisonScoreFactor{
		ID:               pgtype.UUID{Bytes: uid, Valid: true},
		TenantID:         pgtype.UUID{Bytes: uid, Valid: true},
		ComparisonTypeID: pgtype.UUID{Bytes: cid, Valid: true},
		FactorKey:        "cost",
		Label:            "Cost",
		DataSource:       "resolver_metadata",
		DefaultWeight:    numericFromFloat(0.2),
		Config:           []byte(`{"kind":"number","direction":"lower_is_better"}`),
		Version:          1,
		IsSystem:         false,
		CreatedAt:        pgtype.Timestamptz{Time: now, Valid: true},
		UpdatedAt:        pgtype.Timestamptz{Time: now, Valid: true},
	}

	f, err := scoreFactorFromRow(row)
	if err != nil {
		t.Fatalf("scoreFactorFromRow: %v", err)
	}

	if f.FactorKey != "cost" {
		t.Fatalf("factor key = %q, want cost", f.FactorKey)
	}

	if f.ComparisonTypeID == nil || *f.ComparisonTypeID != cid.String() {
		t.Fatalf("comparison type id = %v, want %s", f.ComparisonTypeID, cid.String())
	}

	if f.DefaultWeight != 0.2 {
		t.Fatalf("default weight = %v, want 0.2", f.DefaultWeight)
	}

	if f.Config.Kind != "number" {
		t.Fatalf("config kind = %q, want number", f.Config.Kind)
	}
}
