//go:build integration

package jce

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/auth"
	"github.com/jedafe/japa/internal/comparison"
	"github.com/jedafe/japa/internal/comparison/resolver"
	"github.com/jedafe/japa/internal/config"
	"github.com/jedafe/japa/internal/content"
	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/db/gen"
	"github.com/jedafe/japa/internal/rbac"
	"github.com/jedafe/japa/internal/testdb"
	"github.com/valkey-io/valkey-go"
	"github.com/valkey-io/valkey-go/valkeycompat"
)

type recordedEnqueue struct {
	taskType string
	payload  []byte
	opts     []any
}

type recordingEnqueuer struct {
	calls []recordedEnqueue
}

func (e *recordingEnqueuer) Enqueue(taskType string, payload []byte, opts ...any) error {
	e.calls = append(e.calls, recordedEnqueue{taskType: taskType, payload: payload, opts: opts})
	return nil
}

func (e *recordingEnqueuer) last() recordedEnqueue {
	if len(e.calls) == 0 {
		return recordedEnqueue{}
	}
	return e.calls[len(e.calls)-1]
}

func (e *recordingEnqueuer) taskType() string { return e.last().taskType }
func (e *recordingEnqueuer) payload() []byte  { return e.last().payload }

func startTestValkey(t *testing.T) valkeycompat.Cmdable {
	t.Helper()
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis: %v", err)
	}
	t.Cleanup(mr.Close)

	vk, err := valkey.NewClient(valkey.ClientOption{
		InitAddress:  []string{mr.Addr()},
		DisableCache: true,
	})
	if err != nil {
		t.Fatalf("valkey.NewClient: %v", err)
	}
	t.Cleanup(vk.Close)
	return valkeycompat.NewAdapter(vk)
}

// grantPermission sets up a real permission for the test user by creating a
// role, linking it to the tenant's members group, and adding the user to that
// group. This lets the RBAC resolver authenticate RefreshComparison calls.
func grantPermission(t *testing.T, d *db.DB, tenantID, userID pgtype.UUID, permKey string) {
	t.Helper()
	ctx := t.Context()

	// Ensure the permission key exists globally.
	var permID pgtype.UUID
	if err := d.Writer.QueryRow(ctx, `
		INSERT INTO permissions (id, key, description)
		VALUES (gen_random_uuid(), $1, $2)
		ON CONFLICT (key) DO UPDATE SET key=$1
		RETURNING id
	`, permKey, fmt.Sprintf("test %s", permKey)).Scan(&permID); err != nil {
		t.Fatalf("inserting permission %s: %v", permKey, err)
	}

	// Get the tenant's members group.
	var groupID pgtype.UUID
	if err := d.Reader.QueryRow(ctx, `SELECT id FROM user_groups WHERE tenant_id=$1 AND slug='members'`, tenantID).Scan(&groupID); err != nil {
		t.Fatalf("finding members group: %v", err)
	}

	roleID := pgUUID(t, uuid.New().String())
	if _, err := d.Writer.Exec(ctx, `
		INSERT INTO user_roles (id, tenant_id, name, slug) VALUES ($1, $2, $3, $4)
	`, roleID, tenantID, "test-role", "test-role"); err != nil {
		t.Fatalf("inserting role: %v", err)
	}

	if _, err := d.Writer.Exec(ctx, `
		INSERT INTO role_permissions (role_id, permission_id) VALUES ($1, $2)
	`, roleID, permID); err != nil {
		t.Fatalf("linking role permission: %v", err)
	}

	if _, err := d.Writer.Exec(ctx, `
		INSERT INTO group_roles (group_id, role_id) VALUES ($1, $2)
	`, groupID, roleID); err != nil {
		t.Fatalf("linking group role: %v", err)
	}

	if _, err := d.Writer.Exec(ctx, `
		INSERT INTO user_group_memberships (id, tenant_id, user_id, group_id)
		VALUES (gen_random_uuid(), $1, $2, $3)
		ON CONFLICT (tenant_id, user_id, group_id) DO NOTHING
	`, tenantID, userID, groupID); err != nil {
		t.Fatalf("adding user to members group: %v", err)
	}
}

