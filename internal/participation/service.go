// Package participation implements the Participation Orchestration Engine
// (Section 76, ADR-077).
//
// The orchestration engine selects eligible content, generates action content
// via the AI gateway (RAG-grounded, §69), and dispatches actions as the
// persona's is_bot backing account — or routes them to the §22 review queue
// in queued mode. No new action endpoints are created; the engine reuses
// existing action endpoints.
//
// High-stakes gate: parent content in guide categories visa, banking, grants,
// health_insurance, or tagged immigration-advice/financial-advice always
// routes to content_reviews regardless of autonomy_mode. Hardcoded in task,
// not a setting.
//
// Ref: japaconnect-engineering/sections/76-participation-orchestration.md
//      japaconnect-engineering/adr/ADR-077-participation-orchestration.md
package participation

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/valkey-io/valkey-go/valkeycompat"

	"github.com/jedafe/japa/internal/config"
	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/personas"
)

// TaskEnqueuer is the subset of worker.Enqueuer that participation needs.
type TaskEnqueuer interface {
	Enqueue(taskType string, payload []byte, opts ...interface{}) error
}

// Service holds the business logic for the Participation Orchestration Engine.
type Service struct {
	db       *db.DB
	settings *config.Settings
	vk       valkeycompat.Cmdable
	enqueuer TaskEnqueuer
}

// NewService creates a Service backed by the given database, settings, and
// Valkey client.
func NewService(database *db.DB, settings *config.Settings, vk valkeycompat.Cmdable) *Service {
	return &Service{db: database, settings: settings, vk: vk}
}

// SetEnqueuer injects the task enqueuer (called at server wiring time).
func (s *Service) SetEnqueuer(e TaskEnqueuer) { s.enqueuer = e }

// highStakesCategories are the guide categories that always require human
// review, regardless of autonomy_mode. Hardcoded per §76 — not configurable.
var highStakesCategories = map[string]bool{
	"visa":            true,
	"banking":         true,
	"grants":          true,
	"health_insurance": true,
}

// highStakesTags are tags that trigger the high-stakes gate.
var highStakesTags = map[string]bool{
	"immigration-advice": true,
	"financial-advice":   true,
}

// IsHighStakesContent returns true if the parent content's guide category or
// tags require mandatory human review. This is the unconditional gate from
// §76 — it cannot be overridden by autonomy_mode or any setting.
//
// Exported so the personas package and studio package can reuse the same gate.
func IsHighStakesContent(guideCategory string, tags []string) bool {
	if highStakesCategories[strings.ToLower(guideCategory)] {
		return true
	}
	for _, tag := range tags {
		if highStakesTags[strings.ToLower(tag)] {
			return true
		}
	}
	return false
}

// ActivityPolicy is the per-persona policy that governs autonomous actions.
// Stored as jsonb in ai_personas.activity_policy.
type ActivityPolicy struct {
	ActionsPerDay        int            `json:"actions_per_day"`
	ActionBudget         map[string]int `json:"action_budget"`
	EligibleContentTypes []string       `json:"eligible_content_types"`
	EligibleSpaces       []pgtype.UUID  `json:"eligible_spaces"`
	MinContentAgeHours   int            `json:"min_content_age_hours"`
	AvoidOwnContent      bool           `json:"avoid_own_content"`
	AvoidRecentAuthors   []pgtype.UUID  `json:"avoid_recent_authors"`
	ActiveHoursUTC       [2]int         `json:"active_hours_utc"`
}

// DefaultActivityPolicy returns a sensible default policy.
func DefaultActivityPolicy() ActivityPolicy {
	return ActivityPolicy{
		ActionsPerDay:        10,
		ActionBudget:         map[string]int{"comment": 4, "forum_answer": 2, "forum_question": 1, "follow": 5, "react": 20, "bookmark": 10, "form_submit": 2},
		EligibleContentTypes: []string{"blog_post", "guide", "discussion", "forum_thread"},
		MinContentAgeHours:   1,
		AvoidOwnContent:      true,
		ActiveHoursUTC:       [2]int{8, 22},
	}
}

