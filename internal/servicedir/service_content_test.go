// service_content_test.go — unit coverage for the §114 pure paths that
// don't need a database: badge derivation (popular/best_value thresholds)
// and faqs[]/steps[] write validation.
//
// Ref: japaconnect-engineering/sections/114-services-marketplace.md
package servicedir

import (
	"encoding/json"
	"errors"
	"testing"
)

func TestDeriveBadge(t *testing.T) {
	ctx := t.Context()

	t.Run("defaults", func(t *testing.T) {
		s := &Service{settings: fakeSettings{}}

		if got := s.deriveBadge(ctx, 9, 3.0, true); got != "" {
			t.Errorf("9 reviews below min_reviews=10 and low rating: got %q", got)
		}

		if got := s.deriveBadge(ctx, 10, 0, false); got != BadgePopular {
			t.Errorf("10 reviews: got %q, want popular", got)
		}

		// best_value needs both a coupon and the rating floor.
		if got := s.deriveBadge(ctx, 0, 4.5, true); got != BadgeBestValue {
			t.Errorf("coupon + 4.5 rating: got %q, want best_value", got)
		}

		if got := s.deriveBadge(ctx, 0, 4.5, false); got != "" {
			t.Errorf("no coupon: got %q, want empty", got)
		}

		if got := s.deriveBadge(ctx, 0, 3.9, true); got != "" {
			t.Errorf("coupon + 3.9 rating below min_rating=4.0: got %q", got)
		}
	})

	t.Run("popular beats best_value", func(t *testing.T) {
		s := &Service{settings: fakeSettings{}}
		if got := s.deriveBadge(ctx, 50, 4.9, true); got != BadgePopular {
			t.Errorf("got %q, want popular", got)
		}
	})

	t.Run("custom thresholds", func(t *testing.T) {
		s := &Service{settings: fakeSettings{
			"service_marketplace.badges.min_reviews": "3",
			"service_marketplace.badges.min_rating":  "4.8",
		}}

		if got := s.deriveBadge(ctx, 3, 0, false); got != BadgePopular {
			t.Errorf("min_reviews=3 with 3 reviews: got %q", got)
		}

		if got := s.deriveBadge(ctx, 0, 4.5, true); got != "" {
			t.Errorf("min_rating=4.8 with 4.5 rating: got %q", got)
		}
	})

	t.Run("nil settings", func(t *testing.T) {
		s := &Service{}
		if got := s.deriveBadge(ctx, 100, 5.0, true); got != "" {
			t.Errorf("unwired settings: got %q, want empty", got)
		}
	})
}

func TestEncodeListingFaqs(t *testing.T) {
	ctx := t.Context()
	s := &Service{settings: fakeSettings{"service_marketplace.content.max_faqs": "2"}}

	if _, err := s.encodeListingFaqs(ctx, []ServiceFaqInput{
		{Question: "a"}, {Question: "b"}, {Question: "c"},
	}); !errors.Is(err, ErrTooManyFaqs) {
		t.Fatalf("expected ErrTooManyFaqs, got %v", err)
	}

	if _, err := s.encodeListingFaqs(ctx, []ServiceFaqInput{{Question: "  "}}); !errors.Is(err, ErrInvalidFaq) {
		t.Fatalf("expected ErrInvalidFaq for blank question, got %v", err)
	}

	longQ := make([]byte, maxFaqStepTitleLen+1)
	for i := range longQ {
		longQ[i] = 'q'
	}

	if _, err := s.encodeListingFaqs(ctx, []ServiceFaqInput{{Question: string(longQ)}}); !errors.Is(err, ErrInvalidFaq) {
		t.Fatalf("expected ErrInvalidFaq for oversized question, got %v", err)
	}

	b, err := s.encodeListingFaqs(ctx, []ServiceFaqInput{{Question: " Visa? ", Answer: "Yes"}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var out []ServiceFaqDTO
	if err := json.Unmarshal(b, &out); err != nil || len(out) != 1 || out[0].Question != "Visa?" {
		t.Fatalf("expected trimmed question round-trip, got %s (err=%v)", b, err)
	}

	// Empty set encodes '[]' so PATCH can clear the column.
	b, err = s.encodeListingFaqs(ctx, nil)
	if err != nil || string(b) != "[]" {
		t.Fatalf("expected '[]', got %q (err=%v)", b, err)
	}
}

func TestEncodeListingSteps(t *testing.T) {
	ctx := t.Context()
	s := &Service{settings: fakeSettings{"service_marketplace.content.max_steps": "1"}}

	if _, err := s.encodeListingSteps(ctx, []ServiceStepInput{
		{Title: "a"}, {Title: "b"},
	}); !errors.Is(err, ErrTooManySteps) {
		t.Fatalf("expected ErrTooManySteps, got %v", err)
	}

	if _, err := s.encodeListingSteps(ctx, []ServiceStepInput{{Title: ""}}); !errors.Is(err, ErrInvalidStep) {
		t.Fatalf("expected ErrInvalidStep for empty title, got %v", err)
	}

	b, err := s.encodeListingSteps(ctx, []ServiceStepInput{{Title: " Apply ", Description: "Fill the form"}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var out []ServiceStepDTO
	if err := json.Unmarshal(b, &out); err != nil || len(out) != 1 || out[0].Title != "Apply" {
		t.Fatalf("expected trimmed title round-trip, got %s (err=%v)", b, err)
	}
}
