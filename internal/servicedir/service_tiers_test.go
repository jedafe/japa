// service_tiers_test.go — unit coverage for the package pricing-tier pure
// paths that don't need a database: validatePackageTiers (count cap, name
// required, price >= 0, at most one featured, currency pass-through), the
// feature flag gate, and the public JSON contract.
//
// Ref: docs/superpowers/specs/2026-10-07-services-marketplace-upgrade-design.md (§2.4)
package servicedir

import (
	"encoding/json"
	"errors"
	"testing"
)

func int64Ptr(v int64) *int64 { return &v }

func TestValidatePackageTiers(t *testing.T) {
	ctx := t.Context()

	t.Run("empty input is ok", func(t *testing.T) {
		s := &Service{settings: fakeSettings{}}

		params, err := s.validatePackageTiers(ctx, nil)
		if err != nil {
			t.Fatalf("nil: unexpected error %v", err)
		}

		if params != nil {
			t.Fatalf("nil: expected nil params, got %+v", params)
		}

		params, err = s.validatePackageTiers(ctx, []TierInput{})
		if err != nil {
			t.Fatalf("empty slice: unexpected error %v", err)
		}

		if params != nil {
			t.Fatalf("empty slice: expected nil params, got %+v", params)
		}
	})

	t.Run("valid set maps to insert params", func(t *testing.T) {
		s := &Service{settings: fakeSettings{}}

		params, err := s.validatePackageTiers(ctx, []TierInput{
			{
				Name:        "Essential",
				Badge:       "Basic",
				Description: "Starter pack",
				PriceAmount: int64Ptr(1000),
				Features:    []string{"a", "b"},
			},
			{
				Name:          "Premium",
				PriceAmount:   int64Ptr(0), // free tier — 0 is valid
				PriceCurrency: "USD",
				IsFeatured:    boolPtr(true),
				IsActive:      boolPtr(false),
			},
		})
		if err != nil {
			t.Fatalf("unexpected error %v", err)
		}

		if len(params) != 2 {
			t.Fatalf("expected 2 params, got %d", len(params))
		}

		// position = array index; name trimmed; nil features → empty slice.
		if params[0].Position != int32(0) || params[1].Position != int32(1) {
			t.Fatalf("position mismatch: %v, %v", params[0].Position, params[1].Position)
		}

		if params[0].Name != "Essential" || params[1].Name != "Premium" {
			t.Fatalf("name mismatch: %+v", params)
		}

		if params[0].Badge == nil || *params[0].Badge != "Basic" {
			t.Fatalf("badge not carried: %+v", params[0])
		}

		if params[1].IsFeatured != true || params[1].IsActive != false {
			t.Fatalf("featured/active not carried: %+v", params[1])
		}
	})

	t.Run("name is required", func(t *testing.T) {
		s := &Service{settings: fakeSettings{}}
		for _, name := range []string{"", "   "} {
			_, err := s.validatePackageTiers(ctx, []TierInput{
				{Name: name, PriceAmount: int64Ptr(100)},
			})
			if !errors.Is(err, ErrInvalidTiers) {
				t.Fatalf("name %q: expected ErrInvalidTiers, got %v", name, err)
			}
		}
	})

	t.Run("price_amount >= 0 required", func(t *testing.T) {
		s := &Service{settings: fakeSettings{}}

		_, err := s.validatePackageTiers(ctx, []TierInput{{Name: "X"}})
		if !errors.Is(err, ErrInvalidTiers) {
			t.Fatalf("nil price: expected ErrInvalidTiers, got %v", err)
		}

		_, err = s.validatePackageTiers(ctx, []TierInput{
			{Name: "X", PriceAmount: int64Ptr(-1)},
		})
		if !errors.Is(err, ErrInvalidTiers) {
			t.Fatalf("negative price: expected ErrInvalidTiers, got %v", err)
		}
	})

	t.Run("more than max_count rejected", func(t *testing.T) {
		// Default cap is 5 (service_packages.tiers.max_count, migration
		// 000804 seed). A lower override is honored too.
		s := &Service{settings: fakeSettings{"service_packages.tiers.max_count": "2"}}

		tiers := make([]TierInput, 3)
		for i := range tiers {
			tiers[i] = TierInput{Name: "T", PriceAmount: int64Ptr(1)}
		}

		_, err := s.validatePackageTiers(ctx, tiers)
		if !errors.Is(err, ErrInvalidTiers) {
			t.Fatalf("over cap: expected ErrInvalidTiers, got %v", err)
		}

		s6 := &Service{settings: fakeSettings{}}

		tiers6 := make([]TierInput, 6)
		for i := range tiers6 {
			tiers6[i] = TierInput{Name: "T", PriceAmount: int64Ptr(1)}
		}

		_, err = s6.validatePackageTiers(ctx, tiers6)
		if !errors.Is(err, ErrInvalidTiers) {
			t.Fatalf("over default cap: expected ErrInvalidTiers, got %v", err)
		}
	})

	t.Run("at most one featured", func(t *testing.T) {
		s := &Service{settings: fakeSettings{}}

		_, err := s.validatePackageTiers(ctx, []TierInput{
			{Name: "A", PriceAmount: int64Ptr(1), IsFeatured: boolPtr(true)},
			{Name: "B", PriceAmount: int64Ptr(1), IsFeatured: boolPtr(true)},
		})
		if !errors.Is(err, ErrInvalidTiers) {
			t.Fatalf("two featured: expected ErrInvalidTiers, got %v", err)
		}
	})

	t.Run("unknown currency passes through", func(t *testing.T) {
		// price_currency is nullable and not validated against a currency
		// enum — free-form values pass through untouched.
		s := &Service{settings: fakeSettings{}}

		params, err := s.validatePackageTiers(ctx, []TierInput{
			{Name: "A", PriceAmount: int64Ptr(1), PriceCurrency: "XXX"},
			{Name: "B", PriceAmount: int64Ptr(1)},
		})
		if err != nil {
			t.Fatalf("unexpected error %v", err)
		}

		if params[0].PriceCurrency == nil || *params[0].PriceCurrency != "XXX" {
			t.Fatalf("currency not carried: %+v", params[0])
		}

		if params[1].PriceCurrency != nil {
			t.Fatalf("empty currency should stay NULL, got %+v", params[1])
		}
	})
}

