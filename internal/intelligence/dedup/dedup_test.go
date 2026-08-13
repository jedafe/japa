package dedup

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/db/gen"
)

// mockLister is a minimal RecordLister implementation for unit testing the
// dedup service without a database connection.
type mockLister struct {
	// record is returned from GetIntelRecordByExternalKey when err is nil.
	record gen.IntelRecord
	// err is returned from GetIntelRecordByExternalKey. Set to
	// pgx.ErrNoRows to simulate the "no existing record" case.
	err error
	// calls records the arguments passed to GetIntelRecordByExternalKey
	// so tests can assert the lookup used the expected key pair.
	calls []gen.GetIntelRecordByExternalKeyParams
}

func (m *mockLister) GetIntelRecordByExternalKey(ctx context.Context, arg gen.GetIntelRecordByExternalKeyParams) (gen.IntelRecord, error) {
	m.calls = append(m.calls, arg)
	return m.record, m.err
}

func TestCheck_NewRecord(t *testing.T) {
	mock := &mockLister{err: pgx.ErrNoRows}
	svc := NewService(mock)

	sourceID := pgtype.UUID{Bytes: [16]byte{1, 2, 3}, Valid: true}
	got, err := svc.Check(context.Background(), sourceID, "ext-123")
	if err != nil {
		t.Fatalf("Check returned unexpected error: %v", err)
	}
	if got != CheckResultNew {
		t.Fatalf("got %v, want %v", got, CheckResultNew)
	}
	if len(mock.calls) != 1 {
		t.Fatalf("expected 1 lookup call, got %d", len(mock.calls))
	}
	if mock.calls[0].ExternalKey != "ext-123" {
		t.Errorf("lookup used external_key %q, want %q", mock.calls[0].ExternalKey, "ext-123")
	}
	if !mock.calls[0].SourceID.Valid {
		t.Errorf("lookup source_id was not valid")
	}
}

func TestCheck_UpdateRecord(t *testing.T) {
	mock := &mockLister{
		record: gen.IntelRecord{ExternalKey: "ext-123"},
		err:    nil,
	}
	svc := NewService(mock)

	sourceID := pgtype.UUID{Bytes: [16]byte{1, 2, 3}, Valid: true}
	got, err := svc.Check(context.Background(), sourceID, "ext-123")
	if err != nil {
		t.Fatalf("Check returned unexpected error: %v", err)
	}
	if got != CheckResultUpdate {
		t.Fatalf("got %v, want %v", got, CheckResultUpdate)
	}
}

func TestCheck_StoreError(t *testing.T) {
	wantErr := errors.New("connection refused")
	mock := &mockLister{err: wantErr}
	svc := NewService(mock)

	sourceID := pgtype.UUID{Bytes: [16]byte{1, 2, 3}, Valid: true}
	_, err := svc.Check(context.Background(), sourceID, "ext-123")
	if err == nil {
		t.Fatal("Check expected error, got nil")
	}
	if !errors.Is(err, wantErr) {
		t.Fatalf("got %v, want error wrapping %v", err, wantErr)
	}
}

func TestCheckResult_String(t *testing.T) {
	cases := []struct {
		r    CheckResult
		want string
	}{
		{CheckResultNew, "new"},
		{CheckResultUpdate, "update"},
		{CheckResultConflict, "conflict"},
		{CheckResult(99), "unknown(99)"},
	}
	for _, tc := range cases {
		if got := tc.r.String(); got != tc.want {
			t.Errorf("got %q, want %q", got, tc.want)
		}
	}
}

func TestCheckAcrossSources_Deferred(t *testing.T) {
	// Until a cross-source query exists, CheckAcrossSources must not
	// block the pipeline: it returns CheckResultNew with no error.
	mock := &mockLister{err: pgx.ErrNoRows}
	svc := NewService(mock)

	sourceID := pgtype.UUID{Bytes: [16]byte{1, 2, 3}, Valid: true}
	got, err := svc.CheckAcrossSources(context.Background(), sourceID, "ext-123")
	if err != nil {
		t.Fatalf("CheckAcrossSources returned unexpected error: %v", err)
	}
	if got != CheckResultNew {
		t.Fatalf("got %v, want %v", got, CheckResultNew)
	}
}
