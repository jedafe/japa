// Package participation implements the Participation Orchestration Engine
// (Section 76, ADR-077).
// // The orchestration engine selects eligible content, generates action content
// via the AI gateway (RAG-grounded, §69), and dispatches actions as the
// persona's is_bot backing account — or routes them to the §22 review queue
// in queued mode. No new action endpoints are created; the engine reuses
// existing action endpoints.
// // High-stakes gate: parent content in guide categories visa, banking, grants,
// health_insurance, or tagged immigration-advice/financial-advice always
// routes to content_reviews regardless of autonomy_mode. Hardcoded in task,
// not a setting.
// // Ref: japaconnect-engineering/sections/76-participation-orchestration.md
// //	japaconnect-engineering/adr/ADR-077-participation-orchestration.md
package participation

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/valkey-io/valkey-go/valkeycompat"

	"github.com/jedafe/japa/internal/cdn"
	"github.com/jedafe/japa/internal/config"
	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/personas"
	"github.com/jedafe/japa/internal/valkey"
)

// TaskEnqueuer is the subset of worker.Enqueuer that participation needs.
type TaskEnqueuer interface {
	Enqueue(taskType string, payload []byte, opts ...interface{}) error
}

// EventTracker is the subset of the analytics service that participation needs
// to record persona-originated platform events with is_synthetic = true
// (ADR-068 §4 / P10 — metrics integrity).
type EventTracker interface {
	TrackSyntheticEvent(ctx context.Context, userID pgtype.UUID, eventType, entityType string, entityID pgtype.UUID, meta map[string]any) error
}

// ActionDispatcher is the dependency-inverted interface for dispatching
// approved persona actions to the existing action endpoints (ADR-077 §5).
// The participation package does NOT import the comments, follows, content,
// forum, or customfields packages — instead the server wiring injects a
// concrete adapter that calls those services directly. This keeps the
// participation dependency graph narrow (BUG-010).
// // Each method is called as the persona's backing user (is_bot=true) and
// receives the platform ID, the persona's backing user_id, and the action
// payload. The methods return the created entity ID (when applicable) and
// an error. Implementations must platform-scope every write.
type ActionDispatcher interface {
	// CreateComment posts a comment on targetContentID as the persona.
	CreateComment(ctx context.Context, personaUserID, targetContentID pgtype.UUID, body []byte) (pgtype.UUID, error)
	// CreateForumThread creates a new forum thread (forum_question action).
	// contentID is the published contents row that backs the thread's root
	// post; the forum thread links to it via forum_threads.content_id
	// (BUG-004 — previously the thread was created with content_id=NULL,
	// leaving an orphan published content row).
	CreateForumThread(ctx context.Context, personaUserID, contentID, spaceID pgtype.UUID, title, slug string, body []byte) (pgtype.UUID, error)
	// CreateForumReply posts a reply to a forum thread (forum_answer action).
	CreateForumReply(ctx context.Context, personaUserID, threadID pgtype.UUID, body []byte) (pgtype.UUID, error)
	// ToggleFollow follows the target entity as the persona.
	ToggleFollow(ctx context.Context, personaUserID pgtype.UUID, entityType string, entityID pgtype.UUID) error
	// AddReaction adds a reaction to the target entity as the persona.
	AddReaction(ctx context.Context, personaUserID pgtype.UUID, entityType string, entityID pgtype.UUID, reactionKey string) error
	// CreateBookmark bookmarks the target entity as the persona.
	CreateBookmark(ctx context.Context, personaUserID pgtype.UUID, entityType string, entityID pgtype.UUID) error
	// SubmitForm submits a form as the persona.
	SubmitForm(ctx context.Context, personaUserID, formID pgtype.UUID, fieldValues map[string]any) (pgtype.UUID, error)
}

// FeatureChecker resolves feature flag state from the feature_flags table —
// the single source of truth for feature reachability.
type FeatureChecker interface {
	IsEnabled(ctx context.Context, key string) bool
}

// Service holds the business logic for the Participation Orchestration Engine.
type Service struct {
	db         *db.DB
	settings   *config.Settings
	checker    FeatureChecker
	vk         valkeycompat.Cmdable
	enqueuer   TaskEnqueuer
	tracker    EventTracker
	dispatcher ActionDispatcher
}

// NewService creates a Service backed by the given database, settings, and
// Valkey client.
func NewService(database *db.DB, settings *config.Settings, vk valkeycompat.Cmdable) *Service {
	return &Service{db: database, settings: settings, vk: vk}
}

