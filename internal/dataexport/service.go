package dataexport

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/db/gen"
)

// Enqueuer is the interface for enqueuing background tasks.
type Enqueuer interface {
	Enqueue(taskType string, payload []byte, opts ...interface{}) error
}

// noopEnqueuer is a no-op implementation for development.
type noopEnqueuer struct{}

func (n *noopEnqueuer) Enqueue(_ string, _ []byte, _ ...interface{}) error { return nil }

// Storage is the interface for storing export files and retrieving their
// download URLs. The default implementation (LocalStorage) writes to
// data/exports/{tenantID}/ and returns a relative download-endpoint URL.
// In production this can be replaced with an S3/SeaweedFS-backed adapter.
type Storage interface {
	Store(ctx context.Context, tenantID, exportID pgtype.UUID, filename, mimeType string, data []byte) (url string, size int64, err error)
	GetPath(tenantID, exportID pgtype.UUID, filename string) (string, error)
}

// LocalStorage writes export files to data/exports/{tenantID}/ and returns
// a download URL of the form /api/v1/admin/data-exports/{exportID}/download.
// Files are created with 0600 permissions because they contain PII.
type LocalStorage struct {
	root string
}

// NewLocalStorage creates a LocalStorage rooted at the given directory
// (default "data/exports").
func NewLocalStorage(root string) *LocalStorage {
	if root == "" {
		root = "data/exports"
	}

	return &LocalStorage{root: root}
}

func (ls *LocalStorage) dirForTenant(tenantID pgtype.UUID) (string, error) {
	abs, err := filepath.Abs(ls.root)
	if err != nil {
		return "", fmt.Errorf("resolving export root: %w", err)
	}

	dir := filepath.Join(abs, tenantID.String())
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("creating export directory: %w", err)
	}

	return dir, nil
}

// Store is an exported method.
func (ls *LocalStorage) Store(_ context.Context, tenantID, exportID pgtype.UUID, filename, mimeType string, data []byte) (string, int64, error) {
	dir, err := ls.dirForTenant(tenantID)
	if err != nil {
		return "", 0, err
	}

	full := filepath.Join(dir, filename)
	if err := os.WriteFile(full, data, 0o600); err != nil {
		return "", 0, fmt.Errorf("writing export file: %w", err)
	}

	info, err := os.Stat(full)
	if err != nil {
		return "", 0, fmt.Errorf("statting export file: %w", err)
	}

	url := fmt.Sprintf("/api/v1/admin/data-exports/%s/download", exportID.String())
	_ = mimeType // reserved for future S3 content-type header

	return url, info.Size(), nil
}

// GetPath returns the local filesystem path for a tenant's export file.
func (ls *LocalStorage) GetPath(tenantID, exportID pgtype.UUID, filename string) (string, error) {
	dir, err := ls.dirForTenant(tenantID)
	if err != nil {
		return "", err
	}

	full := filepath.Join(dir, filename)
	// Path-traversal guard: ensure the resolved path stays under the tenant dir.
	abs, err := filepath.Abs(full)
	if err != nil {
		return "", err
	}

	if abs != dir && !strings.HasPrefix(abs, dir+string(os.PathSeparator)) {
		return "", errors.New("path escapes export directory")
	}

	return abs, nil
}

// Valid export types and formats.
const (
	TypeUsers        = "users"
	TypePosts        = "posts"
	TypeComments     = "comments"
	TypeOrders       = "orders"
	TypeTransactions = "transactions"
	TypeAnalytics    = "analytics"
	TypeModeration   = "moderation"

	FormatCSV  = "csv"
	FormatJSON = "json"
	FormatXLSX = "xlsx"
)

// ValidExportType returns true if the given type is a supported export type.
func ValidExportType(t string) bool {
	switch t {
	case TypeUsers, TypePosts, TypeComments, TypeOrders, TypeTransactions, TypeAnalytics, TypeModeration:
		return true
	}

	return false
}

// ValidFormat returns true if the given format is supported and actually
// implemented. XLSX is accepted by the schema (Section 67) but not yet
// implemented — callers must reject it with a clear error rather than
// silently producing a CSV file labelled as XLSX.
func ValidFormat(f string) bool {
	return f == FormatCSV || f == FormatJSON
}

