package dataimport

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
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

// Valid import types and formats.
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

	ModeCreate = "create" // skip existing rows
	ModeUpsert = "upsert" // update or insert
)

// ValidImportType returns true if the given type is a supported import type.
func ValidImportType(t string) bool {
	switch t {
	case TypeUsers, TypePosts, TypeComments, TypeOrders, TypeTransactions, TypeAnalytics, TypeModeration:
		return true
	}
	return false
}

// ValidFormat returns true if the given format is supported for import.
func ValidFormat(f string) bool {
	return f == FormatCSV || f == FormatJSON
}

// ValidMode returns true if the given mode is supported.
func ValidMode(m string) bool {
	return m == ModeCreate || m == ModeUpsert
}

// importErrorEntry is a single error in the import's error_log.
type importErrorEntry struct {
	Row   int    `json:"row"`
	Error string `json:"error"`
}

// Service provides platform data import operations.
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

// CreateImport creates a new import job and enqueues a background task.
// The fileURL points to the uploaded file (SeaweedFS path or local path).
func (s *Service) CreateImport(ctx context.Context, tenantID, userID pgtype.UUID, name, importType, format, fileURL, mode string) (gen.PlatformDataImport, error) {
	if !ValidImportType(importType) {
		return gen.PlatformDataImport{}, fmt.Errorf("invalid import_type: %s", importType)
	}
	if !ValidFormat(format) {
		return gen.PlatformDataImport{}, fmt.Errorf("invalid format: %s", format)
	}
	if !ValidMode(mode) {
		mode = ModeCreate // default
	}
	if fileURL == "" {
		return gen.PlatformDataImport{}, fmt.Errorf("file_url is required")
	}

	imp, err := s.store.Create(ctx, gen.CreatePlatformDataImportParams{
		TenantID:   tenantID,
		Name:       name,
		ImportType: importType,
		Format:     format,
		FileUrl:    fileURL,
		Mode:       mode,
		CreatedBy:  userID,
	})
	if err != nil {
		return gen.PlatformDataImport{}, fmt.Errorf("creating import: %w", err)
	}

	// Enqueue background task (fire-and-forget, Rule 8).
	payload, _ := json.Marshal(map[string]string{
		"import_id": imp.ID.String(),
		"tenant_id": tenantID.String(),
	})
	if err := s.enqueuer.Enqueue("platform:data_import", payload); err != nil {
		slog.Error("dataimport: enqueue failed", "error", err)
	}

	return imp, nil
}

// GetImport retrieves a single import by ID.
func (s *Service) GetImport(ctx context.Context, id, tenantID pgtype.UUID) (gen.PlatformDataImport, error) {
	return s.store.Get(ctx, id, tenantID)
}