// RunActivity is the orchestration loop entry point, called by the
// persona:run_activity task (every 60 min). It loads the persona's
// activity_policy, checks Valkey daily counters, checks active hours,
// queries eligible content, and dispatches actions.
func (s *Service) RunActivity(ctx context.Context, personaID pgtype.UUID) error {
	if !s.isEnabled(ctx) {
		slog.Info("participation: run_activity: feature disabled, skipping")
		return nil
	}

	// Acquire run-lock to prevent concurrent runs (5 min TTL).
	if !s.acquireRunLock(ctx, personaID) {
		slog.Info("participation: run_activity: run-lock held, skipping", "persona_id", personaID)
		return nil
	}
	defer s.releaseRunLock(ctx, personaID)

	// Load persona.
	persona, err := s.loadPersona(ctx, personaID)
	if err != nil {
		return fmt.Errorf("participation: load persona: %w", err)
	}
	if persona.Status != "active" {
		slog.Info("participation: run_activity: persona not active, skipping", "persona_id", personaID, "status", persona.Status)
		return nil
	}

	// Parse activity policy.
	policy := DefaultActivityPolicy()
	if persona.ActivityPolicy != nil && len(persona.ActivityPolicy) > 0 {
		if err := json.Unmarshal(persona.ActivityPolicy, &policy); err != nil {
			slog.Warn("participation: run_activity: bad activity_policy, using default", "error", err)
		}
	}

	// Check active hours.
	if !s.isWithinActiveHours(policy.ActiveHoursUTC) {
		slog.Info("participation: run_activity: outside active hours, skipping", "persona_id", personaID)
		return nil
	}

	// Check daily counters.
	today := time.Now().UTC().Format("2006-01-02")
	totalToday := s.getDailyCounter(ctx, personaID, today, "total")
	var tenantUUID pgtype.UUID
	_ = tenantUUID.Scan(persona.TenantID)
	globalCap := s.getGlobalDailyCap(ctx, tenantUUID)
	globalToday := s.getGlobalDailyCounter(ctx, tenantUUID, today)
	if totalToday >= policy.ActionsPerDay {
		slog.Info("participation: run_activity: per-persona daily cap reached", "persona_id", personaID, "count", totalToday)
		return nil
	}
	if globalToday >= globalCap {
		slog.Info("participation: run_activity: global daily cap reached", "tenant_id", persona.TenantID, "count", globalToday)
		return nil
	}

	// Query eligible content.
	candidates, err := s.findEligibleContent(ctx, persona, policy)
	if err != nil {
		return fmt.Errorf("participation: find eligible content: %w", err)
	}
	slog.Info("participation: run_activity: found candidates", "persona_id", personaID, "count", len(candidates))

	// Dispatch actions up to the remaining budget.
	remaining := policy.ActionsPerDay - totalToday
	dispatched := 0
	for _, c := range candidates {
		if dispatched >= remaining {
			break
		}
		actionType := s.selectAction(ctx, personaID, today, policy.ActionBudget)
		if actionType == "" {
			break
		}

		highStakes := IsHighStakesContent(c.GuideCategory, c.Tags)
		if highStakes || persona.AutonomyMode == "queued" {
			// Route to content_reviews queue.
			if err := s.routeToReviewQueue(ctx, persona, c, actionType); err != nil {
				slog.Warn("participation: route to review failed", "content_id", c.ID, "error", err)
				continue
			}
		} else {
			// Autonomous dispatch — enqueue persona:dispatch_action task.
			if err := s.enqueueDispatchAction(ctx, personaID, c.ID, actionType); err != nil {
				slog.Warn("participation: enqueue dispatch failed", "content_id", c.ID, "error", err)
				continue
			}
		}

		s.incrementDailyCounter(ctx, personaID, today, actionType)
		s.incrementDailyCounter(ctx, personaID, today, "total")
		s.incrementGlobalCounter(ctx, tenantUUID, today)
		dispatched++
	}

	slog.Info("participation: run_activity: complete", "persona_id", personaID, "dispatched", dispatched)
	return nil
}