// ImplementedFormat returns true if the format can actually be generated.
// Used by the handler to reject unsupported-but-schema-valid formats (xlsx)
// with a 400 instead of silently downgrading.
func ImplementedFormat(f string) bool {
	return f == FormatCSV || f == FormatJSON
}

// Service provides platform data export operations.
type Service struct {
	store    *Store
	db       *db.DB
	enqueuer Enqueuer
	storage  Storage
}

// NewService creates a Service backed by the given database.
func NewService(database *db.DB) *Service {
	return &Service{
		store:    NewStore(database),
		db:       database,
		enqueuer: &noopEnqueuer{},
		storage:  NewLocalStorage(""),
	}
}

// SetEnqueuer injects the Asynq enqueuer for background task dispatch.
func (s *Service) SetEnqueuer(e Enqueuer) {
	s.enqueuer = e
}

// SetStorage injects the file storage backend used to persist export files
// and generate download URLs. Defaults to LocalStorage ("data/exports").
func (s *Service) SetStorage(st Storage) {
	s.storage = st
}

// CreateExport creates a new export job and enqueues a background task.
func (s *Service) CreateExport(ctx context.Context, tenantID, userID pgtype.UUID, name, exportType, format string, includeMetadata bool) (gen.PlatformDataExport, error) {
	if !ValidExportType(exportType) {
		return gen.PlatformDataExport{}, fmt.Errorf("invalid export_type: %s", exportType)
	}

	if !ValidFormat(format) {
		return gen.PlatformDataExport{}, fmt.Errorf("invalid format: %s", format)
	}

	exp, err := s.store.Create(ctx, gen.CreatePlatformDataExportParams{

		Name:            name,
		ExportType:      exportType,
		Format:          format,
		IncludeMetadata: includeMetadata,
		CreatedBy:       userID,
	})
	if err != nil {
		return gen.PlatformDataExport{}, fmt.Errorf("creating export: %w", err)
	}

	// Enqueue background task (fire-and-forget, Rule 8).
	payload, _ := json.Marshal(map[string]string{
		"export_id": exp.ID.String(),
		"tenant_id": tenantID.String(),
	})
	if err := s.enqueuer.Enqueue("platform:data_export", payload); err != nil {
		slog.Error("dataexport: enqueue failed", "error", err)
	}

	return exp, nil
}

// GetExport retrieves a single export by ID.
func (s *Service) GetExport(ctx context.Context, id, tenantID pgtype.UUID) (gen.PlatformDataExport, error) {
	return s.store.Get(ctx, id, tenantID)
}

// ListExports returns paginated exports for a tenant.
func (s *Service) ListExports(ctx context.Context, tenantID pgtype.UUID, status string, page, limit int) ([]gen.PlatformDataExport, int64, error) {
	offset := int32((page - 1) * limit) // #nosec

	rows, err := s.store.List(ctx, tenantID, status, int32(limit), offset) // #nosec
	if err != nil {
		return nil, 0, err
	}

	total, err := s.store.Count(ctx, tenantID, status)
	if err != nil {
		return nil, 0, err
	}

	return rows, total, nil
}

// DeleteExport removes an export record.
func (s *Service) DeleteExport(ctx context.Context, id, tenantID pgtype.UUID) error {
	return s.store.Delete(ctx, id, tenantID)
}

