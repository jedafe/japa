package worker

// Tests for HandleFederationAutoOptin (§126 auto opt-in): the task must
// gate on federation.auto_optin at run time, then provision under each
// feature flag independently. Ineligible members and opted-out rows are
// handled inside AutoProvision; here we lock the orchestration.

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5/pgtype"
)

type fakeFlagChecker struct{ on map[string]bool }

func (f fakeFlagChecker) IsEnabled(_ context.Context, key string) bool { return f.on[key] }

type fakeProvisioner struct {
	calls       []string
	invalidated []string
	err         error
}

func (f *fakeProvisioner) AutoProvision(_ context.Context, userID pgtype.UUID) error {
	f.calls = append(f.calls, userID.String())
	return f.err
}

func (f *fakeProvisioner) InvalidateActorIdentityForUser(_ context.Context, userID pgtype.UUID, _ ...string) {
	f.invalidated = append(f.invalidated, userID.String())
}

// §126 M2 seams — unused by the auto-opt-in handler tests.
func (f *fakeProvisioner) ProcessInbound(_ context.Context, _ pgtype.UUID) error { return nil }
func (f *fakeProvisioner) Fanout(_ context.Context, _ pgtype.UUID) error         { return nil }
func (f *fakeProvisioner) FanoutActorDelete(_ context.Context, _ pgtype.UUID) error {
	return nil
}
func (f *fakeProvisioner) Deliver(_ context.Context, _ pgtype.UUID) error { return nil }
func (f *fakeProvisioner) ResolveRemoteActorTask(_ context.Context, _ string) error {
	return nil
}
func (f *fakeProvisioner) PruneDeliveries(_ context.Context, _ int) error { return nil }

// Remaining atprotoRepoWriter methods — unused by the auto-opt-in handler.
func (f *fakeProvisioner) PublishRecordJSON(_ context.Context, _ pgtype.UUID, _, _ string, _ json.RawMessage, _ pgtype.UUID) (string, error) {
	return "", nil
}
func (f *fakeProvisioner) PublishContent(_ context.Context, _ pgtype.UUID) (string, error) {
	return "", nil
}
func (f *fakeProvisioner) DeleteRecordURI(_ context.Context, _ pgtype.UUID, _ string) error {
	return nil
}
func (f *fakeProvisioner) RequestCrawl(_ context.Context, _ pgtype.UUID) error { return nil }
func (f *fakeProvisioner) ReconcileRepo(_ context.Context, _ pgtype.UUID) error {
	return nil
}

var fakeUserActive = true // lifecycle seam: run-time status the fake reports

func (f *fakeProvisioner) SyncUserLifecycle(_ context.Context, _ pgtype.UUID) (bool, error) {
	return fakeUserActive, nil
}

func (f *fakeProvisioner) MigratePLC(_ context.Context, _ pgtype.UUID) error { return nil }

// setAutoOptinSeams injects fakes into the package-level seams the handler
// reads and restores them on cleanup.
func setAutoOptinSeams(t *testing.T, s settingsReader, c FeatureChecker, fed federationProvisioner, atp atprotoRepoWriter) {
	t.Helper()
	oldS, oldC, oldFed, oldAtp := appSettings, featureChecker, federationSvc, atprotoSvc
	appSettings, featureChecker, federationSvc, atprotoSvc = s, c, fed, atp
	t.Cleanup(func() {
		appSettings, featureChecker, federationSvc, atprotoSvc = oldS, oldC, oldFed, oldAtp
	})
}

func autoOptinTask(t *testing.T, userID pgtype.UUID) *asynq.Task {
	t.Helper()
	task := asynq.NewTask(TypeFederationAutoOptin,
		[]byte(`{"user_id":"`+userID.String()+`"}`))
	return task
}

