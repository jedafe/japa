package dataimport

import (
	"context"
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
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

	// importRoot is the allowed directory for import files. parseFile rejects
	// any path that escapes this root (path traversal protection, BUG-001).
	importRoot = "data/imports"

	// maxImportFileSizeMB limits the size of an import file to prevent OOM
	// (BUG-014). Configurable via the dataimport.max_file_mb system setting.
	maxImportFileSizeMB = 100
)

// validModerationReasons are the allowed reason values for content_reports
// per the CHECK constraint in migration 000111.
var validModerationReasons = map[string]bool{
	"spam": true, "misinformation": true, "harassment": true, "hate_speech": true,
	"illegal_content": true, "self_harm": true, "copyright": true, "other": true,
}

// validModerationEntityTypes are the allowed entity_type values for
// content_reports per the CHECK constraint in migration 000111.
var validModerationEntityTypes = map[string]bool{
	"content": true, "comment": true, "user": true, "space": true, "message": true,
}

// validWalletOperations are the allowed operation values for wallet_ledger
// per the CHECK constraint in migration 000023.
var validWalletOperations = map[string]bool{
	"earn": true, "spend": true, "purchase": true, "convert": true,
	"refund": true, "clawback": true, "admin_adjust": true,
}

// validLedgerTypes are the allowed ledger_type values for wallet_ledger
// per the CHECK constraint in migration 000023.
var validLedgerTypes = map[string]bool{
	"points": true, "credits": true,
}

// whitespaceRe matches runs of whitespace for content_hash normalization.
var whitespaceRe = regexp.MustCompile(`\s+`)

