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
	calls []string
	err   error
}

func (f *fakeProvisioner) AutoProvision(_ context.Context, userID pgtype.UUID) error {
	f.calls = append(f.calls, userID.String())
	return f.err
}

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
func (f *fakeProvisioner) SyncUserLifecycle(_ context.Context, _ pgtype.UUID) error {
	return nil
}

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