// DispatchAction executes a single approved queued persona action. Called by
// the persona:dispatch_action task.
func (s *Service) DispatchAction(ctx context.Context, actionID pgtype.UUID) error {
	if !s.isEnabled(ctx) {
		return nil
	}

	// Load the queued action from content_reviews.
	var (
		tenantID   pgtype.UUID
		contentID  pgtype.UUID
		reviewerID pgtype.UUID
		status     string
	)
	err := s.db.Reader.QueryRow(ctx,
		`SELECT tenant_id, content_id, reviewer_user_id, status
		 FROM content_reviews WHERE id = $1`, actionID,
	).Scan(&tenantID, &contentID, &reviewerID, &status)
	if err != nil {
		return fmt.Errorf("participation: load action: %w", err)
	}

	if status != "approved" {
		slog.Info("participation: dispatch_action: action not approved, skipping", "action_id", actionID, "status", status)
		return nil
	}

	// The action is approved — publish the content.
	_, err = s.db.Writer.Exec(ctx,
		`UPDATE contents SET status = 'published', published_at = now()
		 WHERE id = $1 AND tenant_id = $2`,
		contentID, tenantID)
	if err != nil {
		return fmt.Errorf("participation: publish content: %w", err)
	}

	// Mark the studio job or persona action as published.
	_, _ = s.db.Writer.Exec(ctx,
		`UPDATE studio_jobs SET status = 'published', published_at = now()
		 WHERE draft_content_id = $1 AND tenant_id = $2`,
		contentID, tenantID)

	slog.Info("participation: dispatch_action: published", "action_id", actionID, "content_id", contentID)
	return nil
}

// SweepActivePersonas queries all active personas across all tenants and
// enqueues a persona:run_activity task for each. Called by the periodic
// scheduler. The sweep interval is controlled by the
// participation.run_interval_minutes system_settings key (default 60 min)
// via a Valkey timestamp guard — this makes the interval hot-reloadable
// without restarting the scheduler.
// Ref: japaconnect-engineering/sections/76-participation-orchestration.md
func (s *Service) SweepActivePersonas(ctx context.Context) error {
	if !s.isEnabled(ctx) {
		slog.Info("participation: sweep: feature disabled, skipping")
		return nil
	}

	// Hot-reloadable interval guard via Valkey. The scheduler fires at a
	// fixed 60-min cron, but this check skips the sweep if the configured
	// interval hasn't elapsed since the last sweep.
	intervalMinutes := 60
	if s.settings != nil {
		intervalMinutes = s.settings.GetIntOr(ctx, pgtype.UUID{}, "participation.run_interval_minutes", 60)
	}
	if intervalMinutes <= 0 {
		intervalMinutes = 60
	}

	if s.vk != nil {
		lastSweepKey := "jc:persona:sweep:last_run"
		lastVal, err := s.vk.Get(ctx, lastSweepKey).Result()
		if err == nil {
			if lastTime, err := time.Parse(time.RFC3339, lastVal); err == nil {
				if time.Since(lastTime) < time.Duration(intervalMinutes)*time.Minute {
					slog.Debug("participation: sweep: interval not elapsed, skipping",
						"elapsed", time.Since(lastTime), "interval_min", intervalMinutes)
					return nil
				}
			}
		}
		// Record this sweep time with TTL = interval + buffer.
		_ = s.vk.Set(ctx, lastSweepKey, time.Now().UTC().Format(time.RFC3339),
			time.Duration(intervalMinutes+10)*time.Minute).Err()
	}

	if s.enqueuer == nil {
		slog.Warn("participation: sweep: no enqueuer configured, skipping")
		return nil
	}

	// Query all active personas across all tenants.
	rows, err := s.db.Reader.Query(ctx,
		`SELECT id FROM ai_personas WHERE status = 'active'`)
	if err != nil {
		return fmt.Errorf("participation: sweep: query active personas: %w", err)
	}
	defer rows.Close()

	count := 0
	for rows.Next() {
		var personaID pgtype.UUID
		if err := rows.Scan(&personaID); err != nil {
			slog.Warn("participation: sweep: scan persona id", "error", err)
			continue
		}
		payload, err := json.Marshal(map[string]string{"persona_id": personaID.String()})
		if err != nil {
			slog.Warn("participation: sweep: marshal payload", "error", err)
			continue
		}
		if err := s.enqueuer.Enqueue("persona:run_activity", payload); err != nil {
			slog.Warn("participation: sweep: enqueue failed", "persona_id", personaID, "error", err)
			continue
		}
		count++
	}

	slog.Info("participation: sweep: enqueued run_activity tasks", "count", count, "interval_min", intervalMinutes)
	return rows.Err()
}