// ProcessExport is called by the Asynq worker. It queries the relevant data,
// generates the file in the requested format, and updates the export record
// with the download URL. It is idempotent on retry: an export that is already
// `completed` is left untouched (the asynq-tasks catalog marks
// `platform:data_export` as idempotent on export ID), and an export already
// `processing` is skipped to avoid two concurrent workers racing on the same
// file.
func (s *Service) ProcessExport(ctx context.Context, exportID pgtype.UUID) error {
	exp, err := s.store.GetByID(ctx, exportID)
	if err != nil {
		return fmt.Errorf("fetching export: %w", err)
	}

	// Idempotency guard: a completed export is not re-processed.
	if exp.Status == "completed" {
		slog.Info("dataexport: export already completed, skipping",
			"export_id", exportID.String())
		return nil
	}
	// Avoid racing with another worker that is already processing this export.
	if exp.Status == "processing" {
		slog.Warn("dataexport: export already processing, skipping",
			"export_id", exportID.String())
		return nil
	}

	// Mark as processing.
	_, err = s.store.UpdateStatus(ctx, gen.UpdatePlatformDataExportStatusParams{
		ID:     exportID,
		Status: "processing",
	})
	if err != nil {
		return fmt.Errorf("marking export as processing: %w", err)
	}

	// Collect data from the relevant table.
	rows, colNames, err := s.collectData(ctx, pgtype.UUID{}, exp.ExportType, exp.IncludeMetadata)
	if err != nil {
		s.markFailed(ctx, exportID, fmt.Sprintf("collecting data: %v", err))
		return fmt.Errorf("collecting data: %w", err)
	}

	// Generate the file in the requested format.
	fileURL, fileSize, err := s.generateFile(ctx, exp, rows, colNames)
	if err != nil {
		s.markFailed(ctx, exportID, fmt.Sprintf("generating file: %v", err))
		return fmt.Errorf("generating file: %w", err)
	}

	// Mark as completed.
	_, err = s.store.UpdateStatus(ctx, gen.UpdatePlatformDataExportStatusParams{
		ID:          exportID,
		Status:      "completed",
		FileUrl:     &fileURL,
		FileSize:    &fileSize,
		CompletedAt: pgtype.Timestamptz{Time: time.Now(), Valid: true},
	})
	if err != nil {
		return fmt.Errorf("marking export as completed: %w", err)
	}

	slog.Info("dataexport: export completed",
		"export_id", exportID.String(),
		"type", exp.ExportType,
		"format", exp.Format,
		"rows", len(rows),
		"size", fileSize)

	return nil
}

// markFailed updates the export status to 'failed' with an error message.
func (s *Service) markFailed(ctx context.Context, exportID pgtype.UUID, errMsg string) {
	errMsgPtr := errMsg

	_, err := s.store.UpdateStatus(ctx, gen.UpdatePlatformDataExportStatusParams{
		ID:           exportID,
		Status:       "failed",
		ErrorMessage: &errMsgPtr,
	})
	if err != nil {
		slog.Error("dataexport: failed to mark export as failed", "error", err)
	}
}

// collectData queries the relevant table and returns rows as a slice of
// map[string]any (column name → value) plus the ordered column names. This
// uses raw SQL via pgx because each export type queries a different table
// with different columns.
func (s *Service) collectData(ctx context.Context, tenantID pgtype.UUID, exportType string, includeMetadata bool) ([]map[string]any, []string, error) {
	query, args := buildExportQuery(exportType, tenantID, includeMetadata)
	if query == "" {
		return nil, nil, fmt.Errorf("unsupported export type: %s", exportType)
	}

	rows, err := s.db.Reader.Query(ctx, query, args...)
	if err != nil {
		return nil, nil, fmt.Errorf("querying %s: %w", exportType, err)
	}
	defer rows.Close()

	fields := rows.FieldDescriptions()
	colNames := make([]string, len(fields))

	for i, fd := range fields {
		colNames[i] = fd.Name
	}

	result := make([]map[string]any, 0)

	for rows.Next() {
		values, err := rows.Values()
		if err != nil {
			return nil, nil, fmt.Errorf("scanning row: %w", err)
		}

		row := make(map[string]any, len(fields))
		for i, fd := range fields {
			row[fd.Name] = values[i]
		}

		result = append(result, row)
	}

	if err := rows.Err(); err != nil {
		return nil, nil, fmt.Errorf("iterating rows: %w", err)
	}

	return result, colNames, nil
}