func TestFederationAutoOptinSettingGate(t *testing.T) {
	var userID pgtype.UUID
	_ = userID.Scan("11111111-1111-1111-1111-111111111111")
	fed, atp := &fakeProvisioner{}, &fakeProvisioner{}

	// Setting off → nothing provisions even with both flags on.
	setAutoOptinSeams(t,
		fakeSettingsReader{bools: map[string]bool{"federation.auto_optin": false}},
		fakeFlagChecker{on: map[string]bool{"feature.federation": true, "feature.atproto": true}},
		fed, atp)
	if err := HandleFederationAutoOptin(context.Background(), autoOptinTask(t, userID)); err != nil {
		t.Fatalf("setting off: %v", err)
	}
	if len(fed.calls) != 0 || len(atp.calls) != 0 {
		t.Fatalf("provisioned while setting off: fed=%v atp=%v", fed.calls, atp.calls)
	}
}

func TestFederationAutoOptinFlagMatrix(t *testing.T) {
	var userID pgtype.UUID
	_ = userID.Scan("22222222-2222-2222-2222-222222222222")
	settings := fakeSettingsReader{bools: map[string]bool{"federation.auto_optin": true}}

	cases := []struct {
		name             string
		fedOn, atpOn     bool
		wantFed, wantAtp int
	}{
		{"both on", true, true, 1, 1},
		{"federation only", true, false, 1, 0},
		{"atproto only", false, true, 0, 1},
		{"both off", false, false, 0, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fed, atp := &fakeProvisioner{}, &fakeProvisioner{}
			setAutoOptinSeams(t, settings,
				fakeFlagChecker{on: map[string]bool{
					"feature.federation": tc.fedOn,
					"feature.atproto":    tc.atpOn,
				}},
				fed, atp)
			if err := HandleFederationAutoOptin(context.Background(), autoOptinTask(t, userID)); err != nil {
				t.Fatalf("handler: %v", err)
			}
			if len(fed.calls) != tc.wantFed || len(atp.calls) != tc.wantAtp {
				t.Fatalf("calls fed=%d atp=%d, want %d/%d",
					len(fed.calls), len(atp.calls), tc.wantFed, tc.wantAtp)
			}
		})
	}
}

func TestFederationBackfillOptinPaging(t *testing.T) {
	mkID := func(s string) pgtype.UUID {
		var id pgtype.UUID
		_ = id.Scan(s)
		return id
	}
	page1 := []pgtype.UUID{
		mkID("00000000-0000-0000-0000-000000000001"),
		mkID("00000000-0000-0000-0000-000000000002"),
	}
	page2 := []pgtype.UUID{mkID("00000000-0000-0000-0000-000000000003")}

	fed, atp := &fakeProvisioner{}, &fakeProvisioner{}
	setAutoOptinSeams(t,
		fakeSettingsReader{bools: map[string]bool{}},
		fakeFlagChecker{on: map[string]bool{"feature.federation": true, "feature.atproto": true}},
		fed, atp)

	// Fake the user pager: two pages — a full one then a partial.
	var pages [][]pgtype.UUID
	oldLister, oldCont := listActiveUserIDs, enqueueBackfillContinuation
	listActiveUserIDs = func(_ context.Context, after pgtype.UUID, limit int) ([]pgtype.UUID, error) {
		if len(pages) == 0 {
			return nil, nil
		}
		p := pages[0]
		pages = pages[1:]
		return p, nil
	}
	var continued string
	enqueueBackfillContinuation = func(cursor string, _ int) error {
		continued = cursor
		// Simulate the follow-on task consuming the next page inline.
		return HandleFederationBackfillOptin(context.Background(),
			asynq.NewTask(TypeFederationBackfillOptin,
				[]byte(`{"cursor":"`+cursor+`","batch":2}`)))
	}
	t.Cleanup(func() { listActiveUserIDs, enqueueBackfillContinuation = oldLister, oldCont })
	pages = [][]pgtype.UUID{page1, page2}

	task := asynq.NewTask(TypeFederationBackfillOptin, []byte(`{"cursor":"","batch":2}`))
	if err := HandleFederationBackfillOptin(context.Background(), task); err != nil {
		t.Fatalf("handler: %v", err)
	}
	if continued != page1[len(page1)-1].String() {
		t.Fatalf("continuation cursor = %q, want %q", continued, page1[len(page1)-1].String())
	}
	if len(fed.calls) != 3 || len(atp.calls) != 3 {
		t.Fatalf("provisioned fed=%d atp=%d, want 3/3", len(fed.calls), len(atp.calls))
	}
}