func TestTiersEnabled(t *testing.T) {
	ctx := t.Context()

	t.Run("flag on via settings", func(t *testing.T) {
		s := &Service{settings: fakeSettings{"service_packages.tiers_enabled": "true"}}
		if !s.TiersEnabled(ctx) {
			t.Fatal("expected enabled")
		}
	})

	t.Run("flag off via settings", func(t *testing.T) {
		s := &Service{settings: fakeSettings{}}
		if s.TiersEnabled(ctx) {
			t.Fatal("expected disabled")
		}
	})

	t.Run("nil settings is disabled", func(t *testing.T) {
		s := &Service{}
		if s.TiersEnabled(ctx) {
			t.Fatal("expected disabled")
		}
	})

	t.Run("checker wins over settings", func(t *testing.T) {
		s := &Service{
			settings: fakeSettings{"service_packages.tiers_enabled": "false"},
			checker:  fakeChecker{"service_packages.tiers_enabled": true},
		}
		if !s.TiersEnabled(ctx) {
			t.Fatal("checker should take precedence")
		}
	})
}

func TestPackageTierPublicJSONShape(t *testing.T) {
	// The public tiers[] contract is exactly these keys — is_active is an
	// input-only field and must never serialize.
	d := PackageTierDTO{
		ID: "id", Name: "n", Badge: "b", Description: "d",
		PriceAmount: 100, PriceCurrency: "USD", Features: []string{"x"},
		Position: 1, IsFeatured: true,
	}

	var m map[string]any

	raw, err := json.Marshal(d)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	for _, k := range []string{
		"id", "name", "badge", "description",
		"price_amount", "price_currency", "features", "position", "is_featured",
	} {
		if _, ok := m[k]; !ok {
			t.Errorf("missing key %q in public payload", k)
		}
	}

	if _, ok := m["is_active"]; ok {
		t.Error("internal key is_active leaked into public payload")
	}
}