func authContext(t *testing.T, d *db.DB, ctx context.Context, userID, tenantID pgtype.UUID) context.Context {
	t.Helper()
	ctx = context.WithValue(ctx, auth.UserContextKey, &auth.AuthenticatedUser{
		UserID:   userID,
		TenantID: tenantID,
	})
	r := rbac.New(d, nil)
	return context.WithValue(ctx, rbac.ResolverKey, r)
}

func testIngestionService(t *testing.T, d *db.DB, vk valkeycompat.Cmdable, enq *recordingEnqueuer) *Service {
	t.Helper()

	settings := config.NewSettings(d, vk)
	t.Cleanup(settings.Close)

	comp := comparison.NewService(d)
	comp.SetSettings(settings)
	comp.SetValkey(vk)
	comp.SetContentService(content.NewService(d))

	r := resolver.NewRegistry()
	svc := NewService(d, comp, r, settings, vk)
	svc.SetFeatureChecker(&stubFeatureChecker{enabled: true})
	svc.SetEnqueuer(enq)
	t.Cleanup(svc.Close)

	return svc
}

// TestIngestion_RefreshComparison_Lifecycle covers job creation, lock
// acquisition, payload round-trip, worker-side processing, and final cleanup.
func TestIngestion_RefreshComparison_Lifecycle(t *testing.T) {
	t.Parallel()

	d := testdb.Connect(t)
	vk := startTestValkey(t)
	enq := &recordingEnqueuer{}
	svc := testIngestionService(t, d, vk, enq)
	ctx := t.Context()

	tenantID := testdb.NewTenant(t, d)
	userID := testdb.SeedUser(t, d, tenantID, testdb.UserOpts{})
	spaceID := testdb.SeedSpaceMembership(t, d, tenantID, userID, testdb.SpaceOpts{})
	grantPermission(t, d, tenantID, userID, "jce.refresh.trigger")
	ctx = authContext(t, d, ctx, userID, tenantID)

	res, err := svc.compSvc.CreateJCEComparison(ctx, tenantID, userID, comparison.CreateJCEComparisonInput{
		Title:      "Lifecycle Test",
		Body:       "body",
		Visibility: content.VisibilityPublic,
		Status:     content.StatusPublished,
		SpaceID:    spaceID,
		Depth:      comparison.DepthDetailed,
		Items: []comparison.JCEItemInput{
			{Title: "Item A", Position: 0, ItemRef: []byte(`{"type":"content","id":"00000000-0000-0000-0000-000000000001"}`)},
		},
		Dimensions: []string{"Cost"},
	})
	if err != nil {
		t.Fatalf("CreateJCEComparison: %v", err)
	}

	ds, err := svc.dataSources.create(ctx, tenantID, DataSourceInput{
		SourceName: "Official Registry",
		SourceKind: "official",
		TrustLevel: 5,
		Config:     map[string]any{"value": "$1,234"},
	})
	if err != nil {
		t.Fatalf("create data source: %v", err)
	}

	sourceKind := "official"
	job, err := svc.RefreshComparison(ctx, res.ID, tenantID, userID, &sourceKind)
	if err != nil {
		t.Fatalf("RefreshComparison: %v", err)
	}
	if job.Status != "pending" {
		t.Fatalf("job status = %q, want pending", job.Status)
	}
	if job.SourceKind == nil || *job.SourceKind != sourceKind {
		t.Fatalf("job source_kind mismatch")
	}

	if enq.taskType() != refreshTaskType {
		t.Fatalf("enqueued task type = %q, want %q", enq.taskType(), refreshTaskType)
	}

	var payload refreshValuesPayload
	if err := json.Unmarshal(enq.payload(), &payload); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if payload.TenantID != tenantID.String() {
		t.Fatalf("payload tenant_id = %q, want %q", payload.TenantID, tenantID.String())
	}
	if payload.ContentID != res.ID.String() {
		t.Fatalf("payload content_id mismatch")
	}
	if payload.IngestionJobID != job.ID {
		t.Fatalf("payload ingestion_job_id mismatch")
	}
	if payload.SourceKind == nil || *payload.SourceKind != sourceKind {
		t.Fatalf("payload source_kind mismatch")
	}

	// Verify the distributed lock is held.
	lockKey := refreshLockKey(res.ID)
	if locked, err := vk.Get(ctx, lockKey).Result(); err != nil || locked != "1" {
		t.Fatalf("refresh lock not held: %v / %q", err, locked)
	}

	// Simulate the worker by invoking ProcessRefreshValues directly.
	jobID := pgUUID(t, job.ID)
	if err := svc.ProcessRefreshValues(ctx, tenantID, res.ID, jobID, &sourceKind, userID); err != nil {
		t.Fatalf("ProcessRefreshValues: %v", err)
	}

	completed, err := svc.GetIngestionJob(ctx, jobID, tenantID)
	if err != nil {
		t.Fatalf("GetIngestionJob: %v", err)
	}
	if completed.Status != "completed" {
		t.Fatalf("job status = %q, want completed", completed.Status)
	}
	if completed.RefreshedCount != 1 {
		t.Fatalf("refreshed_count = %d, want 1", completed.RefreshedCount)
	}

	// The value should have been filled with provenance.
	matrix, err := svc.compSvc.GetJCEComparisonMatrix(ctx, res.ID, tenantID)
	if err != nil {
		t.Fatalf("GetJCEComparisonMatrix: %v", err)
	}
	if len(matrix) != 1 || len(matrix[0].Dimensions) != 1 {
		t.Fatalf("unexpected matrix shape")
	}
	cell := matrix[0].Dimensions[0]
	if cell.Value == nil || *cell.Value != "$1,234" {
		got := "<nil>"
		if cell.Value != nil {
			got = *cell.Value
		}
		t.Fatalf("cell value = %q, want %q", got, "$1,234")
	}

	cv, err := gen.New(d.Reader).GetComparisonValue(ctx, gen.GetComparisonValueParams{
		DimensionID: matrix[0].Dimensions[0].DimensionID,
		ItemID:      matrix[0].Item.ID,
		TenantID:    tenantID,
	})
	if err != nil {
		t.Fatalf("GetComparisonValue: %v", err)
	}
	if !cv.DataSourceID.Valid || cv.DataSourceID.String() != ds.ID {
		t.Fatalf("value data_source_id mismatch")
	}
	var md map[string]any
	if err := json.Unmarshal(cv.Metadata, &md); err != nil {
		t.Fatalf("unmarshal metadata: %v", err)
	}
	if md["source"] != "Official Registry" {
		t.Fatalf("metadata source = %q, want Official Registry", md["source"])
	}
	if md["source_kind"] != "official" {
		t.Fatalf("metadata source_kind = %q, want official", md["source_kind"])
	}
	if md["data_source_id"] != ds.ID {
		t.Fatalf("metadata data_source_id mismatch")
	}

	// Lock and freshness cache should be released/deleted on completion.
	_, err = vk.Get(ctx, lockKey).Result()
	if !valkey.IsValkeyNil(err) {
		t.Fatalf("refresh lock was not released")
	}
}