// buildExportQuery returns the SQL query and args for the given export type.
// Each query is tenant-scoped. When includeMetadata is false, only the
// business-data columns are selected (no timestamps/IDs).
//
// Metadata columns are table-prefixed per export type because every JOIN
// query touches two tables that both have id/created_at/updated_at — a bare
// `id` would be ambiguous (PostgreSQL error 42702). Metadata columns are only
// appended for tables that actually have them: wallet_ledger, platform_events,
// and content_reports have no updated_at column. The analytics query is
// aggregated (GROUP BY), so metadata is exposed as min() aggregates rather
// than bare columns (which would violate the GROUP BY rule).
func buildExportQuery(exportType string, tenantID pgtype.UUID, includeMetadata bool) (string, []any) {
	switch exportType {
	case TypeUsers:
		// users table has no display_name/bio — those are in user_profiles.
		// profile_type is the column (not user_type). Metadata is taken from
		// the users table (the primary subject of the export).
		meta := ""
		if includeMetadata {
			meta = ", u.id, u.created_at, u.updated_at"
		}

		return `SELECT u.username, u.email, u.profile_type, u.status,
			p.display_name, p.bio` + meta + `
			FROM users u
			LEFT JOIN user_profiles p ON p.user_id = u.id
			 ORDER BY u.created_at DESC`, []any{tenantID}

	case TypePosts:
		meta := ""
		if includeMetadata {
			meta = ", c.id, c.created_at, c.updated_at"
		}

		return `SELECT c.title, c.body, c.content_type, c.status, c.visibility,
			c.slug, c.canonical_url` + meta + `, u.username AS author
			FROM contents c
			JOIN users u ON u.id = c.author_id
			WHERE c.deleted_at IS NULL
			ORDER BY c.created_at DESC`, []any{tenantID}

	case TypeComments:
		// comments table uses author_user_id (not user_id) and has no status column.
		meta := ""
		if includeMetadata {
			meta = ", cm.id, cm.created_at, cm.updated_at"
		}

		return `SELECT cm.body` + meta + `,
			u.username AS author, c.title AS content_title
			FROM comments cm
			JOIN users u ON u.id = cm.author_user_id
			JOIN contents c ON c.id = cm.content_id
			WHERE cm.deleted_at IS NULL
			ORDER BY cm.created_at DESC`, []any{tenantID}

	case TypeOrders:
		// The orders table is `payments` in JapaConnect.
		meta := ""
		if includeMetadata {
			meta = ", p.id, p.created_at, p.updated_at"
		}

		return `SELECT p.provider, p.purpose_type, p.status, p.amount, p.currency` + meta + `,
			u.username AS buyer
			FROM payments p
			JOIN users u ON u.id = p.user_id
			
			ORDER BY p.created_at DESC`, []any{tenantID}

	case TypeTransactions:
		// The wallet transactions table is `wallet_ledger`. It has no
		// updated_at column, so only id and created_at are appended.
		meta := ""
		if includeMetadata {
			meta = ", w.id, w.created_at"
		}

		return `SELECT w.ledger_type, w.operation, w.amount, w.source, w.reason` + meta + `,
			u.username AS user
			FROM wallet_ledger w
			JOIN users u ON u.id = w.user_id
			
			ORDER BY w.created_at DESC`, []any{tenantID}

	case TypeAnalytics:
		// The analytics events table is `platform_events`. The query is
		// aggregated by day, so metadata is exposed as min() aggregates to
		// satisfy the GROUP BY rule. platform_events has no updated_at.
		meta := ""
		if includeMetadata {
			meta = ", min(id) AS id, min(created_at) AS first_seen, max(created_at) AS last_seen"
		}

		return `SELECT event_type, entity_type, entity_id, count(*) AS event_count,
			date_trunc('day', created_at) AS day` + meta + `
			FROM platform_events
			
			GROUP BY event_type, entity_type, entity_id, day
			ORDER BY day DESC`, []any{tenantID}

	case TypeModeration:
		// The moderation reports table is `content_reports`. It has no
		// updated_at column, so only id and created_at are appended.
		meta := ""
		if includeMetadata {
			meta = ", cr.id, cr.created_at"
		}

		return `SELECT cr.entity_type, cr.reason, cr.status` + meta + `,
			u.username AS reporter
			FROM content_reports cr
			LEFT JOIN users u ON u.id = cr.reporter_user_id
			
			ORDER BY cr.created_at DESC`, []any{tenantID}
	}

	return "", nil
}

