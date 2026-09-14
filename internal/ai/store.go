package ai

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Store provides database access for AI HITL review operations.
// Uses raw SQL instead of sqlc because the ai_hitl_reviews table is simple
// enough that generated code would add indirection without value.
type Store struct {
	write *pgxpool.Pool
	read  *pgxpool.Pool
}

// NewStore creates a Store backed by the given connection pools.
func NewStore(read, write *pgxpool.Pool) *Store {
	return &Store{read: read, write: write}
}

// ---------------------------------------------------------------------------
// HITL Review CRUD
// ---------------------------------------------------------------------------

// HITLReview represents a human-in-the-loop review record.
type HITLReview struct {
	ID pgtype.UUID `json:"id"`

	ContentID     pgtype.UUID `json:"content_id"`
	ContentType   string      `json:"content_type"`
	AIOutput      []byte      `json:"ai_output"`      // JSONB
	HumanFeedback []byte      `json:"human_feedback"` // JSONB, nullable
	Status        string      `json:"status"`
	ReviewerID    pgtype.UUID `json:"reviewer_id"`      // nullable
	Reason        string      `json:"reason,omitempty"` // nullable
	// Source identifies the origin of the review. Defaults to 'manual'
	// (migration 000395). 'agent_draft_request' marks drafts created by the
	// AI agent via the content.create_draft tool or the
	// agent:create_draft_request Asynq task; 'agent_tool' marks reviews
	// created by the executor for tools with requires_hitl=true.
	Source     string     `json:"source,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
	ReviewedAt *time.Time `json:"reviewed_at,omitempty"`
	// TargetTable identifies which table the reviewed record lives in.
	// Defaults to 'contents' for backward compatibility. When the review
	// targets a tool table draft (housing_guides, visa_routes, etc.), this
	// is set to the tool table name and ContentID holds the record's UUID.
	// Added by migration 000477 (ADR-076 Amendment 1, Approach B).
	TargetTable string `json:"target_table,omitempty"`
}

// HITL source constants (migration 000395 CHECK constraint).
const (
	HITLSourceManual            = "manual"
	HITLSourceAgentDraftRequest = "agent_draft_request"
	HITLSourceAgentTool         = "agent_tool"
	// HITLSourceStudio marks drafts created by the AI Content Studio (§75,
	// ADR-076) so the admin panel can show them in the "Studio" tab. The
	// value is permitted by the source CHECK constraint (migration 000420).
	HITLSourceStudio = "studio"
)

// CreateHITLReview inserts a new HITL review record and returns it.
// The Source field defaults to 'manual' when empty (the column's NOT NULL
// DEFAULT 'manual' handles this at the DB layer, but we set it explicitly
// so the returned struct is consistent).
func (s *Store) CreateHITLReview(ctx context.Context, review HITLReview) (HITLReview, error) {
	source := review.Source
	if source == "" {
		source = HITLSourceManual
	}

	targetTable := review.TargetTable
	if targetTable == "" {
		targetTable = "contents"
	}

	var result HITLReview

	err := s.write.QueryRow(ctx, `
		INSERT INTO ai_hitl_reviews
			(content_id, content_type, ai_output, status, source, target_table)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, content_id, content_type, ai_output, status, source, target_table, created_at
	`, review.ContentID, review.ContentType, review.AIOutput, review.Status, source, targetTable).Scan(&result.ID, &result.ContentID, &result.ContentType,
		&result.AIOutput, &result.Status, &result.Source, &result.TargetTable, &result.CreatedAt)
	if err != nil {
		return HITLReview{}, fmt.Errorf("creating HITL review: %w", err)
	}

	return result, nil
}

// ApproveHITLReview sets the review status to 'approved' with the reviewer identity.
// The filter prevents cross-tenant approve/reject (Section 09 multi-tenancy).
func (s *Store) ApproveHITLReview(ctx context.Context, reviewID, reviewerID pgtype.UUID) error {
	tag, err := s.write.Exec(ctx, `
		UPDATE ai_hitl_reviews
		SET status = 'approved', reviewer_id = $2, reviewed_at = now()
		WHERE id = $1 AND status = 'pending'
	`, reviewID, reviewerID)
	if err != nil {
		return fmt.Errorf("approving HITL review: %w", err)
	}

	if tag.RowsAffected() == 0 {
		return errors.New("not_found: review not found or already processed")
	}

	return nil
}

// RejectHITLReview sets the review status to 'rejected' with a reason.
// The filter prevents cross-tenant approve/reject (Section 09 multi-tenancy).
func (s *Store) RejectHITLReview(ctx context.Context, reviewID, reviewerID pgtype.UUID, reason string) error {
	tag, err := s.write.Exec(ctx, `
		UPDATE ai_hitl_reviews
		SET status = 'rejected', reviewer_id = $2, reviewed_at = now(), reason = $3
		WHERE id = $1 AND status = 'pending'
	`, reviewID, reviewerID, reason)
	if err != nil {
		return fmt.Errorf("rejecting HITL review: %w", err)
	}

	if tag.RowsAffected() == 0 {
		return errors.New("not_found: review not found or already processed")
	}

	return nil
}

// ListPendingHITLReviews returns pending reviews for a tenant with offset
// pagination. When sourceFilter is non-empty, only reviews with that source
// are returned (e.g. "studio" for the Studio tab, §75 Review Routing).
func (s *Store) ListPendingHITLReviews(ctx context.Context, sourceFilter string, limit, offset int32) ([]HITLReview, error) {
	if limit <= 0 {
		limit = 20
	}

	var (
		rows pgx.Rows
		err  error
	)

	if sourceFilter != "" {
		rows, err = s.read.Query(ctx, `
			SELECT id, content_id, content_type, ai_output, status, source, target_table, created_at
			FROM ai_hitl_reviews
			WHERE status = 'pending' AND source = $1
			ORDER BY created_at DESC
			LIMIT $2 OFFSET $3
		`, sourceFilter, limit, offset)
	} else {
		rows, err = s.read.Query(ctx, `
			SELECT id, content_id, content_type, ai_output, status, source, target_table, created_at
			FROM ai_hitl_reviews
			WHERE status = 'pending'
			ORDER BY created_at DESC
			LIMIT $1 OFFSET $2
		`, limit, offset)
	}

	if err != nil {
		return nil, fmt.Errorf("listing pending HITL reviews: %w", err)
	}

	defer rows.Close()

	var reviews []HITLReview

	for rows.Next() {
		var r HITLReview
		if err := rows.Scan(&r.ID, &r.ContentID, &r.ContentType,
			&r.AIOutput, &r.Status, &r.Source, &r.TargetTable, &r.CreatedAt); err != nil {
			return nil, fmt.Errorf("scanning HITL review: %w", err)
		}

		reviews = append(reviews, r)
	}

	return reviews, nil
}

// GetHITLReview returns a single HITL review by tenant + review ID. Used by
// the Studio review hook to discover target_table for tool-table drafts.
func (s *Store) GetHITLReview(ctx context.Context, reviewID pgtype.UUID) (HITLReview, error) {
	var r HITLReview

	err := s.read.QueryRow(ctx, `
		SELECT id, content_id, content_type, ai_output, status, source, target_table, created_at
		FROM ai_hitl_reviews
		WHERE id = $1`, reviewID).Scan(&r.ID, &r.ContentID, &r.ContentType,
		&r.AIOutput, &r.Status, &r.Source, &r.TargetTable, &r.CreatedAt)
	if err != nil {
		return HITLReview{}, fmt.Errorf("getting HITL review: %w", err)
	}

	return r, nil
}

// CountPendingHITLReviews returns the total count of pending reviews for a
// tenant (used for pagination metadata in the admin HITL queue). When
// sourceFilter is non-empty, only reviews with that source are counted.
func (s *Store) CountPendingHITLReviews(ctx context.Context, sourceFilter string) (int64, error) {
	var total int64
	if sourceFilter != "" {
		err := s.read.QueryRow(ctx, `
			SELECT COUNT(*) FROM ai_hitl_reviews
			WHERE status = 'pending' AND source = $1
		`, sourceFilter).Scan(&total)
		if err != nil {
			return 0, fmt.Errorf("counting pending HITL reviews: %w", err)
		}
	} else {
		err := s.read.QueryRow(ctx, `
			SELECT COUNT(*) FROM ai_hitl_reviews
			WHERE status = 'pending'
		`).Scan(&total)
		if err != nil {
			return 0, fmt.Errorf("counting pending HITL reviews: %w", err)
		}
	}

	return total, nil
}

// AIUsageSummary — ---------------------------------------------------------------------------
// AI Usage Log (Section 20 — admin AI usage monitoring)
// ---------------------------------------------------------------------------
// Matches the Section 20 contract: { total_cost_usd, tokens_used, by_task_type }.
// //nolint:revive // exported: stutter naming is intentional
type AIUsageSummary struct {
	TotalCostUSD float64             `json:"total_cost_usd"`
	TokensUsed   int64               `json:"tokens_used"`
	ByTaskType   []AIUsageByTaskType `json:"by_task_type"`
	ByProvider   []AIUsageByProvider `json:"by_provider,omitempty"`
}

// AIUsageByTaskType is the per-task-type cost/token breakdown.
// //nolint:revive // exported: stutter naming is intentional
type AIUsageByTaskType struct {
	TaskType     string  `json:"task_type"`
	TokensUsed   int64   `json:"tokens_used"`
	TotalCostUSD float64 `json:"total_cost_usd"`
	Calls        int64   `json:"calls"`
}

// AIUsageByProvider is the per-provider cost/token breakdown.
// //nolint:revive // exported: stutter naming is intentional
type AIUsageByProvider struct {
	Provider     string  `json:"provider"`
	TokensUsed   int64   `json:"tokens_used"`
	TotalCostUSD float64 `json:"total_cost_usd"`
	Calls        int64   `json:"calls"`
}

// GetAIUsageSummary returns aggregated AI usage for a tenant within the given
// period. periodDays <= 0 defaults to 30. An optional provider filter limits
// the summary to a single provider (empty string = all providers).
func (s *Store) GetAIUsageSummary(ctx context.Context, periodDays int, provider string) (AIUsageSummary, error) {
	if periodDays <= 0 {
		periodDays = 30
	}

	since := time.Now().AddDate(0, 0, -periodDays)

	// Totals (optionally filtered by provider).
	var summary AIUsageSummary

	totalsQuery := `
		SELECT COALESCE(SUM(total_tokens), 0), COALESCE(SUM(cost_usd), 0), COUNT(*)
		FROM ai_usage_log
		WHERE created_at >= $1
	`
	totalsArgs := []any{since}

	if provider != "" {
		totalsQuery += ` AND provider = $1`

		totalsArgs = append(totalsArgs, provider)
	}

	if err := s.read.QueryRow(ctx, totalsQuery, totalsArgs...).Scan(
		&summary.TokensUsed, &summary.TotalCostUSD, new(int64),
	); err != nil {
		return AIUsageSummary{}, fmt.Errorf("querying ai usage totals: %w", err)
	}

	// Per-task-type breakdown (always across all providers for the period).
	rows, err := s.read.Query(ctx, `
		SELECT task_type,
		       COALESCE(SUM(total_tokens), 0) AS tokens_used,
		       COALESCE(SUM(cost_usd), 0)     AS total_cost_usd,
		       COUNT(*)                       AS calls
		FROM ai_usage_log
		WHERE created_at >= $1
		GROUP BY task_type
		ORDER BY total_cost_usd DESC
	`, since)
	if err != nil {
		return AIUsageSummary{}, fmt.Errorf("querying ai usage by task type: %w", err)
	}

	defer rows.Close()

	for rows.Next() {
		var b AIUsageByTaskType
		if err := rows.Scan(&b.TaskType, &b.TokensUsed, &b.TotalCostUSD, &b.Calls); err != nil {
			return AIUsageSummary{}, fmt.Errorf("scanning ai usage by task type: %w", err)
		}

		summary.ByTaskType = append(summary.ByTaskType, b)
	}

	// Per-provider breakdown.
	prows, err := s.read.Query(ctx, `
		SELECT provider,
		       COALESCE(SUM(total_tokens), 0) AS tokens_used,
		       COALESCE(SUM(cost_usd), 0)     AS total_cost_usd,
		       COUNT(*)                       AS calls
		FROM ai_usage_log
		WHERE created_at >= $1
		GROUP BY provider
		ORDER BY total_cost_usd DESC
	`, since)
	if err != nil {
		return AIUsageSummary{}, fmt.Errorf("querying ai usage by provider: %w", err)
	}

	defer func() { prows.Close() }()

	for prows.Next() {
		var b AIUsageByProvider
		if err := prows.Scan(&b.Provider, &b.TokensUsed, &b.TotalCostUSD, &b.Calls); err != nil {
			return AIUsageSummary{}, fmt.Errorf("scanning ai usage by provider: %w", err)
		}

		summary.ByProvider = append(summary.ByProvider, b)
	}

	return summary, rows.Err()
}

// RecordAIUsage inserts a single ai_usage_log row. Called after every AI
// gateway dispatch so admin usage monitoring stays current.
func (s *Store) RecordAIUsage(ctx context.Context, userID pgtype.UUID, provider, model, taskType, requestRef string, promptTokens, completionTokens, totalTokens int32, costUSD float64) error {
	var userIDArg any
	if userID.Valid {
		userIDArg = userID
	} else {
		userIDArg = nil
	}

	_, err := s.write.Exec(ctx, `
		INSERT INTO ai_usage_log
			(user_id, provider, model, task_type,
			 prompt_tokens, completion_tokens, total_tokens, cost_usd, request_ref)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`, userIDArg, provider, model, taskType, promptTokens, completionTokens, totalTokens, costUSD, requestRef)
	if err != nil {
		return fmt.Errorf("recording ai usage: %w", err)
	}

	return nil
}

// RecordToolUsage inserts a single ai_tool_usage row. Called after every
// successful tool execution so admin tool-usage stats stay current (§20).
// tokensUsed and costCents are 0 for internal tools that don't make LLM
// calls — the row records that the tool was invoked.
func (s *Store) RecordToolUsage(ctx context.Context, toolID pgtype.UUID, userID *pgtype.UUID, tokensUsed, costCents int32) error {
	var userIDArg any
	if userID != nil && userID.Valid {
		userIDArg = *userID
	} else {
		userIDArg = nil
	}

	_, err := s.write.Exec(ctx, `
		INSERT INTO ai_tool_usage (tool_id, user_id, tokens_used, cost_cents) VALUES ($1, $2, $3, $4)
	`, toolID, userIDArg, tokensUsed, costCents)
	if err != nil {
		return fmt.Errorf("recording ai tool usage: %w", err)
	}

	return nil
}

// UserBelongsToTenant reports whether the given user id exists within the
// given tenant. Used by the admin AI-context handlers to prevent cross-tenant
// access to a user's Honcho memory (ADR-027, §9 multi-tenancy).
func (s *Store) UserBelongsToTenant(ctx context.Context, userID string) (bool, error) {
	var uid pgtype.UUID
	if err := uid.Scan(userID); err != nil {
		return false, fmt.Errorf("invalid user id %q: %w", userID, err)
	}

	var exists bool

	err := s.read.QueryRow(ctx, `
		SELECT EXISTS(SELECT 1 FROM users WHERE id = $1)
	`, uid).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("checking user tenant membership: %w", err)
	}

	return exists, nil
}
