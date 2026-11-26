// Package newsletter — newsletter_test.go tests the Newsletter system.
//
// Ref: japaconnect-engineering/sections/62-newsletter-digest.md
package newsletter

import (
	"context"
	"testing"
	"time"
)

func TestGenerateUnsubscribeToken(t *testing.T) {
	token, err := GenerateUnsubscribeToken()
	if err != nil {
		t.Fatalf("GenerateUnsubscribeToken error: %v", err)
	}

	if len(token) != 64 { // 32 bytes = 64 hex chars
		t.Errorf("token length = %d, want 64", len(token))
	}

	// Generate multiple tokens and verify uniqueness.
	seen := map[string]bool{token: true}

	for range 100 {
		tk, err := GenerateUnsubscribeToken()
		if err != nil {
			t.Fatalf("GenerateUnsubscribeToken error: %v", err)
		}

		if len(tk) != 64 {
			t.Errorf("token length = %d, want 64", len(tk))
		}

		if seen[tk] {
			t.Errorf("duplicate token generated: %s", tk)
		}

		seen[tk] = true
	}
}

// Double opt-in must fail closed: with no settings provider wired the
// feature is off, preserving the §62 single opt-in contract (migration
// 001170, audit NL-I1).
func TestDoubleOptInDefaultsOff(t *testing.T) {
	s := &Service{}

	if s.DoubleOptIn(context.Background()) {
		t.Error("DoubleOptIn() = true with no settings provider, want false (fail closed)")
	}
}

// §62 scheduling: a PATCH that supplies scheduled_at on a draft must
// promote the campaign to 'scheduled' — otherwise the row stays draft
// carrying a date the delayed queue never claims (a schedule that
// silently never sent).
func TestPatchEffectiveStatus(t *testing.T) {
	now := time.Now()
	draft := CampaignDraft
	scheduled := CampaignScheduled
	cancelled := CampaignCancelled

	tests := []struct {
		name     string
		existing string
		status   *string
		schedAt  *time.Time
		want     *string
	}{
		{name: "scheduled_at on draft promotes", existing: CampaignDraft, schedAt: &now, want: &scheduled},
		{name: "scheduled_at on scheduled stays", existing: CampaignScheduled, schedAt: &now, want: nil},
		{name: "scheduled_at on sent untouched", existing: CampaignSent, schedAt: &now, want: nil},
		{name: "explicit status wins over time", existing: CampaignDraft, status: &cancelled, schedAt: &now, want: &cancelled},
		{name: "no status no time passthrough", existing: CampaignDraft, want: nil},
		{name: "explicit status passthrough", existing: CampaignScheduled, status: &draft, want: &draft},
		{name: "empty status treated as none", existing: CampaignDraft, status: new(string), schedAt: &now, want: &scheduled},
	}

	for _, tt := range tests {
		got := patchEffectiveStatus(tt.existing, tt.status, tt.schedAt)
		if tt.want == nil {
			if got != tt.status {
				t.Errorf("%s: got %v, want request status unchanged", tt.name, got)
			}
			continue
		}
		if got == nil || *got != *tt.want {
			t.Errorf("%s: got %v, want %q", tt.name, got, *tt.want)
		}
	}
}

// §62 invariant: a 'scheduled' campaign must always carry scheduled_at —
// the delayed-queue claim matches on the timestamp, so a NULL row can
// never fire. PATCH must reject a schedule that leaves the row timeless.
func TestScheduledMissingTime(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name       string
		resulting  string
		patchTime  *time.Time
		existValid bool
		want       bool
	}{
		{name: "scheduled with patch time ok", resulting: CampaignScheduled, patchTime: &now, want: false},
		{name: "scheduled with stored time ok", resulting: CampaignScheduled, existValid: true, want: false},
		{name: "scheduled without any time missing", resulting: CampaignScheduled, want: true},
		{name: "draft without time ok", resulting: CampaignDraft, want: false},
		{name: "cancelled without time ok", resulting: CampaignCancelled, want: false},
		{name: "sent without time ok", resulting: CampaignSent, want: false},
	}

	for _, tt := range tests {
		if got := scheduledMissingTime(tt.resulting, tt.patchTime, tt.existValid); got != tt.want {
			t.Errorf("%s: got %v, want %v", tt.name, got, tt.want)
		}
	}
}