// TestIngestion_LockDeduplication verifies that a second RefreshComparison
// call while a pending job exists returns the same job instead of queueing.
func TestIngestion_LockDeduplication(t *testing.T) {
	t.Parallel()

	d := testdb.Connect(t)
	vk := startTestValkey(t)
	enq := &recordingEnqueuer{}
	svc := testIngestionService(t, d, vk, enq)
	ctx := t.Context()

	tenantID := testdb.NewTenant(t, d)
	userID := testdb.SeedUser(t, d, tenantID, testdb.UserOpts{})
	spaceID := testdb.SeedSpaceMembership(t, d, tenantID, userID, testdb.SpaceOpts{})
	grantPermission(t, d, tenantID, userID, "jce.refresh.trigger")
	ctx = authContext(t, d, ctx, userID, tenantID)

	res, err := svc.compSvc.CreateJCEComparison(ctx, tenantID, userID, comparison.CreateJCEComparisonInput{
		Title:      "Lock Test",
		Body:       "body",
		Visibility: content.VisibilityPublic,
		Status:     content.StatusPublished,
		SpaceID:    spaceID,
		Depth:      comparison.DepthDetailed,
		Items: []comparison.JCEItemInput{
			{Title: "Item A", Position: 0, ItemRef: []byte(`{"type":"content","id":"00000000-0000-0000-0000-000000000001"}`)},
		},
		Dimensions: []string{"Cost"},
	})
	if err != nil {
		t.Fatalf("CreateJCEComparison: %v", err)
	}

	if _, err := svc.dataSources.create(ctx, tenantID, DataSourceInput{
		SourceName: "Official Registry",
		SourceKind: "official",
		TrustLevel: 5,
		Config:     map[string]any{"value": "$1,234"},
	}); err != nil {
		t.Fatalf("create data source: %v", err)
	}

	sourceKind := "official"
	job1, err := svc.RefreshComparison(ctx, res.ID, tenantID, userID, &sourceKind)
	if err != nil {
		t.Fatalf("RefreshComparison first: %v", err)
	}

	// Second call while the lock is held should return the same pending job.
	job2, err := svc.RefreshComparison(ctx, res.ID, tenantID, userID, &sourceKind)
	if err != nil {
		t.Fatalf("RefreshComparison second: %v", err)
	}
	if job2.ID != job1.ID {
		t.Fatalf("dedup failed: job1=%s job2=%s", job1.ID, job2.ID)
	}

	// Only one task should have been enqueued.
	if enq.taskType() == "" {
		t.Fatalf("no task enqueued")
	}
	enqFirst := string(enq.payload())
	enq.calls = nil

	// A third call without the lock still returns the same pending job.
	job3, err := svc.RefreshComparison(ctx, res.ID, tenantID, userID, &sourceKind)
	if err != nil {
		t.Fatalf("RefreshComparison third: %v", err)
	}
	if job3.ID != job1.ID {
		t.Fatalf("third call returned different job")
	}
	if enq.taskType() != "" {
		t.Fatalf("third call enqueued a second task")
	}

	_ = enqFirst
}