// eligibleContent is a minimal content row for candidate selection.
type eligibleContent struct {
	ID             pgtype.UUID
	GuideCategory  string
	Tags           []string
	AuthorID       pgtype.UUID
	CreatedAt      time.Time
}

// loadPersona loads a persona by ID.
func (s *Service) loadPersona(ctx context.Context, personaID pgtype.UUID) (personas.PersonaDetail, error) {
	// Use raw SQL to avoid importing the personas store (which would create
	// a dependency cycle if personas ever imports participation).
	var p personas.PersonaDetail
	var (
		tenantID       pgtype.UUID
		userID         pgtype.UUID
		profileID      pgtype.UUID
		voicePrompt    *string
		activityPolicy []byte
		createdBy      pgtype.UUID
		createdAt      time.Time
		updatedAt      time.Time
	)
	err := s.db.Reader.QueryRow(ctx,
		`SELECT id, tenant_id, user_id, profile_id, disclosure_mode,
		        autonomy_mode, voice_prompt, activity_policy, status, created_by,
		        created_at, updated_at
		 FROM ai_personas WHERE id = $1`, personaID,
	).Scan(&p.ID, &tenantID, &userID, &profileID, &p.DisclosureMode,
		&p.AutonomyMode, &voicePrompt, &activityPolicy, &p.Status, &createdBy,
		&createdAt, &updatedAt)
	if err != nil {
		return p, err
	}
	p.TenantID = tenantID.String()
	p.UserID = userID.String()
	p.ProfileID = profileID.String()
	p.CreatedBy = createdBy.String()
	p.CreatedAt = createdAt.Format(time.RFC3339)
	p.UpdatedAt = updatedAt.Format(time.RFC3339)
	p.VoicePrompt = voicePrompt
	p.ActivityPolicy = activityPolicy
	return p, nil
}