// ListImports returns paginated imports for a tenant.
func (s *Service) ListImports(ctx context.Context, tenantID pgtype.UUID, status string, page, limit int) ([]gen.PlatformDataImport, int64, error) {
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

// DeleteImport removes an import record.
func (s *Service) DeleteImport(ctx context.Context, id, tenantID pgtype.UUID) error {
	return s.store.Delete(ctx, id, tenantID)
}

// ProcessImport is called by the Asynq worker. It reads the uploaded file,
// parses each row, and inserts/updates records in the relevant table.
func (s *Service) ProcessImport(ctx context.Context, importID pgtype.UUID) error {
	imp, err := s.store.GetByID(ctx, importID)
	if err != nil {
		return fmt.Errorf("fetching import: %w", err)
	}

	// Mark as processing.
	_, err = s.store.UpdateStatus(ctx, gen.UpdatePlatformDataImportStatusParams{
		ID:        importID,
		Status:    "processing",
		TotalRows: 0, ImportedRows: 0, FailedRows: 0, ErrorLog: []byte("[]"),
	})
	if err != nil {
		return fmt.Errorf("marking import as processing: %w", err)
	}

	// Read and parse the file.
	rows, err := s.parseFile(imp.FileUrl, imp.Format)
	if err != nil {
		s.markFailed(ctx, importID, fmt.Sprintf("parsing file: %v", err))
		return fmt.Errorf("parsing file: %w", err)
	}

	// Insert/update each row.
	totalRows := int32(len(rows))
	var imported, failed int32
	var errors []importErrorEntry

	for i, row := range rows {
		err := s.insertRow(ctx, imp.TenantID, imp.ImportType, imp.Mode, row)
		if err != nil {
			failed++
			errors = append(errors, importErrorEntry{
				Row:   i + 1,
				Error: err.Error(),
			})
			slog.Warn("dataimport: row failed",
				"import_id", importID.String(),
				"row", i+1, "error", err)
		} else {
			imported++
		}
	}

	// Determine final status.
	finalStatus := "completed"
	if failed > 0 && imported == 0 {
		finalStatus = "failed"
	} else if failed > 0 {
		finalStatus = "partial"
	}

	errorLog, _ := json.Marshal(errors)

	_, err = s.store.UpdateStatus(ctx, gen.UpdatePlatformDataImportStatusParams{
		ID:           importID,
		Status:       finalStatus,
		TotalRows:    totalRows,
		ImportedRows: imported,
		FailedRows:   failed,
		ErrorLog:     errorLog,
		CompletedAt:  pgtype.Timestamptz{Time: time.Now(), Valid: true},
	})
	if err != nil {
		return fmt.Errorf("updating import status: %w", err)
	}

	slog.Info("dataimport: import completed",
		"import_id", importID.String(),
		"type", imp.ImportType,
		"total", totalRows, "imported", imported, "failed", failed,
		"status", finalStatus)

	return nil
}

// markFailed updates the import status to 'failed'.
func (s *Service) markFailed(ctx context.Context, importID pgtype.UUID, errMsg string) {
	errors, _ := json.Marshal([]importErrorEntry{{Row: 0, Error: errMsg}})
	_, err := s.store.UpdateStatus(ctx, gen.UpdatePlatformDataImportStatusParams{
		ID:           importID,
		Status:       "failed",
		TotalRows:    0,
		ImportedRows: 0,
		FailedRows:   0,
		ErrorLog:     errors,
	})
	if err != nil {
		slog.Error("dataimport: failed to mark import as failed", "error", err)
	}
}

// parseFile reads the file and returns a slice of row maps.
func (s *Service) parseFile(fileURL, format string) ([]map[string]string, error) {
	f, err := os.Open(fileURL)
	if err != nil {
		return nil, fmt.Errorf("opening file: %w", err)
	}
	defer f.Close()

	switch format {
	case FormatCSV:
		return parseCSV(f)
	case FormatJSON:
		return parseJSON(f)
	default:
		return nil, fmt.Errorf("unsupported format: %s", format)
	}
}

// parseCSV reads a CSV file and returns rows as map[string]string (column
// name → value). The first row is the header.
func parseCSV(r io.Reader) ([]map[string]string, error) {
	reader := csv.NewReader(r)
	reader.FieldsPerRecord = -1 // allow variable fields

	headers, err := reader.Read()
	if err != nil {
		return nil, fmt.Errorf("reading CSV header: %w", err)
	}

	var rows []map[string]string
	for {
		record, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("reading CSV row: %w", err)
		}

		row := make(map[string]string, len(headers))
		for i, header := range headers {
			if i < len(record) {
				row[strings.TrimSpace(header)] = record[i]
			}
		}
		rows = append(rows, row)
	}

	return rows, nil
}

// parseJSON reads a JSON file (array of objects) and returns rows as
// map[string]string. Non-string values are converted to strings.
func parseJSON(r io.Reader) ([]map[string]string, error) {
	var raw []map[string]any
	if err := json.NewDecoder(r).Decode(&raw); err != nil {
		return nil, fmt.Errorf("decoding JSON: %w", err)
	}

	rows := make([]map[string]string, 0, len(raw))
	for _, obj := range raw {
		row := make(map[string]string, len(obj))
		for k, v := range obj {
			if v == nil {
				row[k] = ""
			} else {
				row[k] = fmt.Sprintf("%v", v)
			}
		}
		rows = append(rows, row)
	}

	return rows, nil
}

// insertRow inserts or upserts a single row into the relevant table.
func (s *Service) insertRow(ctx context.Context, tenantID pgtype.UUID, importType, mode string, row map[string]string) error {
	query, args := buildImportQuery(importType, mode, tenantID, row)
	if query == "" {
		return fmt.Errorf("unsupported import type: %s", importType)
	}

	_, err := s.db.Writer.Exec(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("inserting row: %w", err)
	}

	return nil
}

