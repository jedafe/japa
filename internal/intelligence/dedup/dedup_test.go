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
	// crossSourceRecords is returned from ListIntelRecordsByExternalKey.
	crossSourceRecords []gen.IntelRecord
	// crossSourceErr is returned from ListIntelRecordsByExternalKey.
	crossSourceErr error
}

func (m *mockLister) GetIntelRecordByExternalKey(_ context.Context, arg gen.GetIntelRecordByExternalKeyParams) (gen.IntelRecord, error) {
	m.calls = append(m.calls, arg)
	return m.record, m.err
}

func (m *mockLister) ListIntelRecordsByExternalKey(_ context.Context, _ string) ([]gen.IntelRecord, error) {
	return m.crossSourceRecords, m.crossSourceErr
}

// testTenantID is the default tenant UUID used for tenant-scoped queries in
// tests. Matches the system default tenant per ADR-112 Amendment 2.
func testTenantID() pgtype.UUID {
	return pgtype.UUID{Bytes: [16]byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1}, Valid: true}
}

func TestCheck_NewRecord(t *testing.T) {
	t.Parallel()

	mock := &mockLister{err: pgx.ErrNoRows}
	svc := NewService(mock, testTenantID())

	sourceID := pgtype.UUID{Bytes: [16]byte{1, 2, 3}, Valid: true}

	got, err := svc.Check(t.Context(), sourceID, "ext-123")
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
	t.Parallel()

	mock := &mockLister{
		record: gen.IntelRecord{ExternalKey: "ext-123"},
		err:    nil,
	}
	svc := NewService(mock, testTenantID())

	sourceID := pgtype.UUID{Bytes: [16]byte{1, 2, 3}, Valid: true}

	got, err := svc.Check(t.Context(), sourceID, "ext-123")
	if err != nil {
		t.Fatalf("Check returned unexpected error: %v", err)
	}

	if got != CheckResultUpdate {
		t.Fatalf("got %v, want %v", got, CheckResultUpdate)
	}
}

func TestCheck_StoreError(t *testing.T) {
	t.Parallel()

	wantErr := errors.New("connection refused")
	mock := &mockLister{err: wantErr}
	svc := NewService(mock, testTenantID())

	sourceID := pgtype.UUID{Bytes: [16]byte{1, 2, 3}, Valid: true}

	_, err := svc.Check(t.Context(), sourceID, "ext-123")
	if err == nil {
		t.Fatal("Check expected error, got nil")
	}

	if !errors.Is(err, wantErr) {
		t.Fatalf("got %v, want error wrapping %v", err, wantErr)
	}
}

func TestCheckResult_String(t *testing.T) {
	t.Parallel()

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

func TestCheckAcrossSources_NoConflict(t *testing.T) {
	t.Parallel()
	// No records from any source — no conflict.
	sourceID := pgtype.UUID{Bytes: [16]byte{1, 2, 3}, Valid: true}
	mock := &mockLister{
		crossSourceRecords: nil,
	}
	svc := NewService(mock, testTenantID())

	got, err := svc.CheckAcrossSources(t.Context(), sourceID, "ext-123")
	if err != nil {
		t.Fatalf("CheckAcrossSources returned unexpected error: %v", err)
	}

	if got != CheckResultNew {
		t.Fatalf("got %v, want %v (no cross-source records)", got, CheckResultNew)
	}
}

func TestCheckAcrossSources_SameSourceNoConflict(t *testing.T) {
	t.Parallel()
	// Records exist but only from the SAME source — not a cross-source conflict.
	sourceID := pgtype.UUID{Bytes: [16]byte{1, 2, 3}, Valid: true}
	mock := &mockLister{
		crossSourceRecords: []gen.IntelRecord{
			{SourceID: sourceID, ExternalKey: "ext-123"},
		},
	}
	svc := NewService(mock, testTenantID())

	got, err := svc.CheckAcrossSources(t.Context(), sourceID, "ext-123")
	if err != nil {
		t.Fatalf("CheckAcrossSources returned unexpected error: %v", err)
	}

	if got != CheckResultNew {
		t.Fatalf("got %v, want %v (same source is not a conflict)", got, CheckResultNew)
	}
}

func TestCheckAcrossSources_DifferentSourceConflict(t *testing.T) {
	t.Parallel()
	// A record exists from a DIFFERENT source with the same external_key —
	// this is a cross-source conflict (BUG-20 fix).
	sourceID := pgtype.UUID{Bytes: [16]byte{1, 2, 3}, Valid: true}
	otherSourceID := pgtype.UUID{Bytes: [16]byte{9, 9, 9}, Valid: true}
	mock := &mockLister{
		crossSourceRecords: []gen.IntelRecord{
			{SourceID: otherSourceID, ExternalKey: "ext-123"},
		},
	}
	svc := NewService(mock, testTenantID())

	got, err := svc.CheckAcrossSources(t.Context(), sourceID, "ext-123")
	if err != nil {
		t.Fatalf("CheckAcrossSources returned unexpected error: %v", err)
	}

	if got != CheckResultConflict {
		t.Fatalf("got %v, want %v (different source = conflict)", got, CheckResultConflict)
	}
}
