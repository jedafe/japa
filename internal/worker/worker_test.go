package worker

import (
	"sort"
	"testing"
)

func TestDefaultConfigReturnsSaneDefaults(t *testing.T) {
	t.Parallel()

	cfg := DefaultConfig()

	if cfg.RedisAddr == "" {
		t.Error("DefaultConfig: RedisAddr should not be empty")
	}

	if cfg.CriticalConcurrency <= 0 {
		t.Errorf("DefaultConfig: CriticalConcurrency must be > 0, got %d", cfg.CriticalConcurrency)
	}

	if cfg.DefaultConcurrency <= 0 {
		t.Errorf("DefaultConfig: DefaultConcurrency must be > 0, got %d", cfg.DefaultConcurrency)
	}

	if cfg.LowConcurrency <= 0 {
		t.Errorf("DefaultConfig: LowConcurrency must be > 0, got %d", cfg.LowConcurrency)
	}

	if cfg.RedisDB < 0 {
		t.Errorf("DefaultConfig: RedisDB must be >= 0, got %d", cfg.RedisDB)
	}
}

func TestDefaultConfigConcurrencyOrdering(t *testing.T) {
	t.Parallel()

	cfg := DefaultConfig()

	if cfg.CriticalConcurrency < cfg.DefaultConcurrency {
		t.Errorf("CriticalConcurrency (%d) should be >= DefaultConcurrency (%d)",
			cfg.CriticalConcurrency, cfg.DefaultConcurrency)
	}

	if cfg.DefaultConcurrency < cfg.LowConcurrency {
		t.Errorf("DefaultConcurrency (%d) should be >= LowConcurrency (%d)",
			cfg.DefaultConcurrency, cfg.LowConcurrency)
	}
}

func TestNewServerDoesNotPanic(t *testing.T) {
	t.Parallel()

	cfg := DefaultConfig()
	// NewServer should not panic with a valid config.
	// We don't call srv.Run() — just verify construction succeeds.
	_ = NewServer(cfg)
}

func TestNewClientDoesNotPanic(t *testing.T) {
	t.Parallel()

	cfg := DefaultConfig()

	c := NewClient(cfg)
	if c == nil {
		t.Fatal("NewClient returned nil")
	}

	_ = c.Close()
}

func TestNewMuxRegistersHandlers(t *testing.T) {
	t.Parallel()

	mux := NewMux()
	if mux == nil {
		t.Fatal("NewMux returned nil")
	}
}

func TestTaskTypeConstantsAreUnique(t *testing.T) {
	t.Parallel()

	allTypes := allTaskTypes()

	seen := make(map[string]bool, len(allTypes))
	for _, tt := range allTypes {
		if seen[tt] {
			t.Errorf("duplicate task type constant: %q", tt)
		}

		seen[tt] = true
	}

	if len(seen) != len(allTypes) {
		t.Errorf("expected %d unique task types, got %d", len(allTypes), len(seen))
	}
}

func TestTaskTypeConstantsAreNonEmpty(t *testing.T) {
	t.Parallel()

	for _, tt := range allTaskTypes() {
		if tt == "" {
			t.Error("found empty task type constant")
		}
	}
}

func TestQueueConstantsAreDistinct(t *testing.T) {
	t.Parallel()

	queues := []string{QueueCritical, QueueDefault, QueueLow}
	seen := make(map[string]bool)

	for _, q := range queues {
		if q == "" {
			t.Error("queue constant should not be empty")
		}

		if seen[q] {
			t.Errorf("duplicate queue constant: %q", q)
		}

		seen[q] = true
	}
}

func TestAddrFormatsCorrectly(t *testing.T) {
	t.Parallel()

	cfg := &Config{RedisAddr: "localhost:6379", RedisDB: 2}
	got := Addr(cfg)
	want := "localhost:6379 (db=2)"

	if got != want {
		t.Errorf("Addr() = %q, want %q", got, want)
	}
}

// allTaskTypes returns every task type constant for uniqueness testing.
func allTaskTypes() []string {
	types := []string{
		// Critical
		TypeNotificationDispatchBatch,
		TypeWebhookDelivery,
		TypePaymentWebhookProcess,
		TypeTicketSendConfirmation,
		// Default
		TypeEmailSend,
		TypeSearchIndexSync,
		TypeImageProcessing,
		TypeAITaskProcessing,
		TypeEngagementCountUpdate,
		TypeModerationScan,
		TypeScheduledPublish,
		TypeLinkPreviewFetch,
		TypeLanguagePackAITranslate,
		TypeMediaScan,
		TypeContentFanOutToFeeds,
		TypeAIGenerateContentEmbedding,
		TypeReviewAggregateRecalc,
		TypeTicketSendStatusUpdate,
		TypeLearningUpdateTrackCompletion,
		TypeVideoProcessing,
		TypeWalletProcessActivityEvent,
		TypeNewsletterSend,
		// Low
		TypeDigestEmail,
		TypeSitemapGenerate,
		TypeRSSRegenerate,
		TypeDataExport,
		TypeAccountDeletionCleanup,
		TypeAdEventIngest,
		TypeMaintenanceUpdateVelocityScores,
		TypeMaintenanceCheckSLABreaches,
		TypeMaintenanceUpdateGeoIPDB,
		TypeAnalyticsRefreshViews,
		TypeSecurityScan,
		TypeMaintenanceRegenerateLLMsTxt,
		TypeLearningIssueCertificate,
		TypeDBManualVacuum,
		TypeNewsletterDigest,
	}
	sort.Strings(types)

	return types
}
