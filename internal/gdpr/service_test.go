package gdpr

import (
	"testing"
)

// ---------------------------------------------------------------------------
// Type tests (pure functions, no DB)
// ---------------------------------------------------------------------------

func TestConsentTypeConstants(t *testing.T) {
	if ConsentMarketingEmail != "marketing_email" {
		t.Errorf("expected marketing_email, got %s", ConsentMarketingEmail)
	}
	if ConsentAnalyticsTracking != "analytics_tracking" {
		t.Errorf("expected analytics_tracking, got %s", ConsentAnalyticsTracking)
	}
	if ConsentThirdPartySharing != "third_party_sharing" {
		t.Errorf("expected third_party_sharing, got %s", ConsentThirdPartySharing)
	}
	if ConsentAIProcessing != "ai_processing" {
		t.Errorf("expected ai_processing, got %s", ConsentAIProcessing)
	}
}

func TestExportStatusConstants(t *testing.T) {
	if ExportPending != "pending" {
		t.Errorf("expected pending, got %s", ExportPending)
	}
	if ExportProcessing != "processing" {
		t.Errorf("expected processing, got %s", ExportProcessing)
	}
	if ExportCompleted != "completed" {
		t.Errorf("expected completed, got %s", ExportCompleted)
	}
	if ExportFailed != "failed" {
		t.Errorf("expected failed, got %s", ExportFailed)
	}
}

func TestDeletionStatusConstants(t *testing.T) {
	if DeletionPending != "pending" {
		t.Errorf("expected pending, got %s", DeletionPending)
	}
	if DeletionProcessing != "processing" {
		t.Errorf("expected processing, got %s", DeletionProcessing)
	}
	if DeletionCompleted != "completed" {
		t.Errorf("expected completed, got %s", DeletionCompleted)
	}
	if DeletionFailed != "failed" {
		t.Errorf("expected failed, got %s", DeletionFailed)
	}
}

func TestAllConsentTypes(t *testing.T) {
	types := AllConsentTypes()
	if len(types) != 4 {
		t.Errorf("expected 4 consent types, got %d", len(types))
	}
}

func TestValidConsentType(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"marketing_email", true},
		{"analytics_tracking", true},
		{"third_party_sharing", true},
		{"ai_processing", true},
		{"invalid_type", false},
		{"", false},
		{"MARKETING_EMAIL", false},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if got := ValidConsentType(tt.input); got != tt.want {
				t.Errorf("ValidConsentType(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestPaginationDefaults(t *testing.T) {
	if DefaultPageSize != 20 {
		t.Errorf("expected DefaultPageSize 20, got %d", DefaultPageSize)
	}
	if MaxPageSize != 100 {
		t.Errorf("expected MaxPageSize 100, got %d", MaxPageSize)
	}
}

func TestNoopEnqueuer(t *testing.T) {
	n := &noopEnqueuer{}
	if err := n.Enqueue("test_task", []byte("{}")); err != nil {
		t.Errorf("noopEnqueuer.Enqueue should return nil, got %v", err)
	}
}