// TestIngestion_ListAndGet covers the ingestion job store list/get paths.
func TestIngestion_ListAndGet(t *testing.T) {
	t.Parallel()

	d := testdb.Connect(t)
	vk := startTestValkey(t)
	enq := &recordingEnqueuer{}
	svc := testIngestionService(t, d, vk, enq)
	ctx := t.Context()

	tenantID := testdb.NewTenant(t, d)
	userID := testdb.SeedUser(t, d, tenantID, testdb.UserOpts{})
	spaceID := testdb.SeedSpaceMembership(t, d, tenantID, userID, testdb.SpaceOpts{})
	ctx = authContext(t, d, ctx, userID, tenantID)

	res, err := svc.compSvc.CreateJCEComparison(ctx, tenantID, userID, comparison.CreateJCEComparisonInput{
		Title:      "List Test",
		Body:       "body",
		Visibility: content.VisibilityPublic,
		Status:     content.StatusPublished,
		SpaceID:    spaceID,
		Depth:      comparison.DepthQuick,
		Items:      []comparison.JCEItemInput{{Title: "Item A", Position: 0, ItemRef: []byte("{}")}},
		Dimensions: []string{"Cost"},
	})
	if err != nil {
		t.Fatalf("CreateJCEComparison: %v", err)
	}

	// Manually create a couple of jobs to test list/get without enqueueing.
	job1, err := svc.ingestion.create(ctx, tenantID, res.ID, nil)
	if err != nil {
		t.Fatalf("create job1: %v", err)
	}
	time.Sleep(10 * time.Millisecond)
	job2, err := svc.ingestion.create(ctx, tenantID, res.ID, ptrTo("official"))
	if err != nil {
		t.Fatalf("create job2: %v", err)
	}

	got, err := svc.GetIngestionJob(ctx, pgUUID(t, job2.ID), tenantID)
	if err != nil {
		t.Fatalf("GetIngestionJob: %v", err)
	}
	if got.ID != job2.ID {
		t.Fatalf("GetIngestionJob returned wrong job")
	}

	jobs, err := svc.ListIngestionJobs(ctx, res.ID, tenantID, 10, 0)
	if err != nil {
		t.Fatalf("ListIngestionJobs: %v", err)
	}
	if len(jobs) != 2 {
		t.Fatalf("len(jobs) = %d, want 2", len(jobs))
	}
	// Most recent first.
	if jobs[0].ID != job2.ID {
		t.Fatalf("jobs not sorted by created_at desc")
	}

	// Tenant isolation.
	otherTenant := testdb.NewTenant(t, d)
	if _, err := svc.GetIngestionJob(ctx, pgUUID(t, job1.ID), otherTenant); err == nil {
		t.Fatalf("expected tenant isolation error")
	}
}

