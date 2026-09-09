package video

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/db/gen"
	"github.com/jedafe/japa/internal/media"
	"github.com/jedafe/japa/internal/users"
	"github.com/jedafe/japa/internal/worker"
)

type fakeEnqueuer struct {
	recorded []recordedTask
}

type recordedTask struct {
	typeString string
	payload    []byte
}

func (f *fakeEnqueuer) Enqueue(taskType string, payload []byte, opts ...interface{}) error {
	f.recorded = append(f.recorded, recordedTask{taskType, payload})
	return nil
}

type fakeUserManagerForBulk struct{}

func (f *fakeUserManagerForBulk) EnsureSyntheticUser(ctx context.Context, tenantID pgtype.UUID, input users.SyntheticUserInput) (pgtype.UUID, error) {
	return pgtype.UUID{}, nil
}

type fakeMediaManagerForBulk struct{}

func (f *fakeMediaManagerForBulk) StoreFromURL(ctx context.Context, tenantID, uploaderID pgtype.UUID, sourceURL, filename string) (pgtype.UUID, error) {
	return pgtype.UUID{}, nil
}

func (f *fakeMediaManagerForBulk) StoreFromBytes(ctx context.Context, tenantID, uploaderID pgtype.UUID, data []byte, filename string, attribution *media.MediaAttribution) (pgtype.UUID, error) {
	return pgtype.UUID{}, nil
}

func (f *fakeMediaManagerForBulk) GetMedia(ctx context.Context, tenantID, mediaID pgtype.UUID) (gen.Medium, error) {
	return gen.Medium{}, errors.New("not implemented")
}

func (f *fakeMediaManagerForBulk) MaxUploadBytes(ctx context.Context, tenantID pgtype.UUID) int64 {
	return 50 << 20
}

type fakeResolverForBulk struct{}

func (f *fakeResolverForBulk) Resolve(ctx context.Context, rawURL string) (*VideoMetadata, error) {
	return &VideoMetadata{SourceType: SourceYouTube, SourceURL: rawURL}, nil
}
func (f *fakeResolverForBulk) Platform() string { return "" }

type fakeRow struct {
	values []any
	called bool
	err    error
}

func (r *fakeRow) Scan(dest ...any) error {
	if r.err != nil {
		return r.err
	}
	if len(r.values) == 0 {
		return pgx.ErrNoRows
	}
	if len(dest) != len(r.values) {
		return fmt.Errorf("destination count %d does not match value count %d", len(dest), len(r.values))
	}
	for i, v := range r.values {
		reflect.ValueOf(dest[i]).Elem().Set(reflect.ValueOf(v))
	}
	r.called = true
	return nil
}

func mustParseUUID(t *testing.T, s string) pgtype.UUID {
	t.Helper()
	var u pgtype.UUID
	if err := u.Scan(s); err != nil {
		t.Fatalf("parse uuid %q: %v", s, err)
	}
	return u
}

func importJobRowValues(t *testing.T, tenantID, userID, jobID pgtype.UUID) []any {
	sourceURL := "https://www.youtube.com/channel/UCtest"
	status := "pending"
	total := int32(0)
	imported := int32(0)
	failed := int32(0)
	errorLog := []byte("[]")
	createdAt := pgtype.Timestamptz{}
	completedAt := pgtype.Timestamptz{}
	startedAt := pgtype.Timestamptz{}

	return []any{
		jobID,             // id
		tenantID,          // tenant_id
		userID,            // user_id
		"youtube_channel", // source_type
		&sourceURL,        // source_url
		(*string)(nil),    // query
		pgtype.UUID{},     // target_author_user_id
		status,            // status
		total,             // total_items
		imported,          // imported_count
		failed,            // failed_count
		errorLog,          // error_log
		createdAt,         // created_at
		completedAt,       // completed_at
		false,             // auto_publish
		startedAt,         // started_at
	}
}

type fakeDBTXForBulk struct {
	queryRowResults []*fakeRow
}

func (f *fakeDBTXForBulk) Exec(context.Context, string, ...any) (pgconn.CommandTag, error) {
	return pgconn.CommandTag{}, nil
}

func (f *fakeDBTXForBulk) Query(context.Context, string, ...any) (pgx.Rows, error) {
	return &fakeRowsForBulk{}, nil
}

func (f *fakeDBTXForBulk) QueryRow(context.Context, string, ...any) pgx.Row {
	if len(f.queryRowResults) == 0 {
		return &fakeRow{err: pgx.ErrNoRows}
	}
	row := f.queryRowResults[0]
	f.queryRowResults = f.queryRowResults[1:]
	return row
}

func (f *fakeDBTXForBulk) CopyFrom(context.Context, pgx.Identifier, []string, pgx.CopyFromSource) (int64, error) {
	return 0, nil
}

type fakeRowsForBulk struct{}

func (f *fakeRowsForBulk) Close()                                       {}
func (f *fakeRowsForBulk) Err() error                                   { return nil }
func (f *fakeRowsForBulk) CommandTag() pgconn.CommandTag                { return pgconn.CommandTag{} }
func (f *fakeRowsForBulk) FieldDescriptions() []pgconn.FieldDescription { return nil }
func (f *fakeRowsForBulk) Next() bool                                   { return false }
func (f *fakeRowsForBulk) Scan(...any) error                            { return nil }
func (f *fakeRowsForBulk) Values() ([]any, error)                       { return nil, nil }
func (f *fakeRowsForBulk) RawValues() [][]byte                          { return nil }
func (f *fakeRowsForBulk) Conn() *pgx.Conn                              { return nil }

func TestService_StartBulkImport_EnqueuesTask(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	tenantID := mustParseUUID(t, uuid.New().String())
	userID := mustParseUUID(t, uuid.New().String())
	jobID := mustParseUUID(t, uuid.New().String())

	fakeDB := &fakeDBTXForBulk{
		queryRowResults: []*fakeRow{{values: importJobRowValues(t, tenantID, userID, jobID)}},
	}

	store := &Store{read: gen.New(fakeDB), write: gen.New(fakeDB)}
	svc := NewService(store, &fakeUserManagerForBulk{}, &fakeMediaManagerForBulk{}, &fakeResolverForBulk{})

	enqueuer := &fakeEnqueuer{}
	svc.SetEnqueuer(enqueuer)

	job, err := svc.StartBulkImport(ctx, tenantID, userID, BulkImportRequest{
		SourceType:  "channel",
		SourceURL:   "https://www.youtube.com/channel/UCtest",
		AutoPublish: false,
	})
	if err != nil {
		t.Fatalf("StartBulkImport: %v", err)
	}

	if job.ID != jobID {
		t.Errorf("job.ID = %v, want %v", job.ID, jobID)
	}
	if len(enqueuer.recorded) != 1 {
		t.Fatalf("expected 1 enqueued task, got %d", len(enqueuer.recorded))
	}

	task := enqueuer.recorded[0]
	if task.typeString != "video:import_bulk" {
		t.Errorf("task type = %q, want %q", task.typeString, "video:import_bulk")
	}

	var payload worker.BulkImportPayload
	if err := json.Unmarshal(task.payload, &payload); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}

	if payload.ImportJobID != jobID.String() {
		t.Errorf("payload.import_job_id = %q, want %q", payload.ImportJobID, jobID.String())
	}
	if payload.TenantID != tenantID.String() {
		t.Errorf("payload.tenant_id = %q, want %q", payload.TenantID, tenantID.String())
	}
	if payload.UserID != userID.String() {
		t.Errorf("payload.user_id = %q, want %q", payload.UserID, userID.String())
	}
}