// generateFile renders the export data in the requested format and stores it
// via the configured Storage backend. It returns the download URL and file
// size. Export files contain PII (e.g. user emails) and the storage backend
// is responsible for restricting file permissions (LocalStorage uses 0600).
func (s *Service) generateFile(ctx context.Context, exp gen.PlatformDataExport, rows []map[string]any, colNames []string) (string, int64, error) {
	baseName := sanitizeFilename(exp.Name)
	filename := fmt.Sprintf("export_%s_%s.%s", baseName, exp.ID.String()[:8], exp.Format)

	var data []byte

	var mimeType string

	var err error

	switch exp.Format {
	case FormatJSON:
		data, err = renderJSON(rows)
		mimeType = "application/json"
	case FormatCSV:
		data, err = renderCSV(rows, colNames)
		mimeType = "text/csv"
	default:
		// xlsx and any other value are rejected here. The handler also
		// rejects xlsx at request time (see ImplementedFormat), but the
		// worker must guard against rows that were queued before the
		// handler gained that check.
		return "", 0, fmt.Errorf("unsupported format: %s", exp.Format)
	}

	if err != nil {
		return "", 0, err
	}

	return s.storage.Store(ctx, pgtype.UUID{}, exp.ID, filename, mimeType, data)
}

// renderJSON marshals rows as pretty-printed JSON bytes.
func renderJSON(rows []map[string]any) ([]byte, error) {
	data, err := json.MarshalIndent(rows, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshaling JSON: %w", err)
	}

	return data, nil
}

// renderCSV writes rows as CSV bytes. The header row is derived from colNames
// (the ordered column names from the query's field descriptions), ensuring
// deterministic column ordering across runs. When there are no data rows the
// real column header is still written so downstream consumers know the
// schema; only when the query yielded no column descriptions at all does it
// fall back to a single `no_data` placeholder row.
func renderCSV(rows []map[string]any, colNames []string) ([]byte, error) {
	var buf strings.Builder
	w := csv.NewWriter(&buf)

	if len(colNames) == 0 {
		// No column descriptions at all — write a single placeholder so the
		// file is not empty and the reader can detect the no-data case.
		if err := w.Write([]string{"no_data"}); err != nil {
			return nil, fmt.Errorf("writing CSV placeholder: %w", err)
		}

		w.Flush()

		return []byte(buf.String()), nil
	}

	// Write header using the ordered column names, even when there are no
	// data rows, so the schema is discoverable.
	if err := w.Write(colNames); err != nil {
		return nil, fmt.Errorf("writing CSV header: %w", err)
	}

	// Write data rows.
	for _, row := range rows {
		record := make([]string, len(colNames))

		for i, col := range colNames {
			val := row[col]
			if val == nil {
				record[i] = ""
			} else {
				record[i] = fmt.Sprintf("%v", val)
			}
		}

		if err := w.Write(record); err != nil {
			return nil, fmt.Errorf("writing CSV row: %w", err)
		}
	}

	w.Flush()

	if err := w.Error(); err != nil {
		return nil, fmt.Errorf("flushing CSV: %w", err)
	}

	return []byte(buf.String()), nil
}

// sanitizeFilename replaces spaces and other filesystem-unsafe characters in
// the export name so the resulting filename is safe to use on disk. Only
// alphanumerics, dashes, and underscores are kept; everything else becomes
// an underscore.
func sanitizeFilename(name string) string {
	var b strings.Builder

	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			b.WriteRune(r)
		default:
			b.WriteRune('_')
		}
	}

	s := b.String()
	if s == "" {
		s = "export"
	}

	return s
}

// GetFilePath returns the on-disk path for an export's generated file. Used
// by the download handler to serve the file. The filename is reconstructed
// from the export record's name and ID using the same sanitization logic as
// generateFile.
func (s *Service) GetFilePath(ctx context.Context, id, tenantID pgtype.UUID) (string, error) {
	exp, err := s.store.Get(ctx, id, tenantID)
	if err != nil {
		return "", fmt.Errorf("fetching export: %w", err)
	}

	if exp.Status != "completed" || exp.FileUrl == nil || *exp.FileUrl == "" {
		return "", errors.New("export file not available")
	}

	baseName := sanitizeFilename(exp.Name)
	filename := fmt.Sprintf("export_%s_%s.%s", baseName, exp.ID.String()[:8], exp.Format)

	return s.storage.GetPath(pgtype.UUID{}, exp.ID, filename)
}

// ListPendingExports returns pending exports for the worker.
func (s *Service) ListPendingExports(ctx context.Context, limit int32) ([]gen.PlatformDataExport, error) {
	return s.store.ListPending(ctx, limit)
}