// buildImportQuery constructs the SQL INSERT (or upsert) for the given import
// type. In 'create' mode, rows that conflict on a natural key are skipped.
// In 'upsert' mode, existing rows are updated.
func buildImportQuery(importType, mode string, tenantID pgtype.UUID, row map[string]string) (string, []any) {
	switch importType {
	case TypeUsers:
		// users table has no display_name/bio — those are in user_profiles.
		// profile_type is the column (not user_type).
		conflict := "ON CONFLICT (tenant_id, username) DO NOTHING"
		if mode == ModeUpsert {
			conflict = "ON CONFLICT (tenant_id, username) DO UPDATE SET email = EXCLUDED.email, profile_type = EXCLUDED.profile_type, status = EXCLUDED.status, updated_at = now()"
		}
		return fmt.Sprintf(`
			INSERT INTO users (tenant_id, username, email, profile_type, status, password_hash)
			VALUES ($1, $2, $3, COALESCE(NULLIF($4, ''), 'personal'), COALESCE(NULLIF($5, ''), 'active'), $6)
			%s`, conflict),
			[]any{tenantID, row["username"], row["email"], row["profile_type"], row["status"], row["password_hash"]}

	case TypePosts:
		// Posts: insert into contents table.
		return `
			INSERT INTO contents (tenant_id, author_id, content_type, title, body, status, visibility, slug, canonical_url)
			SELECT $1, u.id, COALESCE(NULLIF($3, ''), 'blog_post'), $4, $5, COALESCE(NULLIF($6, ''), 'draft'), COALESCE(NULLIF($7, ''), 'public'), $8, $9
			FROM users u WHERE u.username = $2 AND u.tenant_id = $1
			ON CONFLICT DO NOTHING`,
			[]any{tenantID, row["author"], row["content_type"], row["title"], row["body"], row["status"], row["visibility"], row["slug"], row["canonical_url"]}

	case TypeComments:
		// Comments: insert into comments table, resolving content by title.
		// comments uses author_user_id (not user_id) and has no status column.
		return `
			INSERT INTO comments (tenant_id, content_id, author_user_id, body)
			SELECT $1, c.id, u.id, $4
			FROM contents c, users u
			WHERE c.title = $2 AND c.tenant_id = $1 AND u.username = $3 AND u.tenant_id = $1
			ON CONFLICT DO NOTHING`,
			[]any{tenantID, row["content_title"], row["author"], row["body"]}

	case TypeOrders:
		// The orders table is `payments` in JapaConnect.
		return `
			INSERT INTO payments (tenant_id, user_id, provider, purpose_type, amount, currency, status)
			SELECT $1, u.id, COALESCE(NULLIF($3, ''), 'manual_transfer'), COALESCE(NULLIF($4, ''), 'donation'), $5, COALESCE(NULLIF($6, ''), 'NGN'), COALESCE(NULLIF($7, ''), 'pending')
			FROM users u WHERE u.username = $2 AND u.tenant_id = $1
			ON CONFLICT DO NOTHING`,
			[]any{tenantID, row["buyer"], row["provider"], row["purpose_type"], row["amount"], row["currency"], row["status"]}

	case TypeTransactions:
		// The wallet transactions table is `wallet_ledger`.
		return `
			INSERT INTO wallet_ledger (tenant_id, user_id, ledger_type, operation, amount, resulting_balance, source, reason)
			SELECT $1, u.id, COALESCE(NULLIF($3, ''), 'points'), COALESCE(NULLIF($4, ''), 'earn'), $5, 0, $6, $7
			FROM users u WHERE u.username = $2 AND u.tenant_id = $1
			ON CONFLICT DO NOTHING`,
			[]any{tenantID, row["user"], row["ledger_type"], row["operation"], row["amount"], row["source"], row["reason"]}

	case TypeAnalytics:
		// The analytics events table is `platform_events`.
		return `
			INSERT INTO platform_events (tenant_id, event_type, entity_type, entity_id)
			VALUES ($1, $2, $3, $4)
			ON CONFLICT DO NOTHING`,
			[]any{tenantID, row["event_type"], row["entity_type"], row["entity_id"]}

	case TypeModeration:
		// The moderation reports table is `content_reports`.
		return `
			INSERT INTO content_reports (tenant_id, reporter_user_id, entity_type, entity_id, reason, status)
			SELECT $1, u.id, $3, $4, $5, COALESCE(NULLIF($6, ''), 'pending')
			FROM users u WHERE u.username = $2 AND u.tenant_id = $1
			ON CONFLICT DO NOTHING`,
			[]any{tenantID, row["reporter"], row["entity_type"], row["entity_id"], row["reason"], row["status"]}
	}

	return "", nil
}

// ListPendingImports returns pending imports for the worker.
func (s *Service) ListPendingImports(ctx context.Context, limit int32) ([]gen.PlatformDataImport, error) {
	return s.store.ListPending(ctx, limit)
}