func TestFederationBackfillOptinBadCursor(t *testing.T) {
	setAutoOptinSeams(t,
		fakeSettingsReader{bools: map[string]bool{}},
		fakeFlagChecker{on: map[string]bool{"feature.federation": true}},
		&fakeProvisioner{}, &fakeProvisioner{})
	err := HandleFederationBackfillOptin(context.Background(),
		asynq.NewTask(TypeFederationBackfillOptin, []byte(`{"cursor":"bogus"}`)))
	if err == nil || !errors.Is(err, asynq.SkipRetry) {
		t.Fatalf("bad cursor: err=%v, want SkipRetry", err)
	}
}

// The lifecycle handler gates the auto-opt-in re-evaluation on the
// member's status AT RUN TIME — transitions to suspended/banned must
// not produce the task, a restore to active must.
func TestLifecycleSyncAutoOptinGate(t *testing.T) {
	fed, atp := &fakeProvisioner{}, &fakeProvisioner{}
	setAutoOptinSeams(t,
		fakeSettingsReader{bools: map[string]bool{"federation.auto_optin": true}},
		fakeFlagChecker{on: map[string]bool{}},
		fed, atp)

	var fired []string
	oldFire, oldActive := fireFederationAutoOptin, fakeUserActive
	fireFederationAutoOptin = func(_ context.Context, id pgtype.UUID) {
		fired = append(fired, id.String())
	}
	t.Cleanup(func() { fireFederationAutoOptin, fakeUserActive = oldFire, oldActive })

	var userID pgtype.UUID
	_ = userID.Scan("33333333-3333-3333-3333-333333333333")
	payload := []byte(`{"user_id":"` + userID.String() + `"}`)

	fakeUserActive = false // suspend/ban transition
	if err := HandleAtprotoLifecycleSync(context.Background(),
		asynq.NewTask(TypeAtprotoLifecycleSync, payload)); err != nil {
		t.Fatalf("handler: %v", err)
	}
	if len(fired) != 0 {
		t.Fatalf("auto_optin fired on non-active transition: %v", fired)
	}
	// The same transition drops cached actor docs — a suspended member's
	// identity stops serving immediately rather than at the 5-min TTL.
	if len(fed.invalidated) != 1 || fed.invalidated[0] != userID.String() {
		t.Fatalf("actor docs not invalidated on non-active transition: %v", fed.invalidated)
	}

	fakeUserActive = true // restore to active
	if err := HandleAtprotoLifecycleSync(context.Background(),
		asynq.NewTask(TypeAtprotoLifecycleSync, payload)); err != nil {
		t.Fatalf("handler: %v", err)
	}
	if len(fired) != 1 || fired[0] != userID.String() {
		t.Fatalf("auto_optin not fired on active transition: %v", fired)
	}
	if len(fed.invalidated) != 1 {
		t.Fatalf("invalidation fired on active transition: %v", fed.invalidated)
	}
}

func TestFederationAutoOptinBadPayload(t *testing.T) {
	setAutoOptinSeams(t,
		fakeSettingsReader{bools: map[string]bool{"federation.auto_optin": true}},
		fakeFlagChecker{on: map[string]bool{}},
		&fakeProvisioner{}, &fakeProvisioner{})

	for _, payload := range [][]byte{
		[]byte(`not-json`),
		[]byte(`{"user_id":"nope"}`),
		[]byte(`{}`),
	} {
		err := HandleFederationAutoOptin(context.Background(),
			asynq.NewTask(TypeFederationAutoOptin, payload))
		if err == nil || !errors.Is(err, asynq.SkipRetry) {
			t.Fatalf("payload %q: err=%v, want SkipRetry", payload, err)
		}
	}
}