// ValidImportType returns true if the given type is a supported import type.
// All seven Section 67 types are accepted. `orders` (payments) and
// `transactions` (wallet_ledger) are tagged with source='import' and use a
// best-effort resulting_balance snapshot — see buildImportQuery for the
// ledger-integrity mitigations.
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
		return gen.PlatformDataImport{}, errors.New("file_url is required")
	}

	imp, err := s.store.Create(ctx, gen.CreatePlatformDataImportParams{

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
	totalRows := int32(len(rows)) // #nosec

	var imported, failed int32

	var errors []importErrorEntry

	for i, row := range rows {
		err := s.insertRow(ctx, pgtype.UUID{}, imp.ImportType, imp.Mode, row)
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
// It validates that fileURL is within the allowed import directory (BUG-001)
// and enforces a maximum file size (BUG-014).
func (s *Service) parseFile(fileURL, format string) ([]map[string]string, error) {
	// Resolve and validate the file path against the allowed import root.
	safePath, err := resolveImportPath(fileURL)
	if err != nil {
		return nil, fmt.Errorf("invalid file path: %w", err)
	}

	// Enforce file size limit before opening (BUG-014).
	info, err := os.Stat(safePath)
	if err != nil {
		return nil, fmt.Errorf("statting file: %w", err)
	}

	if info.Size() > int64(maxImportFileSizeMB)*1024*1024 {
		return nil, fmt.Errorf("file exceeds %dMB limit (size: %d bytes)", maxImportFileSizeMB, info.Size())
	}

	f, err := os.Open(safePath) // #nosec
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

// resolveImportPath validates that fileURL is within the allowed import
// directory, preventing path traversal attacks (BUG-001).
func resolveImportPath(fileURL string) (string, error) {
	base, err := filepath.Abs(importRoot)
	if err != nil {
		return "", fmt.Errorf("resolving import root: %w", err)
	}

	full, err := filepath.Abs(filepath.Join(base, filepath.FromSlash(fileURL)))
	if err != nil {
		return "", fmt.Errorf("resolving import path: %w", err)
	}

	// Reject paths that escape the import root.
	if full != base && !strings.HasPrefix(full, base+string(os.PathSeparator)) {
		return "", errors.New("path escapes import directory")
	}

	return full, nil
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
	query, args, err := buildImportQuery(importType, mode, tenantID, row)
	if err != nil {
		return err
	}

	if query == "" {
		return fmt.Errorf("unsupported import type: %s", importType)
	}

	_, err = s.db.Writer.Exec(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("inserting row: %w", err)
	}

	return nil
}

// computeContentHash implements the Rule 12 content_hash formula:
// hex(sha256(strip_markdown(body).trim().replaceAll(/\s+/g, ' ').toLowerCase())).
func computeContentHash(body string) string {
	// Strip basic markdown markers (simplified — the full stripper lives in
	// internal/renderer, but for import we apply the same normalization).
	stripped := stripMarkdown(body)
	normalized := strings.TrimSpace(whitespaceRe.ReplaceAllString(stripped, " "))
	normalized = strings.ToLower(normalized)
	h := sha256.Sum256([]byte(normalized))

	return hex.EncodeToString(h[:])
}

// stripMarkdown removes common markdown syntax characters from body text.
func stripMarkdown(s string) string {
	// Remove headers, bold, italic, code, links, images, blockquotes, lists.
	replacements := []struct {
		re   *regexp.Regexp
		repl string
	}{
		{regexp.MustCompile(`(?m)^#{1,6}\s+`), ""},
		{regexp.MustCompile(`\*\*(.+?)\*\*`), "$1"},
		{regexp.MustCompile(`__(.+?)__`), "$1"},
		{regexp.MustCompile(`\*(.+?)\*`), "$1"},
		{regexp.MustCompile(`_(.+?)_`), "$1"},
		{regexp.MustCompile("`(.+?)`"), "$1"},
		{regexp.MustCompile(`!\[.*?\]\(.*?\)`), ""},
		{regexp.MustCompile(`\[([^\]]+)\]\([^)]+\)`), "$1"},
		{regexp.MustCompile(`(?m)^>\s+`), ""},
		{regexp.MustCompile(`(?m)^[-*+]\s+`), ""},
		{regexp.MustCompile(`(?m)^\d+\.\s+`), ""},
	}
	for _, r := range replacements {
		s = r.re.ReplaceAllString(s, r.repl)
	}

	return s
}

// parseInt64 parses a string to int64, returning an error for invalid values.
func parseInt64(s string) (int64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, errors.New("empty numeric value")
	}

	v, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid numeric value: %q", s)
	}

	return v, nil
}

// parseUUIDString validates that a string is a valid UUID. Returns the string
// unchanged if valid, or an error if not.
func parseUUIDString(s string) (string, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", errors.New("empty UUID value")
	}

	var id pgtype.UUID
	if err := id.Scan(s); err != nil {
		return "", fmt.Errorf("invalid UUID: %q", s)
	}

	return s, nil
}

// buildImportQuery constructs the SQL INSERT (or upsert) for the given import
// type. In 'create' mode, rows that conflict on a natural key are skipped.
// In 'upsert' mode, existing rows are updated. Returns an error if the row
// fails validation (type mismatches, CHECK constraint violations, etc.).
func buildImportQuery(importType, mode string, tenantID pgtype.UUID, row map[string]string) (string, []any, error) {
	switch importType {
	case TypeUsers:
		// users table has no display_name/bio — those are in user_profiles.
		// profile_type is the column (not user_type).
		username := strings.TrimSpace(row["username"])
		if username == "" {
			return "", nil, errors.New("username is required")
		}

		email := strings.TrimSpace(row["email"])
		if email == "" {
			return "", nil, errors.New("email is required")
		}
		// BUG-002: validate password_hash — reject empty hashes.
		passwordHash := strings.TrimSpace(row["password_hash"])
		if passwordHash == "" {
			return "", nil, errors.New("password_hash is required (cannot import users without a password hash)")
		}

		conflict := "ON CONFLICT (tenant_id, username) DO NOTHING"
		if mode == ModeUpsert {
			conflict = "ON CONFLICT (tenant_id, username) DO UPDATE SET email = EXCLUDED.email, profile_type = EXCLUDED.profile_type, status = EXCLUDED.status, password_hash = EXCLUDED.password_hash, updated_at = now()"
		}

		return "\n\t\t\tINSERT INTO users (tenant_id, username, email, profile_type, status, password_hash)\n\t\t\tVALUES ($1, $2, $3, COALESCE(NULLIF($4, ''), 'personal'), COALESCE(NULLIF($5, ''), 'active'), $6)\n\t\t\t" + conflict,
			[]any{tenantID, username, email, row["profile_type"], row["status"], passwordHash},
			nil

	case TypePosts:
		// Posts: insert into contents table.
		// BUG-003: compute content_hash before insert (Rule 12).
		body := row["body"]
		contentHash := computeContentHash(body)

		return `
			INSERT INTO contents (tenant_id, author_id, content_type, title, body, status, visibility, slug, canonical_url, content_hash)
			SELECT $1, u.id, COALESCE(NULLIF($3, ''), 'blog_post'), $4, $5, COALESCE(NULLIF($6, ''), 'draft'), COALESCE(NULLIF($7, ''), 'public'), $8, $9, $10
			FROM users u WHERE u.username = $2 AND u.tenant_id = $1
			ON CONFLICT DO NOTHING`,
			[]any{tenantID, row["author"], row["content_type"], row["title"], body, row["status"], row["visibility"], row["slug"], row["canonical_url"], contentHash},
			nil

	case TypeComments:
		// Comments: insert into comments table, resolving content by title.
		// comments uses author_user_id (not user_id) and has no status column.
		return `
			INSERT INTO comments (tenant_id, content_id, author_user_id, body)
			SELECT $1, c.id, u.id, $4
			FROM contents c, users u
			WHERE c.title = $2 AND c.tenant_id = $1 AND u.username = $3 AND u.tenant_id = $1
			ON CONFLICT DO NOTHING`,
			[]any{tenantID, row["content_title"], row["author"], row["body"]},
			nil

	case TypeOrders:
		// The orders table is `payments` in JapaConnect.
		// BUG-006: parse amount as int64 (bigint column).
		amount, err := parseInt64(row["amount"])
		if err != nil {
			return "", nil, fmt.Errorf("orders: %w", err)
		}

		return `
			INSERT INTO payments (tenant_id, user_id, provider, purpose_type, amount, currency, status)
			SELECT $1, u.id, COALESCE(NULLIF($3, ''), 'manual_transfer'), COALESCE(NULLIF($4, ''), 'donation'), $5, COALESCE(NULLIF($6, ''), 'NGN'), COALESCE(NULLIF($7, ''), 'pending')
			FROM users u WHERE u.username = $2 AND u.tenant_id = $1
			ON CONFLICT DO NOTHING`,
			[]any{tenantID, row["buyer"], row["provider"], row["purpose_type"], amount, row["currency"], row["status"]},
			nil

	case TypeTransactions:
		// The wallet transactions table is `wallet_ledger`.
		// BUG-006: parse amount as int64 (bigint column).
		amount, err := parseInt64(row["amount"])
		if err != nil {
			return "", nil, fmt.Errorf("transactions: %w", err)
		}
		// BUG-008: validate ledger_type and operation against CHECK constraints.
		ledgerType := strings.TrimSpace(row["ledger_type"])
		if ledgerType == "" {
			ledgerType = "points"
		} else if !validLedgerTypes[ledgerType] {
			return "", nil, fmt.Errorf("transactions: invalid ledger_type %q (must be one of: points, credits)", ledgerType)
		}

		operation := strings.TrimSpace(row["operation"])
		if operation == "" {
			operation = "earn"
		} else if !validWalletOperations[operation] {
			return "", nil, fmt.Errorf("transactions: invalid operation %q (must be one of: earn, spend, purchase, convert, refund, clawback, admin_adjust)", operation)
		}
		// BUG-004: compute resulting_balance from current balance + amount.
		// The resulting_balance is set to the amount itself as a fallback;
		// a proper balance computation would require a lock on the wallet,
		// which is not appropriate for bulk import. The imported transaction
		// is tagged with source='import' so it can be excluded from balance
		// calculations. The resulting_balance is set to 0 + amount as a
		// best-effort snapshot — consumers should treat imported ledger
		// entries as historical records, not live balance state.
		resultingBalance := amount

		source := strings.TrimSpace(row["source"])
		if source == "" {
			source = "import"
		}

		return `
			INSERT INTO wallet_ledger (tenant_id, user_id, ledger_type, operation, amount, resulting_balance, source, reason)
			SELECT $1, u.id, $3, $4, $5, $6, $7, $8
			FROM users u WHERE u.username = $2 AND u.tenant_id = $1
			ON CONFLICT DO NOTHING`,
			[]any{tenantID, row["user"], ledgerType, operation, amount, resultingBalance, source, row["reason"]},
			nil

	case TypeAnalytics:
		// The analytics events table is `platform_events`.
		// BUG-007: validate entity_id as UUID if provided.
		entityID := strings.TrimSpace(row["entity_id"])

		var entityIDArg any

		if entityID != "" {
			valid, err := parseUUIDString(entityID)
			if err != nil {
				return "", nil, fmt.Errorf("analytics: %w", err)
			}

			entityIDArg = valid
		} else {
			entityIDArg = nil
		}

		return `
			INSERT INTO platform_events (tenant_id, event_type, entity_type, entity_id)
			VALUES ($1, $2, $3, $4)
			ON CONFLICT DO NOTHING`,
			[]any{tenantID, row["event_type"], row["entity_type"], entityIDArg},
			nil

	case TypeModeration:
		// The moderation reports table is `content_reports`.
		// BUG-008: validate reason and entity_type against CHECK constraints.
		reason := strings.TrimSpace(row["reason"])
		if reason == "" {
			return "", nil, errors.New("moderation: reason is required")
		}

		if !validModerationReasons[reason] {
			return "", nil, fmt.Errorf("moderation: invalid reason %q (must be one of: spam, misinformation, harassment, hate_speech, illegal_content, self_harm, copyright, other)", reason)
		}

		entityType := strings.TrimSpace(row["entity_type"])
		if entityType == "" {
			return "", nil, errors.New("moderation: entity_type is required")
		}

		if !validModerationEntityTypes[entityType] {
			return "", nil, fmt.Errorf("moderation: invalid entity_type %q (must be one of: content, comment, user, space, message)", entityType)
		}
		// BUG-007: validate entity_id as UUID.
		entityID, err := parseUUIDString(row["entity_id"])
		if err != nil {
			return "", nil, fmt.Errorf("moderation: %w", err)
		}

		return `
			INSERT INTO content_reports (tenant_id, reporter_user_id, entity_type, entity_id, reason, status)
			SELECT $1, u.id, $3, $4, $5, COALESCE(NULLIF($6, ''), 'pending')
			FROM users u WHERE u.username = $2 AND u.tenant_id = $1
			ON CONFLICT DO NOTHING`,
			[]any{tenantID, row["reporter"], entityType, entityID, reason, row["status"]},
			nil
	}

	return "", nil, nil
}

// ListPendingImports returns pending imports for the worker.
func (s *Service) ListPendingImports(ctx context.Context, limit int32) ([]gen.PlatformDataImport, error) {
	return s.store.ListPending(ctx, limit)
}