// TestIngestion_ScheduleStaleRefresh enqueues a refresh for a comparison whose
// smart_mode is not 'none' and that has no recent completed job.
func TestIngestion_ScheduleStaleRefresh(t *testing.T) {
	t.Parallel()

	d := testdb.Connect(t)
	vk := startTestValkey(t)
	enq := &recordingEnqueuer{}
	svc := testIngestionService(t, d, vk, enq)
	ctx := t.Context()

	tenantID := testdb.NewTenant(t, d)
	userID := testdb.SeedUser(t, d, tenantID, testdb.UserOpts{})
	spaceID := testdb.SeedSpaceMembership(t, d, tenantID, userID, testdb.SpaceOpts{})
	grantPermission(t, d, tenantID, userID, "jce.refresh.trigger")
	ctx = authContext(t, d, ctx, userID, tenantID)

	res, err := svc.compSvc.CreateJCEComparison(ctx, tenantID, userID, comparison.CreateJCEComparisonInput{
		Title:      "Stale Sweep Test",
		Body:       "body",
		Visibility: content.VisibilityPublic,
		Status:     content.StatusPublished,
		SpaceID:    spaceID,
		Depth:      comparison.DepthDetailed,
		Items:      []comparison.JCEItemInput{{Title: "Item A", Position: 0, ItemRef: []byte("{}")}},
		Dimensions: []string{"Cost"},
	})
	if err != nil {
		t.Fatalf("CreateJCEComparison: %v", err)
	}

	// Only smart_mode != 'none' is eligible for the stale sweep.
	if _, err := d.Writer.Exec(ctx, `UPDATE comparison_content_metadata SET smart_mode='cost' WHERE content_id=$1`, res.ID); err != nil {
		t.Fatalf("setting smart_mode: %v", err)
	}

	if _, err := svc.dataSources.create(ctx, tenantID, DataSourceInput{
		SourceName: "Official Registry",
		SourceKind: "official",
		TrustLevel: 5,
		Config:     map[string]any{"value": "$1,234"},
	}); err != nil {
		t.Fatalf("create data source: %v", err)
	}

	// Mark any leftover pending/running jobs as completed so the stale sweep
	// does not exclude the comparison we just created due to in-progress state.
	if _, err := d.Writer.Exec(ctx, `
		UPDATE comparison_data_ingestion_jobs
		SET status='completed', completed_at=now() - interval '1 day'
		WHERE status IN ('pending','running')
	`); err != nil {
		t.Fatalf("clearing leftover pending jobs: %v", err)
	}

	n, err := svc.ScheduleStaleRefresh(ctx, userID)
	if err != nil {
		t.Fatalf("ScheduleStaleRefresh: %v", err)
	}
	if n < 1 {
		t.Fatalf("enqueued = %d, want at least 1", n)
	}

	// Find the call for the comparison we created.
	var found bool
	for _, c := range enq.calls {
		if c.taskType != refreshTaskType {
			t.Fatalf("task type = %q, want %q", c.taskType, refreshTaskType)
		}
		var payload refreshValuesPayload
		if err := json.Unmarshal(c.payload, &payload); err != nil {
			t.Fatalf("unmarshal payload: %v", err)
		}
		if payload.ContentID == res.ID.String() {
			found = true
			if payload.TenantID != tenantID.String() {
				t.Fatalf("payload tenant_id mismatch")
			}
		}
	}
	if !found {
		t.Fatalf("sweep did not enqueue a task for created comparison")
	}

	// A second sweep should not re-enqueue because the pending job exists.
	enq.calls = nil
	n2, err := svc.ScheduleStaleRefresh(ctx, userID)
	if err != nil {
		t.Fatalf("ScheduleStaleRefresh second: %v", err)
	}
	if n2 != 0 {
		t.Fatalf("second sweep enqueued = %d, want 0", n2)
	}
}
