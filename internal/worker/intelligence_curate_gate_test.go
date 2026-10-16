package worker

import (
	"encoding/json"
	"testing"

	"github.com/hibiken/asynq"
	"github.com/jedafe/japa/internal/db/gen"
)

// TestIntelCurateShouldAutoPublish covers the ADR-088 correction (final
// review I2): the curated auto-publish path requires BOTH a trust level at
// or under intel.curate.auto_publish_trust_max AND an entity type outside
// the high-stakes set. High-stakes curated records (visa, remittance,
// credential_pathway, …) always take dedup → publish_gate → review
// regardless of how trusted the source is.
func TestIntelCurateShouldAutoPublish(t *testing.T) {
	// Not parallel: mutates the appSettings package var.
	ctx := t.Context()

	oldSettings := appSettings

	t.Cleanup(func() { appSettings = oldSettings })

	appSettings = fakeSettingsReader{ints: map[string]int{"intel.curate.auto_publish_trust_max": 2}}

	src := func(trust int16) gen.IntelSource { return gen.IntelSource{TrustLevel: trust} }

	cases := []struct {
		name       string
		trust      int16
		entityType string
		want       bool
	}{
		{"trusted news auto-publishes", 1, "news", true},
		{"trusted event auto-publishes", 1, "event", true},
		{"trusted job auto-publishes", 1, "job", true},
		{"trusted visa goes to review (ADR-088)", 1, "visa", false},
		{"trusted credential_pathway goes to review", 1, "credential_pathway", false},
		{"trusted remittance goes to review", 1, "remittance", false},
		{"trusted banking goes to review", 1, "banking", false},
		{"trusted grants goes to review", 1, "grants", false},
		{"trusted health_insurance goes to review", 1, "health_insurance", false},
		{"trusted admission goes to review", 1, "admission", false},
		{"aggregator news goes to gate (trust too low)", 3, "news", false},
		{"aggregator visa also goes to gate", 3, "visa", false},
	}

	for _, tc := range cases {
		if got := intelCurateShouldAutoPublish(ctx, src(tc.trust), tc.entityType); got != tc.want {
			t.Errorf("%s: intelCurateShouldAutoPublish = %v, want %v", tc.name, got, tc.want)
		}
	}
}

// TestHandleIntelCurateChannel_HighStakesGate exercises the fix end-to-end:
// a trust-1 source's curated `visa` record is persisted as 'extracted' and
// routed to intel:dedup → publish_gate (never auto_published), while the
// same trust level's `news` record still takes the auto-publish →
// intel:promote path. Gated on JC_TEST_DATABASE_URL via wireIntelChannelDeps.
func TestHandleIntelCurateChannel_HighStakesGate(t *testing.T) {
	database, srcStore, intelDocStore, inspector := wireIntelChannelDeps(t)

	oldSettings := appSettings
	appSettings = fakeSettingsReader{ints: map[string]int{"intel.curate.auto_publish_trust_max": 2}}

	t.Cleanup(func() { appSettings = oldSettings })

	// runCase creates a trust-1 source + curate channel + raw doc, stubs the
	// curator to emit one record of entityType, runs the handler, and
	// returns the persisted record's status.
	runCase := func(name, entityType string) string {
		src := mkIntelTestSource(t, srcStore, name, func(a *gen.CreateIntelSourceParams) {
			a.TrustLevel = 1 // official — clears the trust ceiling alone
		})
		ch := mkIntelTestChannel(t, srcStore, src, func(a *gen.CreateIntelSourceChannelParams) {
			a.CrawlMethod = "rss"
			a.Pipeline = "curate"
			a.EntityTypes = []string{entityType}
		})

		rawDoc, err := intelDocStore.CreateIntelRawDocument(t.Context(), gen.CreateIntelRawDocumentParams{
			SourceID:    src.ID,
			ChannelID:   ch.ID,
			Url:         ch.EntryUrl,
			HttpStatus:  200,
			HttpHeaders: []byte(`{}`),
			Body:        []byte("<rss/>" + name),
			ContentHash: "gatehash-" + name,
		})
		if err != nil {
			t.Fatalf("create raw doc %s: %v", name, err)
		}

		setIntelCurator(t, &fakeIntelCurator{recs: []IntelCuratedRecord{{
			RawExtraction:     marshalCuratedExtraction(t, map[string]any{"title": name}),
			ConfidenceOverall: 1.0,
			ExternalKey:       "https://example.com/" + name,
			EntityType:        entityType,
		}}})

		payload, _ := json.Marshal(intelCurateChannelPayload{
			Version:   intelPayloadVersion,
			SourceID:  src.ID.String(),
			ChannelID: ch.ID.String(),
			RawDocID:  rawDoc.ID.String(),
		})
		if err := HandleIntelCurateChannel(t.Context(), asynq.NewTask(TypeIntelCurateChannel, payload)); err != nil {
			t.Fatalf("HandleIntelCurateChannel %s: %v", name, err)
		}

		var status string
		if err := database.Writer.QueryRow(t.Context(),
			`SELECT status FROM intel_records WHERE raw_document_id = $1`, rawDoc.ID).Scan(&status); err != nil {
			t.Fatalf("load record status %s: %v", name, err)
		}

		return status
	}

	// High-stakes: trust-1 + visa → extracted + dedup, NOT auto_published.
	if status := runCase("gate-visa", "visa"); status != "extracted" {
		t.Errorf("visa record status = %q, want extracted (dedup pending) — 'auto_published' would bypass the ADR-088 review gate", status)
	}

	if n := len(pendingTasks(t, inspector, TypeIntelDedup)); n != 1 {
		t.Errorf("visa path dedup tasks = %d, want 1", n)
	}

	if n := len(pendingTasks(t, inspector, TypeIntelPromote)); n != 0 {
		t.Errorf("visa path promote tasks = %d — must not promote without review", n)
	}

	// Non-high-stakes control: trust-1 + news → auto_published + promote.
	if status := runCase("gate-news", "news"); status != "auto_published" {
		t.Errorf("news record status = %q, want auto_published", status)
	}

	if n := len(pendingTasks(t, inspector, TypeIntelPromote)); n != 1 {
		t.Errorf("news path promote tasks = %d, want 1", n)
	}
}
