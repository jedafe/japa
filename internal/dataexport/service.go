package dataexport

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
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

// Valid export types and formats.
const (
	TypeUsers       = "users"
	TypePosts       = "posts"
	TypeComments    = "comments"
	TypeOrders      = "orders"
	TypeTransactions = "transactions"
	TypeAnalytics   = "analytics"
	TypeModeration  = "moderation"

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

// ValidFormat returns true if the given format is supported.
func ValidFormat(f string) bool {
	return f == FormatCSV || f == FormatJSON || f == FormatXLSX
}

// Service provides platform data export operations.
type Service struct {
	store    *Store
	db       *db.DB
	enqueuer Enqueuer
}

// NewService creates a Service backed by the given database.
func NewService(database *db.DB) *Service {
	return &Service{
		store:    NewStore(database),
		db:       database,
		enqueuer: &noopEnqueuer{},
	}
}

// SetEnqueuer injects the Asynq enqueuer for background task dispatch.
func (s *Service) SetEnqueuer(e Enqueuer) {
	s.enqueuer = e
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
		TenantID:        tenantID,
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
	offset := int32((page - 1) * limit)
	rows, err := s.store.List(ctx, tenantID, status, int32(limit), offset)
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
// with the download URL.
func (s *Service) ProcessExport(ctx context.Context, exportID pgtype.UUID) error {
	exp, err := s.store.GetByID(ctx, exportID)
	if err != nil {
		return fmt.Errorf("fetching export: %w", err)
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
	rows, colNames, err := s.collectData(ctx, exp.TenantID, exp.ExportType, exp.IncludeMetadata)
	if err != nil {
		s.markFailed(ctx, exportID, fmt.Sprintf("collecting data: %v", err))
		return fmt.Errorf("collecting data: %w", err)
	}

	// Generate the file in the requested format.
	fileURL, fileSize, err := s.generateFile(exp, rows, colNames)
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
func buildExportQuery(exportType string, tenantID pgtype.UUID, includeMetadata bool) (string, []any) {
	metaCols := ""
	if includeMetadata {
		metaCols = ", id, created_at, updated_at"
	}

	switch exportType {
	case TypeUsers:
		// users table has no display_name/bio — those are in user_profiles.
		// profile_type is the column (not user_type).
		return `SELECT u.username, u.email, u.profile_type, u.status,
			p.display_name, p.bio` + metaCols + `
			FROM users u
			LEFT JOIN user_profiles p ON p.user_id = u.id AND p.tenant_id = u.tenant_id
			WHERE u.tenant_id = $1 ORDER BY u.created_at DESC`, []any{tenantID}

	case TypePosts:
		return `SELECT c.title, c.body, c.content_type, c.status, c.visibility,
			c.slug, c.canonical_url` + metaCols + `, u.username AS author
			FROM contents c
			JOIN users u ON u.id = c.author_id
			WHERE c.tenant_id = $1 AND c.deleted_at IS NULL
			ORDER BY c.created_at DESC`, []any{tenantID}

	case TypeComments:
		// comments table uses author_user_id (not user_id) and has no status column.
		return `SELECT cm.body` + metaCols + `,
			u.username AS author, c.title AS content_title
			FROM comments cm
			JOIN users u ON u.id = cm.author_user_id
			JOIN contents c ON c.id = cm.content_id
			WHERE cm.tenant_id = $1 AND cm.deleted_at IS NULL
			ORDER BY cm.created_at DESC`, []any{tenantID}

	case TypeOrders:
		// The orders table is `payments` in JapaConnect.
		return `SELECT p.provider, p.purpose_type, p.status, p.amount, p.currency` + metaCols + `,
			u.username AS buyer
			FROM payments p
			JOIN users u ON u.id = p.user_id
			WHERE p.tenant_id = $1
			ORDER BY p.created_at DESC`, []any{tenantID}

	case TypeTransactions:
		// The wallet transactions table is `wallet_ledger`.
		return `SELECT w.ledger_type, w.operation, w.amount, w.source, w.reason` + metaCols + `,
			u.username AS user
			FROM wallet_ledger w
			JOIN users u ON u.id = w.user_id
			WHERE w.tenant_id = $1
			ORDER BY w.created_at DESC`, []any{tenantID}

	case TypeAnalytics:
		// The analytics events table is `platform_events`.
		return `SELECT event_type, entity_type, entity_id, count(*) AS event_count,
			date_trunc('day', created_at) AS day
			FROM platform_events
			WHERE tenant_id = $1
			GROUP BY event_type, entity_type, entity_id, day
			ORDER BY day DESC`, []any{tenantID}

	case TypeModeration:
		// The moderation reports table is `content_reports`.
		return `SELECT cr.entity_type, cr.reason, cr.status` + metaCols + `,
			u.username AS reporter
			FROM content_reports cr
			LEFT JOIN users u ON u.id = cr.reporter_user_id
			WHERE cr.tenant_id = $1
			ORDER BY cr.created_at DESC`, []any{tenantID}
	}

	return "", nil
}

// generateFile writes the export data to a file in the requested format and
// returns the file URL and size. In production this would upload to S3/
// SeaweedFS; for now it writes to a local tmp directory and returns a
// relative path.
func (s *Service) generateFile(exp gen.PlatformDataExport, rows []map[string]any, colNames []string) (string, int64, error) {
	dir := os.TempDir()
	baseName := strings.ReplaceAll(exp.Name, " ", "_")
	filePath := fmt.Sprintf("%s/export_%s_%s.%s", dir, baseName, exp.ID.String()[:8], exp.Format)

	switch exp.Format {
	case FormatJSON:
		return s.generateJSON(filePath, rows)
	case FormatCSV:
		return s.generateCSV(filePath, rows, colNames)
	case FormatXLSX:
		// XLSX is generated as CSV for now — a proper XLSX library can be
		// added later. The file extension is .csv to avoid misleading users
		// with an .xlsx file that contains CSV content.
		filePath := fmt.Sprintf("%s/export_%s_%s.csv", dir, baseName, exp.ID.String()[:8])
		return s.generateCSV(filePath, rows, colNames)
	default:
		return "", 0, fmt.Errorf("unsupported format: %s", exp.Format)
	}
}

// generateJSON writes rows as pretty-printed JSON.
func (s *Service) generateJSON(filePath string, rows []map[string]any) (string, int64, error) {
	data, err := json.MarshalIndent(rows, "", "  ")
	if err != nil {
		return "", 0, fmt.Errorf("marshaling JSON: %w", err)
	}
	if err := os.WriteFile(filePath, data, 0644); err != nil {
		return "", 0, fmt.Errorf("writing JSON file: %w", err)
	}
	info, _ := os.Stat(filePath)
	return filePath, info.Size(), nil
}

// generateCSV writes rows as CSV. The header row is derived from colNames
// (the ordered column names from the query's field descriptions), ensuring
// deterministic column ordering across runs.
func (s *Service) generateCSV(filePath string, rows []map[string]any, colNames []string) (string, int64, error) {
	f, err := os.Create(filePath)
	if err != nil {
		return "", 0, fmt.Errorf("creating CSV file: %w", err)
	}
	defer f.Close()

	w := csv.NewWriter(f)
	defer w.Flush()

	if len(rows) == 0 || len(colNames) == 0 {
		// Write an empty file with just a header placeholder.
		if err := w.Write([]string{"no_data"}); err != nil {
			return "", 0, fmt.Errorf("writing CSV placeholder: %w", err)
		}
		w.Flush()
		info, _ := os.Stat(filePath)
		return filePath, info.Size(), nil
	}

	// Write header using the ordered column names.
	if err := w.Write(colNames); err != nil {
		return "", 0, fmt.Errorf("writing CSV header: %w", err)
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
			return "", 0, fmt.Errorf("writing CSV row: %w", err)
		}
	}

	w.Flush()
	if err := w.Error(); err != nil {
		return "", 0, fmt.Errorf("flushing CSV: %w", err)
	}

	info, _ := os.Stat(filePath)
	return filePath, info.Size(), nil
}

// ListPendingExports returns pending exports for the worker.
func (s *Service) ListPendingExports(ctx context.Context, limit int32) ([]gen.PlatformDataExport, error) {
	return s.store.ListPending(ctx, limit)
}