// findEligibleContent queries content that the persona can act on.
// Results are cached in Valkey under jc:persona:eligible:{persona_id}:{hash}
// for 15 minutes (§76) to avoid re-running the candidate query on every
// run_activity cycle within the cache window.
func (s *Service) findEligibleContent(ctx context.Context, persona personas.PersonaDetail, policy ActivityPolicy) ([]eligibleContent, error) {
	var tenantID, userID pgtype.UUID
	_ = tenantID.Scan(persona.TenantID)
	_ = userID.Scan(persona.UserID)

	minAge := time.Now().Add(-time.Duration(policy.MinContentAgeHours) * time.Hour)

	// Build a cache key hash from the query parameters.
	cacheHash := buildEligibleCacheHash(persona.ID, minAge, policy.AvoidOwnContent, policy.EligibleContentTypes)
	cacheKey := fmt.Sprintf("jc:persona:eligible:%s:%s", persona.ID, cacheHash)

	// Try cache first.
	if s.vk != nil {
		if cached, err := s.vk.Get(ctx, cacheKey).Bytes(); err == nil && len(cached) > 0 {
			var cachedResults []eligibleContent
			if err := json.Unmarshal(cached, &cachedResults); err == nil {
				return cachedResults, nil
			}
		}
	}

	query := `SELECT c.id, c.guide_category, c.tags, c.author_user_id, c.created_at
	          FROM contents c
	          WHERE c.tenant_id = $1
	            AND c.status = 'published'
	            AND c.deleted_at IS NULL
	            AND c.created_at >= $2`
	args := []interface{}{tenantID, minAge}
	argIdx := 3

	if policy.AvoidOwnContent {
		query += ` AND c.author_user_id != $` + strconv.Itoa(argIdx)
		args = append(args, userID)
		argIdx++
	}

	if len(policy.EligibleContentTypes) > 0 {
		query += ` AND c.content_type = ANY($` + strconv.Itoa(argIdx) + `)`
		args = append(args, policy.EligibleContentTypes)
		argIdx++
	}

	query += ` ORDER BY c.created_at DESC LIMIT 50`

	rows, err := s.db.Reader.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []eligibleContent
	for rows.Next() {
		var c eligibleContent
		if err := rows.Scan(&c.ID, &c.GuideCategory, &c.Tags, &c.AuthorID, &c.CreatedAt); err != nil {
			return nil, err
		}
		results = append(results, c)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Cache the results for 15 minutes (§76).
	if s.vk != nil && len(results) > 0 {
		if data, err := json.Marshal(results); err == nil {
			_ = s.vk.Set(ctx, cacheKey, data, 15*time.Minute).Err()
		}
	}

	return results, nil
}

// buildEligibleCacheHash computes a deterministic hash of the eligible-content
// query parameters for the jc:persona:eligible:{persona_id}:{hash} cache key.
func buildEligibleCacheHash(personaID string, minAge time.Time, avoidOwn bool, contentTypes []string) string {
	h := sha256.New()
	h.Write([]byte(personaID))
	h.Write([]byte(minAge.Format(time.RFC3339)))
	if avoidOwn {
		h.Write([]byte("avoid"))
	}
	for _, ct := range contentTypes {
		h.Write([]byte(ct))
	}
	return hex.EncodeToString(h.Sum(nil))[:16]
}

// selectAction picks an action type that hasn't exhausted its budget.
func (s *Service) selectAction(ctx context.Context, personaID pgtype.UUID, today string, budget map[string]int) string {
	for action, max := range budget {
		used := s.getDailyCounter(ctx, personaID, today, action)
		if used < max {
			return action
		}
	}
	return ""
}

// routeToReviewQueue creates a content_reviews entry for the persona action.
func (s *Service) routeToReviewQueue(ctx context.Context, persona personas.PersonaDetail, content eligibleContent, actionType string) error {
	var tenantID, userID, profileID, createdBy pgtype.UUID
	_ = tenantID.Scan(persona.TenantID)
	_ = userID.Scan(persona.UserID)
	_ = profileID.Scan(persona.ProfileID)
	_ = createdBy.Scan(persona.CreatedBy)

	// Create a draft content for the persona action. The persona's backing
	// user account has is_bot=true (added in migration 000246), so the
	// content is attributed to the persona's user_id.
	var contentID pgtype.UUID
	err := s.db.Writer.QueryRow(ctx,
		`INSERT INTO contents (tenant_id, author_user_id, author_profile_id, content_type,
		                       body, status, created_at)
		 VALUES ($1, $2, $3, $4, $5, 'draft', now())
		 RETURNING id`,
		tenantID, userID, profileID, actionType,
		`{"type":"doc"}`,
	).Scan(&contentID)
	if err != nil {
		return fmt.Errorf("create draft: %w", err)
	}

	// Create the review entry.
	_, err = s.db.Writer.Exec(ctx,
		`INSERT INTO content_reviews (tenant_id, content_id, reviewer_user_id, status)
		 VALUES ($1, $2, $3, 'pending')`,
		tenantID, contentID, createdBy)
	if err != nil {
		return fmt.Errorf("create review: %w", err)
	}
	return nil
}

// enqueueDispatchAction enqueues a persona:dispatch_action task.
func (s *Service) enqueueDispatchAction(ctx context.Context, personaID, contentID pgtype.UUID, actionType string) error {
	if s.enqueuer == nil {
		return fmt.Errorf("no enqueuer configured")
	}
	payload, _ := json.Marshal(map[string]string{
		"persona_id":  personaID.String(),
		"content_id":  contentID.String(),
		"action_type": actionType,
	})
	return s.enqueuer.Enqueue("persona:dispatch_action", payload)
}

// --- Valkey helpers ---

func (s *Service) acquireRunLock(ctx context.Context, personaID pgtype.UUID) bool {
	if s.vk == nil {
		return true
	}
	key := "jc:persona:lock:" + personaID.String()
	ok, err := s.vk.SetNX(ctx, key, "1", 5*time.Minute).Result()
	if err != nil {
		slog.Warn("participation: acquire run-lock failed", "error", err)
		return true // fail open
	}
	return ok
}

func (s *Service) releaseRunLock(ctx context.Context, personaID pgtype.UUID) {
	if s.vk == nil {
		return
	}
	_ = s.vk.Del(ctx, "jc:persona:lock:"+personaID.String()).Err()
}

func (s *Service) getDailyCounter(ctx context.Context, personaID pgtype.UUID, date, actionType string) int {
	if s.vk == nil {
		return 0
	}
	key := fmt.Sprintf("jc:persona:daily:%s:%s:%s", personaID.String(), date, actionType)
	val, err := s.vk.Get(ctx, key).Int()
	if err != nil {
		return 0
	}
	return val
}

func (s *Service) incrementDailyCounter(ctx context.Context, personaID pgtype.UUID, date, actionType string) {
	if s.vk == nil {
		return
	}
	key := fmt.Sprintf("jc:persona:daily:%s:%s:%s", personaID.String(), date, actionType)
	pipe := s.vk.Pipeline()
	pipe.Incr(ctx, key)
	pipe.ExpireAt(ctx, key, nextMidnightUTC())
	_, _ = pipe.Exec(ctx)
}

func (s *Service) getGlobalDailyCounter(ctx context.Context, tenantID pgtype.UUID, date string) int {
	if s.vk == nil {
		return 0
	}
	key := fmt.Sprintf("jc:persona:global:%s:%s", tenantID.String(), date)
	val, err := s.vk.Get(ctx, key).Int()
	if err != nil {
		return 0
	}
	return val
}

func (s *Service) incrementGlobalCounter(ctx context.Context, tenantID pgtype.UUID, date string) {
	if s.vk == nil {
		return
	}
	key := fmt.Sprintf("jc:persona:global:%s:%s", tenantID.String(), date)
	pipe := s.vk.Pipeline()
	pipe.Incr(ctx, key)
	pipe.ExpireAt(ctx, key, nextMidnightUTC())
	_, _ = pipe.Exec(ctx)
}

func (s *Service) getGlobalDailyCap(ctx context.Context, tenantID pgtype.UUID) int {
	if s.settings == nil {
		return 500
	}
	return s.settings.GetIntOr(ctx, tenantID, "participation.global_daily_cap", 500)
}

func (s *Service) isEnabled(ctx context.Context) bool {
	if s.settings == nil {
		return true
	}
	// Use the default tenant for the global check.
	return s.settings.GetBoolOr(ctx, pgtype.UUID{}, "participation.enabled", true)
}

func (s *Service) isWithinActiveHours(hours [2]int) bool {
	if hours[0] == 0 && hours[1] == 0 {
		return true
	}
	hour := time.Now().UTC().Hour()
	return hour >= hours[0] && hour < hours[1]
}

func nextMidnightUTC() time.Time {
	now := time.Now().UTC()
	return time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 0, time.UTC)
}