// SetEnqueuer injects the task enqueuer (called at server wiring time).
func (s *Service) SetEnqueuer(e TaskEnqueuer) { s.enqueuer = e }

// SetEventTracker injects the analytics event tracker (called at server wiring
// time). When set, every dispatched persona action is recorded as a
// platform_event with is_synthetic = true so the activity log works and
// real-metric materialized views exclude persona activity (ADR-068 §4).
func (s *Service) SetEventTracker(t EventTracker) { s.tracker = t }

// SetDispatcher injects the action dispatcher used to route approved persona
// actions to the existing action endpoints (ADR-077 §5, BUG-010). When unset,
// only content-type actions (forum_question, blog_post) are dispatched via
// the publish-content path; non-content actions (follow, react, bookmark,
// comment, forum_answer, form_submit) are skipped with a warning.
func (s *Service) SetDispatcher(d ActionDispatcher) { s.dispatcher = d }

// highStakesCategories are the guide categories that always require human
// review, regardless of autonomy_mode. Hardcoded per §76 — not configurable.
var highStakesCategories = map[string]bool{
	"visa":             true,
	"banking":          true,
	"grants":           true,
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
// // Exported so the personas package and studio package can reuse the same gate.
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
// // EligibleSpaces and AvoidRecentAuthors use []string (space slugs / user
// slugs) to match the personas package's ActivityPolicy type (BUG-006).
// The §68 spec defines eligible_spaces as a string array; using pgtype.UUID
// here caused a JSON unmarshal mismatch when the participation engine read
// a policy written by the personas service.
type ActivityPolicy struct {
	ActionsPerDay        int            `json:"actions_per_day"`
	ActionBudget         map[string]int `json:"action_budget"`
	EligibleContentTypes []string       `json:"eligible_content_types"`
	EligibleSpaces       []string       `json:"eligible_spaces"`
	MinContentAgeHours   int            `json:"min_content_age_hours"`
	AvoidOwnContent      bool           `json:"avoid_own_content"`
	AvoidRecentAuthors   []string       `json:"avoid_recent_authors"`
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

	// Parse activity policy. Start with code-level defaults, then adopt the
	// stored policy only if it has a non-zero ActionsPerDay. The DB default
	// for activity_policy is '{}' (migration 000246); unmarshaling '{}' into
	// the struct zeroes all fields, overwriting the defaults and leaving
	// ActionsPerDay=0 — the persona would never act (BUG-003).
	policy := DefaultActivityPolicy()

	if persona.ActivityPolicy != nil && len(persona.ActivityPolicy) > 0 {
		var stored ActivityPolicy
		if err := json.Unmarshal(persona.ActivityPolicy, &stored); err != nil {
			slog.Warn("participation: run_activity: bad activity_policy, using default", "error", err)
		} else if stored.ActionsPerDay > 0 {
			policy = stored
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

	globalCap := s.getGlobalDailyCap(ctx)
	globalToday := s.getGlobalDailyCounter(ctx, today)

	if totalToday >= policy.ActionsPerDay {
		slog.Info("participation: run_activity: per-persona daily cap reached", "persona_id", personaID, "count", totalToday)
		return nil
	}

	if globalToday >= globalCap {
		slog.Info("participation: run_activity: global daily cap reached", "count", globalToday)
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
			if err := s.enqueueDispatchAction(ctx, persona, c, actionType); err != nil {
				slog.Warn("participation: enqueue dispatch failed", "content_id", c.ID, "error", err)
				continue
			}
		}

		s.incrementDailyCounter(ctx, personaID, today, actionType)
		s.incrementDailyCounter(ctx, personaID, today, "total")
		s.incrementGlobalCounter(ctx, today)

		dispatched++
	}

	slog.Info("participation: run_activity: complete", "persona_id", personaID, "dispatched", dispatched)

	return nil
}

// DispatchAction executes a single approved queued persona action. Called by
// the persona:dispatch_action task. It reads the action payload from
// content_reviews.persona_payload and routes the action to the correct existing
// endpoint via the ActionDispatcher (ADR-077 §5, BUG-010).
// // Content-type actions (forum_question, blog_post, discussion) are published
// directly via the contents table. Non-content actions (comment, forum_answer,
// follow, react, bookmark, form_submit) are dispatched through the existing
// action services via the injected ActionDispatcher.
func (s *Service) DispatchAction(ctx context.Context, actionID pgtype.UUID) error {
	if !s.isEnabled(ctx) {
		return nil
	}

	// Load the queued action from content_reviews + the content's author
	// (the persona's backing user_id) + the action payload from
	// persona_payload (migration 000482, BUG-003). COALESCE falls back to
	// feedback for legacy rows queued before the persona_payload column
	// existed (the 000482 backfill copies them, but this guard covers any
	// row that escaped the backfill).
	var (
		contentID  pgtype.UUID
		reviewerID pgtype.UUID
		status     string
		authorID   pgtype.UUID
		payload    []byte
	)

	err := s.db.Reader.QueryRow(ctx, `SELECT cr.content_id, cr.reviewer_user_id, cr.status,
		        c.author_id, COALESCE(cr.persona_payload::text, cr.feedback)::bytea
		 FROM content_reviews cr
		 JOIN contents c ON c.id = cr.content_id
		 WHERE cr.id = $1 AND cr.source = 'persona_action'`, actionID).Scan(&contentID, &reviewerID, &status, &authorID, &payload)
	if err != nil {
		return fmt.Errorf("participation: load action: %w", err)
	}

	if status != "approved" {
		slog.Info("participation: dispatch_action: action not approved, skipping", "action_id", actionID, "status", status)
		return nil
	}

	// Parse the action payload to determine the routing.
	var p actionPayload
	if len(payload) > 0 {
		if err := json.Unmarshal(payload, &p); err != nil {
			return fmt.Errorf("participation: parse action payload: %w", err)
		}
	}

	if p.ActionType == "" {
		// Legacy row queued before BUG-010 fix — default to content publish.
		p.ActionType = "blog_post"
	}

	// Route the action. Content-type actions publish the draft content row
	// directly; non-content actions call the existing action endpoints via
	// the dispatcher (ADR-077 §5, BUG-010).
	var dispatchedEntityID pgtype.UUID

	// ADR-134: the direct UPDATE below bypasses content.Service, so the
	// edge purge fires here — clears any cached 404/draft response at the
	// content URL the moment the orchestration publishes it.
	publishContent := func(id pgtype.UUID) error {
		var slug, cType string
		err := s.db.Writer.QueryRow(ctx, `UPDATE contents SET status = 'published', published_at = now()
			 WHERE id = $1 RETURNING slug, content_type`, id).Scan(&slug, &cType)
		if err != nil {
			return err
		}

		cdn.PurgeContentDetail(ctx, id.String(), cType, slug)

		return nil
	}

	switch p.ActionType {
	case "blog_post", "discussion", "guide", "forum_thread":
		// Content-type action: publish the draft content row.
		if err := publishContent(contentID); err != nil {
			return fmt.Errorf("participation: publish content: %w", err)
		}

		dispatchedEntityID = contentID
	case "forum_question":
		// forum_question creates a new forum thread in the target space.
		// The draft content row is the thread's root content; publish it
		// first, then create the forum thread linked to that content via
		// forum_threads.content_id (BUG-004 — previously the thread was
		// created with content_id=NULL, leaving an orphan content row).
		if err := publishContent(contentID); err != nil {
			return fmt.Errorf("participation: publish forum thread content: %w", err)
		}

		dispatchedEntityID = contentID

		if s.dispatcher != nil {
			var spaceID pgtype.UUID
			_ = spaceID.Scan(p.TargetSpaceID)
			// Derive a title and slug from the body. In production the
			// AI gateway generates a proper title; here we use a fallback.
			title := "Persona Discussion"
			slug := fmt.Sprintf("persona-%d", time.Now().UnixNano())

			threadID, derr := s.dispatcher.CreateForumThread(ctx, authorID, contentID, spaceID, title, slug, p.Body)
			if derr != nil {
				slog.Error("participation: dispatch forum_question", "action_id", actionID, "error", derr)
			} else {
				dispatchedEntityID = threadID
			}
		}
	default:
		// Non-content action: route through the dispatcher (BUG-010).
		if s.dispatcher == nil {
			slog.Warn("participation: dispatch_action: no dispatcher configured, skipping non-content action", "action_id", actionID, "action_type", p.ActionType)
			return nil
		}

		dispErr := s.dispatchNonContentAction(ctx, p, authorID)
		if dispErr != nil {
			slog.Error("participation: dispatch_action: non-content action failed", "action_id", actionID, "action_type", p.ActionType, "error", dispErr)
			return dispErr
		}

		dispatchedEntityID = contentID
	}

	// Record a synthetic platform_event so the persona activity log works
	// and real-metric materialized views exclude this action (ADR-068 §4, P10).
	s.trackPersonaEvent(ctx, authorID, "persona.action_dispatched", "content", dispatchedEntityID, map[string]any{
		"action_id":   actionID.String(),
		"reviewer_id": reviewerID.String(),
		"action_type": p.ActionType,
	})

	slog.Info("participation: dispatch_action: complete", "action_id", actionID, "action_type", p.ActionType, "content_id", contentID)

	return nil
}

// dispatchNonContentAction routes a non-content persona action to the correct
// existing action endpoint via the ActionDispatcher (ADR-077 §5, BUG-010).
func (s *Service) dispatchNonContentAction(ctx context.Context, payload actionPayload, personaUserID pgtype.UUID) error {
	var (
		targetContentID pgtype.UUID
		targetEntityID  pgtype.UUID
		targetThreadID  pgtype.UUID
		targetFormID    pgtype.UUID
	)

	_ = targetContentID.Scan(payload.TargetContentID)
	_ = targetEntityID.Scan(payload.TargetEntityID)
	_ = targetThreadID.Scan(payload.TargetThreadID)
	_ = targetFormID.Scan(payload.TargetFormID)

	switch payload.ActionType {
	case "comment":
		_, err := s.dispatcher.CreateComment(ctx, personaUserID, targetContentID, payload.Body)
		return err
	case "forum_answer":
		// Look up the forum thread by the target content's content_id.
		var threadID pgtype.UUID

		err := s.db.Reader.QueryRow(ctx, `SELECT id FROM forum_threads WHERE content_id = $1`, targetContentID).Scan(&threadID)
		if err != nil {
			return fmt.Errorf("participation: find forum thread for answer: %w", err)
		}

		_, err = s.dispatcher.CreateForumReply(ctx, personaUserID, threadID, payload.Body)

		return err
	case "follow":
		return s.dispatcher.ToggleFollow(ctx, personaUserID, payload.TargetEntityType, targetEntityID)
	case "react":
		return s.dispatcher.AddReaction(ctx, personaUserID, payload.TargetEntityType, targetEntityID, payload.ReactionKey)
	case "bookmark":
		return s.dispatcher.CreateBookmark(ctx, personaUserID, payload.TargetEntityType, targetEntityID)
	case "form_submit":
		var fieldValues map[string]any
		if len(payload.Body) > 0 {
			_ = json.Unmarshal(payload.Body, &fieldValues)
		}

		_, err := s.dispatcher.SubmitForm(ctx, personaUserID, targetFormID, fieldValues)

		return err
	default:
		return fmt.Errorf("participation: unknown action type %q", payload.ActionType)
	}
}

// SweepActivePersonas queries all active personas across all platforms and
// enqueues a persona:run_activity task for each. Called by the periodic
// scheduler. The sweep interval is controlled by the
// participation.run_interval_minutes system_settings key (default 60 min)
// via a Valkey timestamp guard — this makes the interval hot-reloadable
// without restarting the scheduler.
// Ref: japaconnect-engineering/sections/76-participation-orchestration.md.
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
		intervalMinutes = s.settings.GetIntOr(ctx, "participation.run_interval_minutes", 60)
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
					slog.Debug("participation: sweep: interval not elapsed, skipping", "elapsed", time.Since(lastTime), "interval_min", intervalMinutes)
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

	// Query all active personas across all platforms.
	rows, err := s.db.Reader.Query(ctx, `SELECT id FROM ai_personas WHERE status = 'active'`)
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
	ID            pgtype.UUID
	SpaceID       pgtype.UUID
	GuideCategory string
	Tags          []string
	AuthorID      pgtype.UUID
	CreatedAt     time.Time
}

// loadPersona loads a persona by ID.
func (s *Service) loadPersona(ctx context.Context, personaID pgtype.UUID) (personas.PersonaDetail, error) {
	// Use raw SQL to avoid importing the personas store (which would create
	// a dependency cycle if personas ever imports participation).
	var p personas.PersonaDetail

	var (
		userID         pgtype.UUID
		profileID      pgtype.UUID
		voicePrompt    *string
		activityPolicy []byte
		createdBy      pgtype.UUID
		createdAt      time.Time
		updatedAt      time.Time
	)

	err := s.db.Reader.QueryRow(ctx, `SELECT id, user_id, profile_id, disclosure_mode,
		        autonomy_mode, voice_prompt, activity_policy, status, created_by,
		        created_at, updated_at
		 FROM ai_personas WHERE id = $1`, personaID).Scan(&p.ID, &userID, &profileID, &p.DisclosureMode,
		&p.AutonomyMode, &voicePrompt, &activityPolicy, &p.Status, &createdBy,
		&createdAt, &updatedAt)
	if err != nil {
		return p, err
	}

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
	var userID pgtype.UUID
	_ = userID.Scan(persona.UserID)

	minAge := time.Now().Add(-time.Duration(policy.MinContentAgeHours) * time.Hour)

	// Build a cache key hash from the query parameters.
	cacheHash := buildEligibleCacheHash(persona.ID, minAge, policy.AvoidOwnContent, policy.EligibleContentTypes, policy.EligibleSpaces, policy.AvoidRecentAuthors)
	cacheKey := valkey.MustKey("persona:eligible:%s:%s", persona.ID, cacheHash)

	// Try cache first.
	if s.vk != nil {
		if cached, err := s.vk.Get(ctx, cacheKey).Bytes(); err == nil && len(cached) > 0 {
			var cachedResults []eligibleContent
			if err := json.Unmarshal(cached, &cachedResults); err == nil {
				return cachedResults, nil
			}
		}
	}

	query := `SELECT c.id, c.space_id, COALESCE(c.guide_category, '') AS guide_category,
	          COALESCE(
	              (SELECT array_agg(t.name) FROM content_tags ct
	               JOIN tags t ON t.id = ct.tag_id
	               WHERE ct.content_id = c.id ),
	              '{}'
	          ) AS tags,
	          c.author_id, c.created_at
	          FROM contents c
	          WHERE c.status = 'published'
	            AND c.deleted_at IS NULL
	            AND c.created_at >= $1`
	args := []interface{}{minAge}
	argIdx := 2

	if policy.AvoidOwnContent {
		query += ` AND c.author_id != $` + strconv.Itoa(argIdx)

		args = append(args, userID)
		argIdx++
	}

	if len(policy.EligibleContentTypes) > 0 {
		query += ` AND c.content_type = ANY($` + strconv.Itoa(argIdx) + `)`

		args = append(args, policy.EligibleContentTypes)
		argIdx++
	}

	// eligible_spaces: empty = all spaces. When set, restrict to contents
	// whose space slug is in the list (BUG-007 — previously ignored).
	if len(policy.EligibleSpaces) > 0 {
		query += ` AND EXISTS (SELECT 1 FROM spaces sp WHERE sp.id = c.space_id AND sp.slug = ANY($` + strconv.Itoa(argIdx) + `))`

		args = append(args, policy.EligibleSpaces)
		argIdx++
	}

	// avoid_recent_authors: list of usernames to never engage with
	// (BUG-007 — previously ignored). users.username is the identifier.
	if len(policy.AvoidRecentAuthors) > 0 {
		query += ` AND NOT EXISTS (SELECT 1 FROM users au WHERE au.id = c.author_id AND au.username = ANY($` + strconv.Itoa(argIdx) + `))`

		args = append(args, policy.AvoidRecentAuthors)
		argIdx++
	}

	// §76 / ADR-077 §2: score by recency and engagement gap — prefer content
	// with fewer comments/reactions (adds value where missing). Order by
	// engagement ascending, then recency descending (BUG-006 — previously
	// only ordered by created_at DESC, always targeting the newest content).
	query += ` ORDER BY (
		(SELECT count(*) FROM comments cm WHERE cm.content_id = c.id AND cm.deleted_at IS NULL) +
		(SELECT count(*) FROM reactions r WHERE r.entity_type = 'content' AND r.entity_id = c.id)
	) ASC, c.created_at DESC LIMIT 50`

	rows, err := s.db.Reader.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []eligibleContent

	for rows.Next() {
		var c eligibleContent
		if err := rows.Scan(&c.ID, &c.SpaceID, &c.GuideCategory, &c.Tags, &c.AuthorID, &c.CreatedAt); err != nil {
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
func buildEligibleCacheHash(personaID string, minAge time.Time, avoidOwn bool, contentTypes, eligibleSpaces, avoidAuthors []string) string {
	h := sha256.New()
	h.Write([]byte(personaID))
	h.Write([]byte(minAge.Format(time.RFC3339)))

	if avoidOwn {
		h.Write([]byte("avoid"))
	}

	for _, ct := range contentTypes {
		h.Write([]byte(ct))
	}

	for _, sp := range eligibleSpaces {
		h.Write([]byte("sp:"))
		h.Write([]byte(sp))
	}

	for _, au := range avoidAuthors {
		h.Write([]byte("au:"))
		h.Write([]byte(au))
	}

	return hex.EncodeToString(h.Sum(nil))[:16]
}

// actionPriority is the deterministic order in which action types are
// considered when selecting the next action within budget. Iterating a map
// directly yields non-deterministic order (BUG-011); this fixed order makes
// selection reproducible per run and biases toward higher-value actions
// (comment, forum_answer) before lightweight ones (react, bookmark, follow).
var actionPriority = []string{
	"comment",
	"forum_answer",
	"forum_question",
	"follow",
	"react",
	"bookmark",
	"form_submit",
}

// selectAction picks an action type that hasn't exhausted its budget. It
// iterates the action types in a deterministic priority order (BUG-011)
// rather than over the budget map, so the same run yields the same choice.
func (s *Service) selectAction(ctx context.Context, personaID pgtype.UUID, today string, budget map[string]int) string {
	for _, action := range actionPriority {
		max, ok := budget[action]
		if !ok {
			continue
		}

		used := s.getDailyCounter(ctx, personaID, today, action)
		if used < max {
			return action
		}
	}

	return ""
}

// actionPayload is the JSON blob stored in content_reviews.persona_payload when a
// persona action is queued. It carries everything DispatchAction needs to
// route the action to the correct existing endpoint (ADR-077 §5, BUG-010):
// - action_type: the orchestration verb (comment, follow, react, ...)
// - target_content_id: the content the persona is engaging with
// - target_space_id: the space for forum_question thread creation
// - target_thread_id: the forum thread for forum_answer replies
// - target_entity_type: the polymorphic entity type for follow/react/bookmark
// - target_entity_id: the polymorphic entity ID for follow/react/bookmark
// - target_form_id: the form ID for form_submit
// - reaction_key: the reaction key for react
// - body: the AI-generated TipTap JSON body for content-bearing actions.
type actionPayload struct {
	ActionType       string          `json:"action_type"`
	TargetContentID  string          `json:"target_content_id,omitempty"`
	TargetSpaceID    string          `json:"target_space_id,omitempty"`
	TargetThreadID   string          `json:"target_thread_id,omitempty"`
	TargetEntityType string          `json:"target_entity_type,omitempty"`
	TargetEntityID   string          `json:"target_entity_id,omitempty"`
	TargetFormID     string          `json:"target_form_id,omitempty"`
	ReactionKey      string          `json:"reaction_key,omitempty"`
	Body             json.RawMessage `json:"body,omitempty"`
}

// createPersonaAction creates the draft contents row and the content_reviews
// row that back a persona action, returning the review and content IDs plus
// the action payload that was stored. status is either "pending" (queued for
// human approval) or "approved" (autonomous dispatch).
func (s *Service) createPersonaAction(ctx context.Context, persona personas.PersonaDetail, content eligibleContent, actionType, status string) (reviewID, contentID pgtype.UUID, payload actionPayload, err error) {
	var userID, profileID pgtype.UUID
	_ = userID.Scan(persona.UserID)
	_ = profileID.Scan(persona.ProfileID)

	// Build the action payload that DispatchAction will read back after
	// approval (BUG-010, ADR-077 §5). This captures the full action context
	// at queue time so the dispatcher can route to the correct existing
	// endpoint without re-deriving the target.
	payload = actionPayload{
		ActionType: actionType,
		Body:       json.RawMessage(`{"type":"doc"}`), // AI-generated body placeholder
	}

	switch actionType {
	case "comment":
		payload.TargetContentID = content.ID.String()
	case "forum_answer":
		// The eligible content is the thread's root content; the dispatcher
		// looks up the forum thread by content_id at dispatch time.
		payload.TargetContentID = content.ID.String()
	case "forum_question":
		// The persona starts a new thread in the content's space.
		payload.TargetSpaceID = content.SpaceID.String()
	case "follow":
		payload.TargetEntityType = "user"
		payload.TargetEntityID = content.AuthorID.String()
	case "react":
		payload.TargetEntityType = "content"
		payload.TargetEntityID = content.ID.String()
		payload.ReactionKey = "like" // default reaction key
	case "bookmark":
		payload.TargetEntityType = "content"
		payload.TargetEntityID = content.ID.String()
	case "form_submit":
		// form_submit target is set by the caller; eligible content doesn't
		// carry a form_id, so this action type requires external enrichment.
	}

	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return reviewID, contentID, payload, fmt.Errorf("marshal action payload: %w", err)
	}

	// Create a draft content for the persona action. The persona's backing
	// user account has is_bot=true (added in migration 000246), so the
	// content is attributed to the persona's user_id.
	// Map the action type to a valid §14 content_type (BUG-007).
	// For non-content actions (follow, react, bookmark, form_submit), the
	// draft content row is still created as a record of the AI-generated
	// body, but DispatchAction routes through the existing action endpoints
	// instead of publishing it (BUG-010, ADR-077 §5).
	contentType := actionTypeToContentType(actionType)
	slug := fmt.Sprintf("persona-%s-%s", actionType, uuid.NewString())
	title := "Persona " + strings.Title(actionType)

	err = s.db.Writer.QueryRow(ctx, `INSERT INTO contents (author_id, author_profile_id, content_type,
		                       title, slug, body, status, created_at) VALUES ($1, $2, $3, $4, $5, $6, 'draft', now())
		 RETURNING id`, userID, profileID, contentType, title, slug, `{"type":"doc"}`).Scan(&contentID)
	if err != nil {
		return reviewID, contentID, payload, fmt.Errorf("create draft: %w", err)
	}

	// For autonomous dispatch, the persona's backing user is recorded as the
	// reviewer so the row is approved immediately (ADR-077 §4).
	var reviewerID pgtype.UUID
	if status == "approved" {
		_ = reviewerID.Scan(persona.UserID)
	}

	// Create the review entry with source='persona_action' (§76, ADR-077 §4,
	// migration 000411) so the admin queue can filter persona actions from
	// editorial reviews. reviewer_user_id is NULL until an admin approves
	// or rejects (BUG-009) — createdBy is the persona's creator, not the
	// reviewer. The action payload is stored in the dedicated persona_payload
	// jsonb column (migration 000482, BUG-003) so DispatchAction can route the
	// action to the correct existing endpoint (BUG-010). The feedback column
	// remains reserved for editorial free-text.
	err = s.db.Writer.QueryRow(ctx, `INSERT INTO content_reviews (content_id, reviewer_user_id, status, source, persona_payload) VALUES ($1, $2, $3, 'persona_action', $4)
		 RETURNING id`, contentID, reviewerID, status, payloadJSON).Scan(&reviewID)
	if err != nil {
		return reviewID, contentID, payload, fmt.Errorf("create review: %w", err)
	}

	return reviewID, contentID, payload, nil
}

// routeToReviewQueue creates a pending content_reviews entry for the persona
// action so an admin can approve or reject it before it is dispatched.
func (s *Service) routeToReviewQueue(ctx context.Context, persona personas.PersonaDetail, content eligibleContent, actionType string) error {
	_, contentID, payload, err := s.createPersonaAction(ctx, persona, content, actionType, "pending")
	if err != nil {
		return err
	}

	var userID pgtype.UUID
	_ = userID.Scan(persona.UserID)

	// Record a synthetic platform_event for the queued action (ADR-068 §4, P10).
	s.trackPersonaEvent(ctx, userID, "persona.action_queued", "content", contentID, map[string]any{
		"action_type":        actionType,
		"content_type":       actionTypeToContentType(actionType),
		"target_content_id":  content.ID.String(),
		"target_entity_type": payload.TargetEntityType,
		"target_entity_id":   payload.TargetEntityID,
	})

	return nil
}

// actionTypeToContentType maps a participation action type to a valid §14
// content_type. Action types (comment, forum_answer, follow, react, bookmark)
// are orchestration verbs, not content types. Without this mapping, the draft
// content row would carry an invalid content_type that breaks feed rendering,
// RAG indexing, and content-type filters (BUG-007).
func actionTypeToContentType(actionType string) string {
	switch actionType {
	case "comment":
		return "comment"
	case "forum_answer", "forum_question":
		return "forum_post"
	case "blog_post", "guide", "discussion", "forum_thread":
		return actionType // already a valid content_type
	default:
		return "discussion" // safe fallback for react/follow/bookmark/form_submit
	}
}

// trackPersonaEvent records a platform_event with is_synthetic = true for a
// persona action. This is the core metrics-integrity mechanism (ADR-068 §4,
// P10): persona-originated events are tagged so the activity log can find
// them (WHERE is_synthetic = true) and real-metric materialized views can
// exclude them (WHERE NOT is_synthetic). If no tracker is wired, the event
// is silently skipped — the orchestration engine must not fail because of
// analytics unavailability.
func (s *Service) trackPersonaEvent(ctx context.Context, userID pgtype.UUID, eventType, entityType string, entityID pgtype.UUID, meta map[string]any) {
	if s.tracker == nil {
		slog.Warn("participation: no event tracker configured, synthetic event not recorded", "event_type", eventType)
		return
	}

	if err := s.tracker.TrackSyntheticEvent(ctx, userID, eventType, entityType, entityID, meta); err != nil {
		slog.Warn("participation: track synthetic event failed", "event_type", eventType, "error", err)
	}
}

// enqueueDispatchAction creates an auto-approved content_reviews row for an
// autonomous persona action and enqueues persona:dispatch_action with the
// review ID. This matches the contract the worker expects (BUG-010, ADR-077 §5).
func (s *Service) enqueueDispatchAction(ctx context.Context, persona personas.PersonaDetail, content eligibleContent, actionType string) error {
	if s.enqueuer == nil {
		return errors.New("no enqueuer configured")
	}

	reviewID, _, _, err := s.createPersonaAction(ctx, persona, content, actionType, "approved")
	if err != nil {
		return err
	}

	payload, _ := json.Marshal(map[string]string{
		"action_id": reviewID.String(),
	})

	return s.enqueuer.Enqueue("persona:dispatch_action", payload)
}

// --- Valkey helpers ---

func (s *Service) acquireRunLock(ctx context.Context, personaID pgtype.UUID) bool {
	if s.vk == nil {
		return true
	}

	key := valkey.MustKey("persona:lock:%s", personaID.String())

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

	_ = s.vk.Del(ctx, valkey.MustKey("persona:lock:%s", personaID.String())).Err()
}

func (s *Service) getDailyCounter(ctx context.Context, personaID pgtype.UUID, date, actionType string) int {
	if s.vk == nil {
		return 0
	}

	key := valkey.MustKey("persona:daily:%s:%s:%s", personaID.String(), date, actionType)

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

	key := valkey.MustKey("persona:daily:%s:%s:%s", personaID.String(), date, actionType)
	pipe := s.vk.Pipeline()
	pipe.Incr(ctx, key)
	pipe.ExpireAt(ctx, key, nextMidnightUTC())
	_, _ = pipe.Exec(ctx)
}

func (s *Service) getGlobalDailyCounter(ctx context.Context, date string) int {
	if s.vk == nil {
		return 0
	}

	key := valkey.MustKey("persona:global::%s", date)

	val, err := s.vk.Get(ctx, key).Int()
	if err != nil {
		return 0
	}

	return val
}

func (s *Service) incrementGlobalCounter(ctx context.Context, date string) {
	if s.vk == nil {
		return
	}

	key := valkey.MustKey("persona:global::%s", date)
	pipe := s.vk.Pipeline()
	pipe.Incr(ctx, key)
	pipe.ExpireAt(ctx, key, nextMidnightUTC())
	_, _ = pipe.Exec(ctx)
}

func (s *Service) getGlobalDailyCap(ctx context.Context) int {
	if s.settings == nil {
		return 500
	}

	return s.settings.GetIntOr(ctx, "participation.global_daily_cap", 500)
}

// SetFeatureChecker injects the feature flag checker so isEnabled reads the
// feature_flags table instead of system_settings. Falls back to
// system_settings when nil.
func (s *Service) SetFeatureChecker(c FeatureChecker) { s.checker = c }

// isEnabled resolves the participation.enabled feature flag;
// system_settings is consulted only as a fallback when no checker is wired.
func (s *Service) isEnabled(ctx context.Context) bool {
	if s.checker != nil {
		return s.checker.IsEnabled(ctx, "participation.enabled")
	}

	if s.settings == nil {
		return true
	}
	// Use the default platform for the global check.
	return s.settings.GetBoolOr(ctx, "participation.enabled", true)
}

func (s *Service) isWithinActiveHours(hours [2]int) bool {
	if hours[0] == 0 && hours[1] == 0 {
		return true
	}

	hour := time.Now().UTC().Hour()
	// §76 / ADR-077 §1: active_hours_utc is an inclusive range [start, end].
	// Previously used < (exclusive), which excluded the final hour (BUG-005).
	return hour >= hours[0] && hour <= hours[1]
}

func nextMidnightUTC() time.Time {
	now := time.Now().UTC()
	return time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 0, time.UTC)
}
