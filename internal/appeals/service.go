// Package appeals implements the user-facing appeal flow (§22) backed by
// the moderation_appeals table and integrated with the §38 ticket system:
// every submitted appeal creates a linked staff-facing ticket under the
// system 'appeal' ticket type so the discussion happens in the normal
// ticket queue.
//
// Subject types (entity_type on moderation_appeals) each register
// eligibility + overturn hooks so any feature with a rejectable decision
// (service claims, org ownership claims, video channel claims, moderation
// bans/suspensions/warnings, content removals) can plug into the same
// appeal surface.
//
// Status vocabulary (admin panel): pending | upheld | overturn | dismissed
//
//	upheld    — original decision stands (appeal denied)
//	overturn  — appeal granted; the subject-type reversal runs
//	dismissed — invalid appeal; no subject action
//
// Ref: japaconnect-engineering/sections/22-admin-panel.md (Appeals)
//
//	japaconnect-engineering/sections/38-ticket-system.md (Appeal Tickets)
package appeals

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/db/gen"
	"github.com/jedafe/japa/internal/tickets"
)

// Appeal statuses (admin resolution vocabulary — §22).
const (
	StatusPending   = "pending"
	StatusUpheld    = "upheld"
	StatusOverturn  = "overturn"
	StatusDismissed = "dismissed"
)

// Subject types stored in moderation_appeals.entity_type. 'content' and
// 'user' are legacy aliases already present in seeded rows — they resolve
// to the same handlers as content_removal and ban.
const (
	SubjectContent           = "content"
	SubjectContentRemoval    = "content_removal"
	SubjectComment           = "comment"
	SubjectUser              = "user"
	SubjectBan               = "ban"
	SubjectSuspension        = "suspension"
	SubjectWarning           = "warning"
	SubjectOperationBan      = "operation_ban"
	SubjectServiceClaim      = "service_claim"
	SubjectOrgClaim          = "org_claim"
	SubjectVideoClaim        = "video_channel_claim"
	SubjectMedia             = "media"
	SubjectWikiRevision      = "wiki_revision"
	SubjectContentReport     = "content_report"
	SubjectScamReport        = "scam_report"
	SubjectAcademyInstructor = "academy_instructor"

	// Extended registry (§22 round 2): per-domain moderation decisions
	// that produce a user-visible rejected/removed/suspended state.
	SubjectAcademyTrack        = "academy_track"
	SubjectReview              = "review"
	SubjectMutualAidPost       = "mutual_aid_post"
	SubjectVideoSource         = "video_source"
	SubjectRoadmapItem         = "roadmap_item"
	SubjectOrganization        = "organization"
	SubjectSpace               = "space"
	SubjectAffiliateMember     = "affiliate_member"
	SubjectReferral            = "referral"
	SubjectWithdrawal          = "withdrawal"
	SubjectAdCampaign          = "ad_campaign"
	SubjectContentReview       = "content_review"
	SubjectCommunityOrg        = "community_org"
	SubjectCommunityOrgClaim   = "community_org_claim"
	SubjectVerificationRequest = "verification_request"
)

// AppealTicketTypeSlug is the system ticket type under which appeal
// threads are created (migration 000818 seeds it is_system + !is_public).
const AppealTicketTypeSlug = "appeal"

const maxReasonLen = 4000

// Sentinel errors mapped to the standard API envelope in handlers.
var (
	ErrUnknownSubject  = errors.New("unknown appeal subject type")
	ErrNotEligible     = errors.New("subject is not eligible for appeal")
	ErrDuplicate       = errors.New("a pending appeal already exists for this subject")
	ErrInvalidStatus   = errors.New("status must be upheld, overturn, or dismissed")
	ErrAlreadyResolved = errors.New("appeal is already resolved")
	ErrReasonRequired  = errors.New("reason is required")
	// ErrReversalBlocked marks an overturn that cannot safely complete —
	// e.g. the refunded credits were already spent, so re-reserving them
	// would push the wallet negative. Resolution stays pending; the admin
	// can retry after the blocker clears.
	ErrReversalBlocked = errors.New("overturn reversal is currently blocked")
)

// Appeal is the resolved record returned to callers.
type Appeal struct {
	ID             string     `json:"id"`
	EntityType     string     `json:"entity_type"`
	EntityID       string     `json:"entity_id"`
	Label          string     `json:"label,omitempty"`
	Reason         string     `json:"reason"`
	Status         string     `json:"status"`
	TicketID       string     `json:"ticket_id,omitempty"`
	TicketNumber   *int64     `json:"ticket_number,omitempty"`
	ResolutionNote string     `json:"resolution_note,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
	ResolvedAt     *time.Time `json:"resolved_at,omitempty"`
}

// EligibleItem is one appealable decision for a user — returned by
// ListEligible hooks and aggregated by Service.Eligible for the
// "submit an appeal" picker (GET /me/appeals/eligible). EntityType is
// stamped by the aggregator; hooks fill the rest.
type EligibleItem struct {
	EntityType string `json:"entity_type"`
	EntityID   string `json:"entity_id"`
	Label      string `json:"label"`
	Detail     string `json:"detail,omitempty"`
}

// SubjectHooks wires one appealable subject type.
//
// CanAppeal validates the subject exists, is in an appealable state, and
// the user is entitled to appeal it. Returns a short human label used in
// the linked ticket's subject line (e.g. "service claim — Relocation Help
// Lagos"). Any error other than ErrNotEligible is treated as a 500.
//
// OnOverturn applies the domain reversal when an appeal is granted —
// e.g. approve the claim, lift the ban, restore the content. Runs before
// the appeal row is marked resolved; a returned error aborts the
// resolution and leaves the appeal pending. reviewedBy is the resolving
// admin (needed when the reversal re-runs a domain review path).
type SubjectHooks struct {
	CanAppeal  func(ctx context.Context, userID, entityID pgtype.UUID) (label string, err error)
	OnOverturn func(ctx context.Context, appeal Appeal, reviewedBy pgtype.UUID) error
	// ListEligible returns the caller's currently appealable entities for
	// this subject — powers the discovery endpoint so users don't need to
	// know entity IDs. Optional; subjects without it are simply absent
	// from the picker.
	ListEligible func(ctx context.Context, userID pgtype.UUID) ([]EligibleItem, error)
}

// TicketService is the §38 surface appeals needs: create the staff thread
// at submit and resolve it when the appeal closes.
type TicketService interface {
	CreateTicket(ctx context.Context, input tickets.CreateTicketInput) (gen.Ticket, error)
	UpdateTicketStatus(ctx context.Context, id pgtype.UUID, input tickets.UpdateTicketStatusInput) (gen.Ticket, error)
	ResolveTicket(ctx context.Context, id pgtype.UUID, input tickets.ResolveTicketInput) (gen.Ticket, error)
	GetTicketTypeBySlug(ctx context.Context, slug string) (gen.TicketType, error)
}

// Service coordinates appeal submission and resolution.
type Service struct {
	db       *db.DB
	read     *gen.Queries
	write    *gen.Queries
	tickets  TicketService
	subjects map[string]SubjectHooks

	// invalidateUserAccess clears per-user access caches (perm cache + ban
	// cache) after a ban/suspension overturn. Wired by the server; nil-safe.
	invalidateUserAccess func(ctx context.Context, userID pgtype.UUID)
}

// NewService builds the appeals service. tickets may be nil — submission
// then still records the appeal but skips the staff-thread creation.
// Built-in subjects cover moderation actions, content removals, and the
// §98 service claim; cross-service subjects (org_claim,
// video_channel_claim) are registered from server.go.
func NewService(database *db.DB, ticketSvc TicketService) *Service {
	s := &Service{
		db:       database,
		read:     gen.New(database.Reader),
		write:    gen.New(database.Writer),
		tickets:  ticketSvc,
		subjects: make(map[string]SubjectHooks),
	}
	s.registerBuiltins()

	return s
}

// SetAccessCacheInvalidator injects the callback that clears ban/perm
// caches after a ban or suspension overturn.
func (s *Service) SetAccessCacheInvalidator(fn func(ctx context.Context, userID pgtype.UUID)) {
	s.invalidateUserAccess = fn
}

// RegisterSubject wires (or overrides) a subject type's hooks. Called from
// server.go for subjects owned by other packages.
func (s *Service) RegisterSubject(entityType string, hooks SubjectHooks) {
	s.subjects[entityType] = hooks
}

// RegisterOverturn attaches (or replaces) only the OnOverturn hook for a
// subject — used when the domain reversal lives in another package but the
// built-in eligibility check already covers submission.
func (s *Service) RegisterOverturn(entityType string, fn func(ctx context.Context, appeal Appeal, reviewedBy pgtype.UUID) error) {
	h := s.subjects[entityType]
	h.OnOverturn = fn
	s.subjects[entityType] = h
}

// Subjects returns the registered subject type names (admin/debug use).
func (s *Service) Subjects() []string {
	out := make([]string, 0, len(s.subjects))
	for k := range s.subjects {
		out = append(out, k)
	}

	return out
}

// appealRowData is the shared shape of the three generated row types
// (CreateAppealRow, GetAppealByIDRow, ListAppealsByUserRow).
type appealRowData struct {
	ID             pgtype.UUID
	EntityType     string
	EntityID       pgtype.UUID
	Reason         string
	SubjectLabel   *string
	Status         string
	TicketID       pgtype.UUID
	TicketNumber   *int64
	ResolutionNote *string
	CreatedAt      pgtype.Timestamptz
	ResolvedAt     pgtype.Timestamptz
}

func appealFromRow(r appealRowData) Appeal {
	a := Appeal{
		ID:         uuidStr(r.ID),
		EntityType: r.EntityType,
		EntityID:   uuidStr(r.EntityID),
		Reason:     r.Reason,
		Status:     r.Status,
		CreatedAt:  r.CreatedAt.Time,
	}
	if r.SubjectLabel != nil {
		a.Label = *r.SubjectLabel
	}

	if r.TicketID.Valid {
		a.TicketID = uuidStr(r.TicketID)
	}

	a.TicketNumber = r.TicketNumber
	if r.ResolutionNote != nil {
		a.ResolutionNote = *r.ResolutionNote
	}

	if r.ResolvedAt.Valid {
		t := r.ResolvedAt.Time
		a.ResolvedAt = &t
	}

	return a
}

func uuidStr(id pgtype.UUID) string {
	if !id.Valid {
		return ""
	}

	return id.String()
}

// namedLabel builds "<prefix> — <name>" for subjects with a human-readable
// name/title, else just the prefix — a label must never fall back to a raw
// entity UUID (it feeds the linked ticket subject + appeal lists).
func namedLabel(prefix, name string) string {
	if name = strings.TrimSpace(name); name != "" {
		return fmt.Sprintf("%s — %s", prefix, name)
	}

	return prefix
}

func namedLabelPtr(prefix string, name *string) string {
	if name == nil {
		return prefix
	}

	return namedLabel(prefix, *name)
}

// subjectAliases returns the entity_type spellings that refer to the same
// subject — legacy aliases ('content' ↔ 'content_removal', 'user' ↔
// 'ban'/'suspension'/'warning') share an entity_id space but store under
// different types, so dedupe must check the whole group.
func subjectAliases(entityType string) []string {
	switch entityType {
	case SubjectContent, SubjectContentRemoval:
		return []string{SubjectContent, SubjectContentRemoval}
	case SubjectUser:
		return []string{SubjectUser}
	default:
		return []string{entityType}
	}
}

// Submit validates eligibility, records the appeal, and creates the linked
// staff ticket. Returns the created appeal.
func (s *Service) Submit(ctx context.Context, userID pgtype.UUID, entityType, entityIDStr, reason string) (Appeal, error) {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return Appeal{}, ErrReasonRequired
	}

	if utf8.RuneCountInString(reason) > maxReasonLen {
		reason = string([]rune(reason)[:maxReasonLen])
	}

	hooks, ok := s.subjects[entityType]
	if !ok {
		return Appeal{}, ErrUnknownSubject
	}

	var entityID pgtype.UUID
	if err := entityID.Scan(entityIDStr); err != nil {
		return Appeal{}, fmt.Errorf("%w: invalid entity_id", ErrNotEligible)
	}

	// The aggregate 'user' subject shares a restriction with the granular
	// 'ban'/'suspension'/'warning'/'operation_ban' subjects (which key on
	// moderation-action row ids, not user_id), so the (entity_type,
	// entity_id) pending check can't catch it — guard explicitly.
	if entityType == SubjectUser {
		dup, err := s.write.HasPendingUserRestrictionAppeal(ctx, gen.HasPendingUserRestrictionAppealParams{
			UserID:  userID,
			Column2: []string{SubjectUser, SubjectBan, SubjectSuspension, SubjectWarning, SubjectOperationBan},
		})
		if err != nil {
			return Appeal{}, fmt.Errorf("checking pending restriction appeal: %w", err)
		}
		if dup {
			return Appeal{}, ErrDuplicate
		}
	}

	label, err := hooks.CanAppeal(ctx, userID, entityID)
	if err != nil {
		return Appeal{}, err
	}

	// Friendly duplicate check — the partial unique index is the
	// authoritative guard against races. The alias set covers 'content'
	// vs 'content_removal' (same entity_id, different stored type).
	pending, err := s.read.CountPendingAppealByUserSubject(ctx, gen.CountPendingAppealByUserSubjectParams{
		UserID:      userID,
		EntityTypes: subjectAliases(entityType),
		EntityID:    entityID,
	})
	if err != nil {
		return Appeal{}, fmt.Errorf("checking pending appeals: %w", err)
	}

	if pending > 0 {
		return Appeal{}, ErrDuplicate
	}

	var subjectLabel *string
	if label != "" {
		subjectLabel = &label
	}

	row, err := s.write.CreateAppeal(ctx, gen.CreateAppealParams{
		UserID:       userID,
		EntityType:   entityType,
		EntityID:     entityID,
		Reason:       reason,
		SubjectLabel: subjectLabel,
	})
	if err != nil {
		// The partial unique index is the authoritative duplicate guard —
		// map the 23505 race (two concurrent submits past the count check)
		// to ErrDuplicate rather than a generic 500.
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return Appeal{}, ErrDuplicate
		}

		return Appeal{}, fmt.Errorf("creating appeal: %w", err)
	}

	a := appealFromRow(appealRowData{
		ID: row.ID, EntityType: row.EntityType, EntityID: row.EntityID,
		Reason: row.Reason, SubjectLabel: row.SubjectLabel, Status: row.Status, TicketID: row.TicketID,
		ResolutionNote: row.ResolutionNote, CreatedAt: row.CreatedAt, ResolvedAt: row.ResolvedAt,
	})
	if tk := s.createAppealTicket(ctx, userID, a, label); tk != nil {
		a.TicketID = tk.ID.String()
		a.TicketNumber = tk.TicketNumber
	}

	return a, nil
}

// createAppealTicket opens the §38 staff thread and links it back. Failure
// is logged and non-fatal — the appeal record stands on its own. Returns
// the linked ticket (nil when creation/linking was skipped or failed) so
// the submit response can carry both its UUID and public JC-<n> reference.
func (s *Service) createAppealTicket(ctx context.Context, userID pgtype.UUID, a Appeal, label string) *gen.Ticket {
	if s.tickets == nil {
		return nil
	}

	tt, err := s.tickets.GetTicketTypeBySlug(ctx, AppealTicketTypeSlug)
	if err != nil {
		slog.Error("appeals: appeal ticket type missing — run migration 000818", "error", err)
		return nil
	}

	if label == "" {
		label = a.EntityType
	}
	// entity_type/entity_id live in field_values for staff tooling — the
	// user-visible body carries only the human-readable label.
	desc := fmt.Sprintf("Appeal by user.\n\nDecision: %s (%s)\n\nReason:\n%s",
		label, a.EntityType, a.Reason)

	tk, err := s.tickets.CreateTicket(ctx, tickets.CreateTicketInput{
		TypeID:          tt.ID,
		Subject:         "Appeal: " + label,
		Description:     desc,
		SubmitterUserID: userID,
		FieldValues: map[string]any{
			"appeal_id":     a.ID,
			"entity_type":   a.EntityType,
			"entity_id":     a.EntityID,
			"subject_label": label,
		},
	})
	if err != nil {
		slog.Error("appeals: staff ticket creation failed", "appeal_id", a.ID, "error", err)
		return nil
	}

	var aid pgtype.UUID
	if err := aid.Scan(a.ID); err != nil {
		return nil
	}

	if err := s.write.SetAppealTicketID(ctx, gen.SetAppealTicketIDParams{TicketID: tk.ID, ID: aid}); err != nil {
		slog.Error("appeals: linking ticket to appeal failed", "appeal_id", a.ID, "ticket_id", tk.ID.String(), "error", err)
		return nil
	}

	return &tk
}

// ListMine returns the appellant's own appeals, newest first.
func (s *Service) ListMine(ctx context.Context, userID pgtype.UUID, limit, offset int32) ([]Appeal, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}

	rows, err := s.read.ListAppealsByUser(ctx, gen.ListAppealsByUserParams{
		UserID:     userID,
		PageLimit:  limit,
		PageOffset: offset,
	})
	if err != nil {
		return nil, fmt.Errorf("listing appeals: %w", err)
	}

	out := make([]Appeal, 0, len(rows))
	for _, r := range rows {
		out = append(out, appealFromRow(appealRowData{
			ID: r.ID, EntityType: r.EntityType, EntityID: r.EntityID,
			Reason: r.Reason, SubjectLabel: r.SubjectLabel, Status: r.Status, TicketID: r.TicketID,
			TicketNumber:   r.TicketNumber,
			ResolutionNote: r.ResolutionNote, CreatedAt: r.CreatedAt, ResolvedAt: r.ResolvedAt,
		}))
	}

	return out, nil
}

// Eligible aggregates every subject's ListEligible hook into the set of
// decisions the caller can currently appeal — the discovery surface behind
// GET /me/appeals/eligible. A failing hook is logged and skipped so one
// broken subject can't hide the others.
func (s *Service) Eligible(ctx context.Context, userID pgtype.UUID) ([]EligibleItem, error) {
	// Already-pending appeals are filtered so the picker doesn't offer a
	// duplicate submission (Submit would 409 anyway).
	pending, err := s.read.ListPendingAppealSubjects(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("listing pending appeals: %w", err)
	}

	pendingSet := make(map[string]bool, len(pending))
	for _, p := range pending {
		pendingSet[p.EntityType+"/"+uuidStr(p.EntityID)] = true
	}

	// anyPending reports whether an eligible item already has a pending
	// appeal under any alias spelling of its subject type.
	anyPending := func(entityType, entityID string) bool {
		for _, et := range subjectAliases(entityType) {
			if pendingSet[et+"/"+entityID] {
				return true
			}
		}

		return false
	}

	types := s.Subjects()
	sort.Strings(types)

	// Fan out per-subject ListEligible hooks — each is one or two indexed
	// queries, but ~30 of them sequentially added noticeable latency.
	type eligibleBatch struct {
		entityType string
		items      []EligibleItem
	}

	results := make([]eligibleBatch, len(types))

	var wg sync.WaitGroup
	for i, et := range types {
		hook := s.subjects[et].ListEligible
		if hook == nil {
			continue
		}
		wg.Add(1)
		go func(i int, et string, hook func(context.Context, pgtype.UUID) ([]EligibleItem, error)) {
			defer wg.Done()
			items, err := hook(ctx, userID)
			if err != nil {
				slog.Warn("appeals: eligible list failed", "entity_type", et, "error", err)
				return
			}
			results[i] = eligibleBatch{entityType: et, items: items}
		}(i, et, hook)
	}
	wg.Wait()

	out := make([]EligibleItem, 0, 8)

	for _, b := range results {
		for _, it := range b.items {
			it.EntityType = b.entityType
			if anyPending(b.entityType, it.EntityID) {
				continue
			}
			out = append(out, it)
		}
	}

	return out, nil
}

// Resolve applies an admin decision: 'overturn' runs the subject-type
// reversal first (aborting on error so the appeal stays pending), then
// marks the appeal resolved and closes the linked ticket.
func (s *Service) Resolve(ctx context.Context, appealID, reviewerID pgtype.UUID, status, note string) error {
	if status != StatusUpheld && status != StatusOverturn && status != StatusDismissed {
		return ErrInvalidStatus
	}

	row, err := s.read.GetAppealByID(ctx, appealID)
	if err != nil {
		return err
	}

	if row.Status != StatusPending {
		return ErrAlreadyResolved
	}

	appeal := appealFromRow(appealRowData{
		ID: row.ID, EntityType: row.EntityType, EntityID: row.EntityID,
		Reason: row.Reason, SubjectLabel: row.SubjectLabel, Status: row.Status, TicketID: row.TicketID,
		TicketNumber:   row.TicketNumber,
		ResolutionNote: row.ResolutionNote, CreatedAt: row.CreatedAt, ResolvedAt: row.ResolvedAt,
	})

	var notePtr *string

	if strings.TrimSpace(note) != "" {
		n := strings.TrimSpace(note)
		notePtr = &n
	}

	// Claim the transition first — the guarded status='pending' update makes
	// exactly one resolver win, so concurrent resolves can't both apply the
	// reversal. A failed reversal rolls the appeal back to pending.
	tag, err := s.write.ResolveAppeal(ctx, gen.ResolveAppealParams{
		Status:         status,
		ReviewedBy:     reviewerID,
		ResolutionNote: notePtr,
		ID:             appealID,
	})
	if err != nil {
		return fmt.Errorf("resolving appeal: %w", err)
	}

	if tag == 0 {
		return ErrAlreadyResolved
	}

	if status == StatusOverturn {
		if hooks, ok := s.subjects[appeal.EntityType]; ok && hooks.OnOverturn != nil {
			if err := hooks.OnOverturn(ctx, appeal, reviewerID); err != nil {
				if rerr := s.write.ReopenAppealOnFailedOverturn(ctx, appealID); rerr != nil {
					slog.Error("appeals: failed to reopen after overturn failure", "appeal_id", appealID.String(), "error", rerr)
				}
				return fmt.Errorf("applying overturn: %w", err)
			}
		}
	}

	// §22: every resolution is audit-logged. Written after the reversal so a
	// rolled-back overturn isn't recorded as resolved.
	targetType := "appeal"
	metadata, _ := json.Marshal(map[string]any{
		"appeal_id":       appeal.ID,
		"entity_type":     appeal.EntityType,
		"entity_id":       appeal.EntityID,
		"resolution_note": notePtr,
	})
	if _, aerr := s.write.CreateAuditLog(ctx, gen.CreateAuditLogParams{
		ActorUserID: reviewerID,
		ActionType:  "moderation.appeal.resolve",
		TargetType:  &targetType,
		TargetID:    appealID,
		Metadata:    metadata,
	}); aerr != nil {
		slog.Warn("appeals: resolution audit log failed", "appeal_id", appeal.ID, "error", aerr)
	}

	if row.TicketID.Valid && s.tickets != nil {
		s.resolveAppealTicket(ctx, row.TicketID, reviewerID, note, appeal.ID)
	}

	return nil
}

// resolveAppealTicket closes the linked staff thread. The §38 state
// machine requires open → in_progress before resolved, so tickets never
// touched by an agent are walked through in_progress first. Failures are
// logged and non-fatal — the appeal itself is already resolved.
func (s *Service) resolveAppealTicket(ctx context.Context, ticketID, reviewerID pgtype.UUID, note, appealID string) {
	if _, err := s.tickets.UpdateTicketStatus(ctx, ticketID, tickets.UpdateTicketStatusInput{
		Status: string(tickets.StatusInProgress),
		UserID: reviewerID,
	}); err != nil {
		// Already past 'open' is fine; anything else logs below via
		// ResolveTicket's transition check.
		slog.Debug("appeals: ticket in_progress transition skipped", "ticket_id", ticketID.String(), "error", err)
	}
	// The in_progress hop above is a write; ResolveTicket re-reads through
	// the replica, which can lag the write by tens of ms — retry briefly.
	var lastErr error
	for range 4 {
		_, lastErr = s.tickets.ResolveTicket(ctx, ticketID, tickets.ResolveTicketInput{
			UserID:         reviewerID,
			ResolutionNote: note,
		})
		if lastErr == nil {
			return
		}
		select {
		case <-ctx.Done():
			lastErr = ctx.Err()
			goto done
		case <-time.After(150 * time.Millisecond):
		}
	}
done:
	slog.Warn("appeals: linked ticket resolve failed", "appeal_id", appealID, "ticket_id", ticketID.String(), "error", lastErr)
}

// ---------------------------------------------------------------------------
// Built-in subjects
// ---------------------------------------------------------------------------

func (s *Service) registerBuiltins() {
	s.subjects[SubjectContent] = s.contentSubject(false)
	s.subjects[SubjectContentRemoval] = s.contentSubject(true)
	s.subjects[SubjectComment] = s.commentSubject()
	s.subjects[SubjectUser] = s.userSubject()
	s.subjects[SubjectBan] = s.moderationActionSubject("ban")
	s.subjects[SubjectSuspension] = s.moderationActionSubject("suspension")
	s.subjects[SubjectWarning] = s.moderationActionSubject("warning")
	s.subjects[SubjectOperationBan] = s.operationBanSubject()
	s.subjects[SubjectServiceClaim] = s.serviceClaimSubject()
	s.subjects[SubjectOrgClaim] = s.orgClaimSubject()
	s.subjects[SubjectVerificationRequest] = s.verificationRequestSubject()
	s.subjects[SubjectVideoClaim] = s.videoClaimSubject()
	s.subjects[SubjectMedia] = s.mediaSubject()
	s.subjects[SubjectWikiRevision] = s.wikiRevisionSubject()
	s.subjects[SubjectContentReport] = s.contentReportSubject()
	s.subjects[SubjectScamReport] = s.scamReportSubject()
	s.subjects[SubjectAcademyInstructor] = s.academyInstructorSubject()
	s.subjects[SubjectAcademyTrack] = s.academyTrackSubject()
	s.subjects[SubjectReview] = s.reviewSubject()
	s.subjects[SubjectMutualAidPost] = s.mutualAidPostSubject()
	s.subjects[SubjectVideoSource] = s.videoSourceSubject()
	s.subjects[SubjectRoadmapItem] = s.roadmapItemSubject()
	s.subjects[SubjectOrganization] = s.organizationSubject()
	s.subjects[SubjectSpace] = s.spaceSubject()
	s.subjects[SubjectAffiliateMember] = s.affiliateMemberSubject()
	s.subjects[SubjectReferral] = s.referralSubject()
	s.subjects[SubjectWithdrawal] = s.withdrawalSubject()
	s.subjects[SubjectAdCampaign] = s.adCampaignSubject()
	s.subjects[SubjectContentReview] = s.contentReviewSubject()
	s.subjects[SubjectCommunityOrg] = s.communityOrgSubject()
	s.subjects[SubjectCommunityOrgClaim] = s.communityOrgClaimSubject()
}

// contentSubject covers content_removal appeals: the appellant must be the
// author and the content must be soft-deleted. Overturn restores it.
// withList=false gives the legacy 'content' alias the same eligibility +
// reversal but keeps it out of the picker (canonical type is
// content_removal).
func (s *Service) contentSubject(withList bool) SubjectHooks {
	h := SubjectHooks{
		CanAppeal: func(ctx context.Context, userID, entityID pgtype.UUID) (string, error) {
			row, err := s.read.GetContentForAppeal(ctx, entityID)
			if err != nil {
				if errors.Is(err, pgx.ErrNoRows) {
					return "", ErrNotEligible // not removed, or doesn't exist
				}
				return "", fmt.Errorf("fetching content: %w", err)
			}
			if row.AuthorID != userID {
				return "", ErrNotEligible
			}
			return namedLabel("Removed "+row.ContentType, row.Title), nil
		},
		OnOverturn: func(ctx context.Context, a Appeal, _ pgtype.UUID) error {
			var id pgtype.UUID
			if err := id.Scan(a.EntityID); err != nil {
				return err
			}
			_, err := s.write.RestoreContentOnAppeal(ctx, id)
			return err
		},
	}
	if withList {
		h.ListEligible = func(ctx context.Context, userID pgtype.UUID) ([]EligibleItem, error) {
			rows, err := s.read.ListAppealableRemovedContents(ctx, userID)
			if err != nil {
				return nil, err
			}

			out := make([]EligibleItem, 0, len(rows))
			for _, r := range rows {
				out = append(out, EligibleItem{
					EntityID: uuidStr(r.ID),
					Label:    fmt.Sprintf("Removed %s: %s", r.ContentType, r.Title),
				})
			}

			return out, nil
		}
	}

	return h
}

// moderationActionSubject covers ban/suspension/warning appeals where
// entity_id is the user_moderation_actions row (must belong to the
// appellant and match the action type).
func (s *Service) moderationActionSubject(action string) SubjectHooks {
	return SubjectHooks{
		CanAppeal: func(ctx context.Context, userID, entityID pgtype.UUID) (string, error) {
			row, err := s.read.GetModerationActionByID(ctx, entityID)
			if err != nil {
				if errors.Is(err, pgx.ErrNoRows) {
					return "", ErrNotEligible
				}
				return "", fmt.Errorf("fetching moderation action: %w", err)
			}
			if row.UserID != userID || row.Action != action {
				return "", ErrNotEligible
			}
			// Date suffix disambiguates repeat actions (e.g. a second ban).
			return fmt.Sprintf("Account %s (%s)", action, row.CreatedAt.Time.Format("Jan 2, 2006")), nil
		},
		OnOverturn: func(ctx context.Context, a Appeal, _ pgtype.UUID) error {
			var id pgtype.UUID
			if err := id.Scan(a.EntityID); err != nil {
				return err
			}
			// Resolve the affected user before lifting — the lift
			// soft-deletes the row and the read filters deleted rows.
			// Overturn lookup ignores expiry (GetModerationActionForOverturn):
			// a ban that expired mid-appeal still needs finishUserLift for the
			// status restore + cache invalidation. ErrNoRows means already
			// lifted — no-op so resolution still completes.
			row, err := s.read.GetModerationActionForOverturn(ctx, id)
			if err != nil {
				if errors.Is(err, pgx.ErrNoRows) {
					return nil
				}
				return err
			}
			tag, err := s.write.LiftUserActionByID(ctx, id)
			if err != nil {
				return err
			}
			if tag > 0 && (action == "ban" || action == "suspension") {
				s.finishUserLift(ctx, row.UserID)
			}
			return nil
		},
		ListEligible: func(ctx context.Context, userID pgtype.UUID) ([]EligibleItem, error) {
			rows, err := s.read.ListAppealableModerationActions(ctx, userID)
			if err != nil {
				return nil, err
			}
			out := make([]EligibleItem, 0, len(rows))
			for _, r := range rows {
				if r.Action != action {
					continue
				}
				it := EligibleItem{
					EntityID: uuidStr(r.ID),
					Label:    fmt.Sprintf("Account %s (%s)", action, r.CreatedAt.Time.Format("Jan 2, 2006")),
				}
				if r.Reason != nil {
					it.Detail = *r.Reason
				}
				out = append(out, it)
			}
			return out, nil
		},
	}
}

// userSubject is the legacy 'user' alias — entity_id is the user id and
// the appeal contests the user's live ban/suspension collectively.
func (s *Service) userSubject() SubjectHooks {
	return SubjectHooks{
		CanAppeal: func(ctx context.Context, userID, entityID pgtype.UUID) (string, error) {
			if entityID != userID {
				return "", ErrNotEligible
			}
			// CheckUserBanExists covers both 'ban' and 'suspension' rows —
			// the legacy aggregate subject contests the account-level
			// restriction whichever action produced it.
			restricted, err := s.read.CheckUserBanExists(ctx, userID)
			if err != nil {
				return "", fmt.Errorf("checking active bans: %w", err)
			}
			if !restricted {
				return "", ErrNotEligible
			}
			return "Account restrictions", nil
		},
		OnOverturn: func(ctx context.Context, a Appeal, _ pgtype.UUID) error {
			var uid pgtype.UUID
			if err := uid.Scan(a.EntityID); err != nil {
				return err
			}
			var lifted int64
			for _, action := range []string{"ban", "suspension"} {
				tag, err := s.write.LiftUserActionsByUserAndType(ctx, gen.LiftUserActionsByUserAndTypeParams{
					UserID: uid,
					Action: action,
				})
				if err != nil {
					return err
				}
				lifted += tag
			}
			// One audit/status/cache pass regardless of how many action
			// rows were lifted — the aggregate reversal is a single event.
			if lifted > 0 {
				s.finishUserLift(ctx, uid)
			}
			return nil
		},
		// No ListEligible: the granular ban/suspension subjects already
		// surface each action row in the discovery list, so an aggregate
		// 'user' entry would only duplicate them. CanAppeal stays for
		// legacy direct submissions.
	}
}

// finishUserLift writes the 'unban' audit row, restores users.status, and
// clears access caches after a ban/suspension overturn.
func (s *Service) finishUserLift(ctx context.Context, uid pgtype.UUID) {
	if err := s.write.CreateUserUnban(ctx, uid); err != nil {
		slog.Error("appeals: unban audit row failed", "user_id", uid.String(), "error", err)
	}
	// Never resurrect a 'deleted' account — the appeal may outlive the
	// user's existence (banned → appealed → deleted → overturned).
	status, err := s.read.GetUserStatus(ctx, uid)
	if err != nil {
		slog.Error("appeals: status lookup failed", "user_id", uid.String(), "error", err)
		return
	}

	if status == "deleted" {
		return
	}

	newStatus := "active"
	if remaining, err := s.read.CountActiveOperationBans(ctx, uid); err == nil && remaining > 0 {
		newStatus = "restricted"
	}

	if err := s.write.UpdateUserStatus(ctx, gen.UpdateUserStatusParams{ID: uid, Status: newStatus}); err != nil {
		slog.Error("appeals: status restore failed", "user_id", uid.String(), "error", err)
	}

	if s.invalidateUserAccess != nil {
		s.invalidateUserAccess(ctx, uid)
	}
}

// serviceClaimSubject covers §98 listing-claim rejections: the appellant
// must be a member of the org whose claim was rejected (claimed_by_org_id
// is retained on rejection for exactly this). Overturn approves the claim
// and links org ownership.
func (s *Service) serviceClaimSubject() SubjectHooks {
	return SubjectHooks{
		CanAppeal: func(ctx context.Context, userID, entityID pgtype.UUID) (string, error) {
			meta, err := s.read.GetServiceClaimMetaForAppeal(ctx, entityID)
			if err != nil {
				if errors.Is(err, pgx.ErrNoRows) {
					return "", ErrNotEligible
				}
				return "", fmt.Errorf("fetching service claim: %w", err)
			}
			if meta.ClaimStatus != "rejected" || !meta.ClaimedByOrgID.Valid {
				return "", ErrNotEligible
			}
			member, err := s.read.IsOrganizationMember(ctx, gen.IsOrganizationMemberParams{
				OrganizationID: meta.ClaimedByOrgID,
				UserID:         userID,
			})
			if err != nil {
				return "", fmt.Errorf("checking org membership: %w", err)
			}
			if !member {
				return "", ErrNotEligible
			}
			return namedLabelPtr("Rejected service listing claim", meta.ListingTitle), nil
		},
		ListEligible: func(ctx context.Context, userID pgtype.UUID) ([]EligibleItem, error) {
			rows, err := s.read.ListAppealableServiceClaims(ctx, userID)
			if err != nil {
				return nil, err
			}
			out := make([]EligibleItem, 0, len(rows))
			for _, r := range rows {
				label := "Rejected service listing claim"
				if r.ListingTitle != nil && *r.ListingTitle != "" {
					label = "Rejected service listing claim — " + *r.ListingTitle
				}
				out = append(out, EligibleItem{EntityID: uuidStr(r.ContentID), Label: label})
			}
			return out, nil
		},
		OnOverturn: func(ctx context.Context, a Appeal, _ pgtype.UUID) error {
			var id pgtype.UUID
			if err := id.Scan(a.EntityID); err != nil {
				return err
			}
			// Zero rows = claim no longer rejected (re-reviewed meanwhile)
			// — treat as a no-op success so resolution still completes.
			_, err := s.write.ApproveServiceClaimOnAppeal(ctx, id)
			return err
		},
	}
}

// orgClaimSubject covers §07 ownership_claims: only the claimant can
// appeal, and only while the claim is rejected. The overturn action is
// registered by the server (organizations.Service.ReviewOwnershipClaim
// performs the ownership transfer + notifications).
func (s *Service) orgClaimSubject() SubjectHooks {
	return SubjectHooks{
		CanAppeal: func(ctx context.Context, userID, entityID pgtype.UUID) (string, error) {
			claim, err := s.read.GetOwnershipClaimForAppeal(ctx, entityID)
			if err != nil {
				if errors.Is(err, pgx.ErrNoRows) {
					return "", ErrNotEligible
				}
				return "", fmt.Errorf("fetching ownership claim: %w", err)
			}
			if claim.Status != "rejected" || claim.ClaimantUserID != userID {
				return "", ErrNotEligible
			}
			return namedLabelPtr("Rejected organization claim", claim.OrganizationName), nil
		},
		ListEligible: func(ctx context.Context, userID pgtype.UUID) ([]EligibleItem, error) {
			rows, err := s.read.ListAppealableOrgClaims(ctx, userID)
			if err != nil {
				return nil, err
			}
			out := make([]EligibleItem, 0, len(rows))
			for _, r := range rows {
				label := "Rejected organization claim"
				if r.OrganizationName != nil && *r.OrganizationName != "" {
					label = "Rejected organization claim — " + *r.OrganizationName
				}
				out = append(out, EligibleItem{EntityID: uuidStr(r.ID), Label: label})
			}
			return out, nil
		},
	}
}

// verificationRequestSubject covers §07 verification_requests: only the
// requester can appeal, and only while the request is rejected. The overturn
// is registered by the server — it reopens the request back to pending.
func (s *Service) verificationRequestSubject() SubjectHooks {
	return SubjectHooks{
		CanAppeal: func(ctx context.Context, userID, entityID pgtype.UUID) (string, error) {
			req, err := s.read.GetVerificationRequestForAppeal(ctx, entityID)
			if err != nil {
				if errors.Is(err, pgx.ErrNoRows) {
					return "", ErrNotEligible
				}
				return "", fmt.Errorf("fetching verification request: %w", err)
			}
			if req.Status != "rejected" || req.RequesterUserID != userID {
				return "", ErrNotEligible
			}
			return namedLabelPtr("Rejected verification request", req.OrganizationName), nil
		},
		ListEligible: func(ctx context.Context, userID pgtype.UUID) ([]EligibleItem, error) {
			rows, err := s.read.ListAppealableVerificationRequests(ctx, userID)
			if err != nil {
				return nil, err
			}
			out := make([]EligibleItem, 0, len(rows))
			for _, r := range rows {
				label := "Rejected verification request"
				if r.OrganizationName != nil && *r.OrganizationName != "" {
					label = "Rejected verification request — " + *r.OrganizationName
				}
				out = append(out, EligibleItem{EntityID: uuidStr(r.ID), Label: label})
			}
			return out, nil
		},
	}
}

// videoClaimSubject covers video_channel_claims: claimant-only, rejected
// only. Overturn is registered by the server (video.Service.ReviewClaim
// already permits rejected→approved and runs the transfer/takedown
// side-effects).
func (s *Service) videoClaimSubject() SubjectHooks {
	return SubjectHooks{
		CanAppeal: func(ctx context.Context, userID, entityID pgtype.UUID) (string, error) {
			claim, err := s.read.GetVideoChannelClaimForAppeal(ctx, entityID)
			if err != nil {
				if errors.Is(err, pgx.ErrNoRows) {
					return "", ErrNotEligible
				}
				return "", fmt.Errorf("fetching video channel claim: %w", err)
			}
			if claim.Status != "rejected" || claim.ClaimantUserID != userID {
				return "", ErrNotEligible
			}
			return namedLabelPtr("Rejected video channel claim", claim.VideoTitle), nil
		},
		ListEligible: func(ctx context.Context, userID pgtype.UUID) ([]EligibleItem, error) {
			rows, err := s.read.ListAppealableVideoClaims(ctx, userID)
			if err != nil {
				return nil, err
			}
			out := make([]EligibleItem, 0, len(rows))
			for _, r := range rows {
				label := "Rejected video channel claim"
				if r.VideoTitle != nil && *r.VideoTitle != "" {
					label = "Rejected video channel claim — " + *r.VideoTitle
				}
				out = append(out, EligibleItem{EntityID: uuidStr(r.ID), Label: label})
			}
			return out, nil
		},
	}
}

// operationBanSubject covers §22 granular operation bans
// (user_operation_bans): the appellant must own the active op-ban row.
// Overturn lifts it and restores users.status when no restrictions remain.
func (s *Service) operationBanSubject() SubjectHooks {
	return SubjectHooks{
		CanAppeal: func(ctx context.Context, userID, entityID pgtype.UUID) (string, error) {
			row, err := s.read.GetOperationBanForAppeal(ctx, entityID)
			if err != nil {
				if errors.Is(err, pgx.ErrNoRows) {
					return "", ErrNotEligible
				}
				return "", fmt.Errorf("fetching operation ban: %w", err)
			}
			if row.UserID != userID {
				return "", ErrNotEligible
			}
			return "Restricted action: " + row.Operation, nil
		},
		OnOverturn: func(ctx context.Context, a Appeal, _ pgtype.UUID) error {
			var id pgtype.UUID
			if err := id.Scan(a.EntityID); err != nil {
				return err
			}
			row, err := s.read.GetOperationBanForAppeal(ctx, id)
			if err != nil {
				if errors.Is(err, pgx.ErrNoRows) {
					return nil // already lifted/expired
				}
				return err
			}
			if _, err := s.write.LiftOperationBanByID(ctx, id); err != nil {
				return err
			}
			remaining, err := s.read.CountActiveOperationBans(ctx, row.UserID)
			if err != nil {
				return err
			}
			if remaining == 0 {
				// Only a 'restricted' user is reactivated — status set by
				// op-bans. 'banned'/'suspended'/'deleted' keep theirs:
				// those are separate subjects with their own appeal path,
				// so an op-ban overturn must not unsuspend or unban.
				status, serr := s.read.GetUserStatus(ctx, row.UserID)
				if serr != nil {
					return serr
				}
				if status == "restricted" {
					if err := s.write.UpdateUserStatus(ctx, gen.UpdateUserStatusParams{
						ID:     row.UserID,
						Status: "active",
					}); err != nil {
						return err
					}
				}
			}
			if s.invalidateUserAccess != nil {
				s.invalidateUserAccess(ctx, row.UserID)
			}
			return nil
		},
		ListEligible: func(ctx context.Context, userID pgtype.UUID) ([]EligibleItem, error) {
			rows, err := s.read.ListAppealableOperationBans(ctx, userID)
			if err != nil {
				return nil, err
			}
			out := make([]EligibleItem, 0, len(rows))
			for _, r := range rows {
				it := EligibleItem{
					EntityID: uuidStr(r.ID),
					Label:    "Restricted action: " + r.Operation,
				}
				if r.Reason != nil {
					it.Detail = *r.Reason
				}
				out = append(out, it)
			}
			return out, nil
		},
	}
}

// commentSubject covers moderator-removed comments (comments.status =
// 'removed'): author-only. Overturn restores status='published'.
func (s *Service) commentSubject() SubjectHooks {
	return SubjectHooks{
		CanAppeal: func(ctx context.Context, userID, entityID pgtype.UUID) (string, error) {
			row, err := s.read.GetCommentForAppeal(ctx, entityID)
			if err != nil {
				if errors.Is(err, pgx.ErrNoRows) {
					return "", ErrNotEligible
				}
				return "", fmt.Errorf("fetching comment: %w", err)
			}
			if row.AuthorUserID != userID || row.Status != "removed" {
				return "", ErrNotEligible
			}
			return namedLabel("Removed comment", row.Excerpt), nil
		},
		OnOverturn: func(ctx context.Context, a Appeal, _ pgtype.UUID) error {
			var id pgtype.UUID
			if err := id.Scan(a.EntityID); err != nil {
				return err
			}
			row, err := s.write.RestoreCommentOnAppeal(ctx, id)
			if err != nil {
				if errors.Is(err, pgx.ErrNoRows) {
					return nil // already restored or deleted
				}
				return err
			}
			// RemoveComment decremented the denormalized counters — restore
			// them so the comment is counted again. Fire-and-forget like the
			// create path (a failure only skews the count, never blocks).
			if err := s.write.IncrementContentCommentCount(ctx, row.ContentID); err != nil {
				slog.Warn("appeals: comment restore count increment failed",
					"content_id", row.ContentID.String(), "error", err)
			}
			if row.ParentID.Valid {
				if err := s.write.IncrementCommentReplyCount(ctx, row.ParentID); err != nil {
					slog.Warn("appeals: comment reply_count increment failed",
						"parent_id", row.ParentID.String(), "error", err)
				}
			}
			return nil
		},
		ListEligible: func(ctx context.Context, userID pgtype.UUID) ([]EligibleItem, error) {
			rows, err := s.read.ListAppealableRemovedComments(ctx, userID)
			if err != nil {
				return nil, err
			}
			out := make([]EligibleItem, 0, len(rows))
			for _, r := range rows {
				out = append(out, EligibleItem{
					EntityID: uuidStr(r.ID),
					Label:    "Removed comment",
					Detail:   r.Excerpt,
				})
			}
			return out, nil
		},
	}
}

// mediaSubject covers rejected/quarantined media uploads: uploader-only.
// Overturn confirms the asset and clears the rejection/quarantine markers.
func (s *Service) mediaSubject() SubjectHooks {
	return SubjectHooks{
		CanAppeal: func(ctx context.Context, userID, entityID pgtype.UUID) (string, error) {
			row, err := s.read.GetMediaForAppeal(ctx, entityID)
			if err != nil {
				if errors.Is(err, pgx.ErrNoRows) {
					return "", ErrNotEligible
				}
				return "", fmt.Errorf("fetching media: %w", err)
			}
			appealable := !row.Confirmed &&
				(row.RejectedAt.Valid || (row.QuarantineReason != nil && *row.QuarantineReason != ""))
			if row.UploaderUserID != userID || !appealable {
				return "", ErrNotEligible
			}
			return namedLabel("Media under review hold", row.OriginalFilename), nil
		},
		OnOverturn: func(ctx context.Context, a Appeal, reviewedBy pgtype.UUID) error {
			var id pgtype.UUID
			if err := id.Scan(a.EntityID); err != nil {
				return err
			}
			_, err := s.write.ApproveMediaOnAppeal(ctx, gen.ApproveMediaOnAppealParams{
				ID:         id,
				ReviewedBy: reviewedBy,
			})
			return err
		},
		ListEligible: func(ctx context.Context, userID pgtype.UUID) ([]EligibleItem, error) {
			rows, err := s.read.ListAppealableMedia(ctx, userID)
			if err != nil {
				return nil, err
			}
			out := make([]EligibleItem, 0, len(rows))
			for _, r := range rows {
				out = append(out, EligibleItem{
					EntityID: uuidStr(r.ID),
					Label:    "Media under review hold: " + r.OriginalFilename,
				})
			}
			return out, nil
		},
	}
}

// wikiRevisionSubject covers rejected wiki revision submissions:
// submitter-only. Overturn returns the revision to pending for a fresh
// review (ops application is not re-run on appeal).
func (s *Service) wikiRevisionSubject() SubjectHooks {
	return SubjectHooks{
		CanAppeal: func(ctx context.Context, userID, entityID pgtype.UUID) (string, error) {
			row, err := s.read.GetWikiRevisionForAppeal(ctx, entityID)
			if err != nil {
				if errors.Is(err, pgx.ErrNoRows) {
					return "", ErrNotEligible
				}
				return "", fmt.Errorf("fetching wiki revision: %w", err)
			}
			if row.SubmittedBy != userID || row.Status != "rejected" {
				return "", ErrNotEligible
			}
			return namedLabelPtr("Rejected wiki edit", row.PageTitle), nil
		},
		OnOverturn: func(ctx context.Context, a Appeal, _ pgtype.UUID) error {
			var id pgtype.UUID
			if err := id.Scan(a.EntityID); err != nil {
				return err
			}
			_, err := s.write.ReopenWikiRevisionOnAppeal(ctx, id)
			return err
		},
		ListEligible: func(ctx context.Context, userID pgtype.UUID) ([]EligibleItem, error) {
			rows, err := s.read.ListAppealableWikiRevisions(ctx, userID)
			if err != nil {
				return nil, err
			}
			out := make([]EligibleItem, 0, len(rows))
			for _, r := range rows {
				label := "Rejected wiki edit"
				if r.PageTitle != nil && *r.PageTitle != "" {
					label = "Rejected wiki edit — " + *r.PageTitle
				}
				out = append(out, EligibleItem{EntityID: uuidStr(r.ID), Label: label})
			}
			return out, nil
		},
	}
}

// contentReportSubject covers reporter appeals of dismissed content
// reports (§51): reporter-only, dismissed only. Overturn reopens the
// report to pending for a fresh review.
func (s *Service) contentReportSubject() SubjectHooks {
	return SubjectHooks{
		CanAppeal: func(ctx context.Context, userID, entityID pgtype.UUID) (string, error) {
			row, err := s.read.GetContentReportForAppeal(ctx, entityID)
			if err != nil {
				if errors.Is(err, pgx.ErrNoRows) {
					return "", ErrNotEligible
				}
				return "", fmt.Errorf("fetching report: %w", err)
			}
			if row.ReporterUserID != userID || row.Status != "dismissed" {
				return "", ErrNotEligible
			}
			return fmt.Sprintf("Dismissed %s report", row.EntityType), nil
		},
		OnOverturn: func(ctx context.Context, a Appeal, _ pgtype.UUID) error {
			var id pgtype.UUID
			if err := id.Scan(a.EntityID); err != nil {
				return err
			}
			_, err := s.write.ReopenContentReportOnAppeal(ctx, id)
			return err
		},
		ListEligible: func(ctx context.Context, userID pgtype.UUID) ([]EligibleItem, error) {
			rows, err := s.read.ListAppealableContentReports(ctx, userID)
			if err != nil {
				return nil, err
			}
			out := make([]EligibleItem, 0, len(rows))
			for _, r := range rows {
				out = append(out, EligibleItem{
					EntityID: uuidStr(r.ID),
					Label:    fmt.Sprintf("Dismissed %s report", r.EntityType),
				})
			}
			return out, nil
		},
	}
}

// scamReportSubject covers reporter appeals of scam reports marked
// debunked/duplicate. Overturn returns the report to under_review.
func (s *Service) scamReportSubject() SubjectHooks {
	return SubjectHooks{
		CanAppeal: func(ctx context.Context, userID, entityID pgtype.UUID) (string, error) {
			row, err := s.read.GetScamReportForAppeal(ctx, entityID)
			if err != nil {
				if errors.Is(err, pgx.ErrNoRows) {
					return "", ErrNotEligible
				}
				return "", fmt.Errorf("fetching scam report: %w", err)
			}
			if row.ReporterID != userID || (row.Status != "debunked" && row.Status != "duplicate") {
				return "", ErrNotEligible
			}
			return "Closed scam report — " + row.ScamType, nil
		},
		OnOverturn: func(ctx context.Context, a Appeal, _ pgtype.UUID) error {
			var id pgtype.UUID
			if err := id.Scan(a.EntityID); err != nil {
				return err
			}
			_, err := s.write.ReopenScamReportOnAppeal(ctx, id)
			return err
		},
		ListEligible: func(ctx context.Context, userID pgtype.UUID) ([]EligibleItem, error) {
			rows, err := s.read.ListAppealableScamReports(ctx, userID)
			if err != nil {
				return nil, err
			}
			out := make([]EligibleItem, 0, len(rows))
			for _, r := range rows {
				out = append(out, EligibleItem{
					EntityID: uuidStr(r.ID),
					Label:    "Closed scam report — " + r.ScamType,
				})
			}
			return out, nil
		},
	}
}

// academyInstructorSubject covers rejected/suspended academy instructor
// applications: owner-only. Overturn returns the profile to pending for
// re-review (not auto-approve — KYC/tier checks still apply).
func (s *Service) academyInstructorSubject() SubjectHooks {
	return SubjectHooks{
		CanAppeal: func(ctx context.Context, userID, entityID pgtype.UUID) (string, error) {
			row, err := s.read.GetAcademyInstructorForAppeal(ctx, entityID)
			if err != nil {
				if errors.Is(err, pgx.ErrNoRows) {
					return "", ErrNotEligible
				}
				return "", fmt.Errorf("fetching instructor profile: %w", err)
			}
			if row.UserID != userID || (row.Status != "rejected" && row.Status != "suspended") {
				return "", ErrNotEligible
			}
			return "Instructor application", nil
		},
		OnOverturn: func(ctx context.Context, a Appeal, _ pgtype.UUID) error {
			var id pgtype.UUID
			if err := id.Scan(a.EntityID); err != nil {
				return err
			}
			_, err := s.write.ReopenAcademyInstructorOnAppeal(ctx, id)
			return err
		},
		ListEligible: func(ctx context.Context, userID pgtype.UUID) ([]EligibleItem, error) {
			rows, err := s.read.ListAppealableAcademyInstructor(ctx, userID)
			if err != nil {
				return nil, err
			}
			out := make([]EligibleItem, 0, len(rows))
			for _, r := range rows {
				out = append(out, EligibleItem{
					EntityID: uuidStr(r),
					Label:    "Instructor application rejected/suspended",
				})
			}
			return out, nil
		},
	}
}

// reviewSubject covers moderator-removed reviews (reviews.status='removed'
// + removed_by set). Author-only; overturn republishes and recomputes the
// denormalized aggregate (best-effort, same as the flag/remove paths).
func (s *Service) reviewSubject() SubjectHooks {
	return SubjectHooks{
		CanAppeal: func(ctx context.Context, userID, entityID pgtype.UUID) (string, error) {
			row, err := s.read.GetReviewForAppeal(ctx, entityID)
			if err != nil {
				if errors.Is(err, pgx.ErrNoRows) {
					return "", ErrNotEligible
				}
				return "", fmt.Errorf("fetching review: %w", err)
			}
			if row.ReviewerUserID != userID {
				return "", ErrNotEligible
			}
			return namedLabelPtr("Removed review", row.Title), nil
		},
		OnOverturn: func(ctx context.Context, a Appeal, _ pgtype.UUID) error {
			var id pgtype.UUID
			if err := id.Scan(a.EntityID); err != nil {
				return err
			}
			row, err := s.write.RestoreReviewOnAppeal(ctx, id)
			if errors.Is(err, pgx.ErrNoRows) {
				return nil // already re-moderated — no-op like other hooks
			}
			if err != nil {
				return err
			}
			if _, aggErr := s.write.UpsertReviewAggregate(ctx, gen.UpsertReviewAggregateParams{
				EntityType: row.EntityType,
				EntityID:   row.EntityID,
			}); aggErr != nil {
				slog.Error("appeals: review aggregate recompute failed",
					"review_id", a.EntityID, "error", aggErr)
			}
			return nil
		},
		ListEligible: func(ctx context.Context, userID pgtype.UUID) ([]EligibleItem, error) {
			rows, err := s.read.ListAppealableReviews(ctx, userID)
			if err != nil {
				return nil, err
			}
			out := make([]EligibleItem, 0, len(rows))
			for _, r := range rows {
				label := "Removed review"
				if r.Title != nil && *r.Title != "" {
					label = "Removed review — " + *r.Title
				}
				out = append(out, EligibleItem{EntityID: uuidStr(r.ID), Label: label})
			}
			return out, nil
		},
	}
}

// mutualAidPostSubject covers posts removed by moderation. Author-only;
// overturn returns the post to 'active'.
func (s *Service) mutualAidPostSubject() SubjectHooks {
	return SubjectHooks{
		CanAppeal: func(ctx context.Context, userID, entityID pgtype.UUID) (string, error) {
			row, err := s.read.GetMutualAidPostForAppeal(ctx, entityID)
			if err != nil {
				if errors.Is(err, pgx.ErrNoRows) {
					return "", ErrNotEligible
				}
				return "", fmt.Errorf("fetching mutual aid post: %w", err)
			}
			if row.UserID != userID || row.Status != "removed" {
				return "", ErrNotEligible
			}
			return namedLabel("Removed mutual aid post", row.Title), nil
		},
		OnOverturn: func(ctx context.Context, a Appeal, _ pgtype.UUID) error {
			var id pgtype.UUID
			if err := id.Scan(a.EntityID); err != nil {
				return err
			}
			_, err := s.write.RestoreMutualAidPostOnAppeal(ctx, id)
			return err
		},
		ListEligible: func(ctx context.Context, userID pgtype.UUID) ([]EligibleItem, error) {
			rows, err := s.read.ListAppealableMutualAidPosts(ctx, userID)
			if err != nil {
				return nil, err
			}
			out := make([]EligibleItem, 0, len(rows))
			for _, r := range rows {
				out = append(out, EligibleItem{
					EntityID: uuidStr(r.ID),
					Label:    "Removed mutual aid post — " + r.Title,
				})
			}
			return out, nil
		},
	}
}

// videoSourceSubject covers sources hidden by an approved takedown claim
// (status='removed'). Author-only; overturn returns the source to 'active'.
func (s *Service) videoSourceSubject() SubjectHooks {
	return SubjectHooks{
		CanAppeal: func(ctx context.Context, userID, entityID pgtype.UUID) (string, error) {
			row, err := s.read.GetVideoSourceForAppeal(ctx, entityID)
			if err != nil {
				if errors.Is(err, pgx.ErrNoRows) {
					return "", ErrNotEligible
				}
				return "", fmt.Errorf("fetching video source: %w", err)
			}
			if row.AuthorUserID != userID || row.Status != "removed" {
				return "", ErrNotEligible
			}
			return "removed video — " + row.Title, nil
		},
		OnOverturn: func(ctx context.Context, a Appeal, _ pgtype.UUID) error {
			var id pgtype.UUID
			if err := id.Scan(a.EntityID); err != nil {
				return err
			}
			_, err := s.write.RestoreVideoSourceOnAppeal(ctx, id)
			return err
		},
		ListEligible: func(ctx context.Context, userID pgtype.UUID) ([]EligibleItem, error) {
			rows, err := s.read.ListAppealableVideoSources(ctx, userID)
			if err != nil {
				return nil, err
			}
			out := make([]EligibleItem, 0, len(rows))
			for _, r := range rows {
				out = append(out, EligibleItem{
					EntityID: uuidStr(r.ID),
					Label:    "Removed video — " + r.Title,
				})
			}
			return out, nil
		},
	}
}

// roadmapItemSubject covers declined roadmap suggestions. Submitter-only;
// overturn re-enters the item into 'under_review'.
func (s *Service) roadmapItemSubject() SubjectHooks {
	return SubjectHooks{
		CanAppeal: func(ctx context.Context, userID, entityID pgtype.UUID) (string, error) {
			row, err := s.read.GetRoadmapItemForAppeal(ctx, entityID)
			if err != nil {
				if errors.Is(err, pgx.ErrNoRows) {
					return "", ErrNotEligible
				}
				return "", fmt.Errorf("fetching roadmap item: %w", err)
			}
			if row.SubmittedBy != userID || row.Status != "declined" {
				return "", ErrNotEligible
			}
			return "declined roadmap item — " + row.Title, nil
		},
		OnOverturn: func(ctx context.Context, a Appeal, _ pgtype.UUID) error {
			var id pgtype.UUID
			if err := id.Scan(a.EntityID); err != nil {
				return err
			}
			_, err := s.write.ReopenRoadmapItemOnAppeal(ctx, id)
			return err
		},
		ListEligible: func(ctx context.Context, userID pgtype.UUID) ([]EligibleItem, error) {
			rows, err := s.read.ListAppealableRoadmapItems(ctx, userID)
			if err != nil {
				return nil, err
			}
			out := make([]EligibleItem, 0, len(rows))
			for _, r := range rows {
				out = append(out, EligibleItem{
					EntityID: uuidStr(r.ID),
					Label:    "Declined roadmap item — " + r.Title,
				})
			}
			return out, nil
		},
	}
}

// organizationSubject covers suspended or soft-deleted organizations.
// Owner-only; overturn restores 'active' and clears deleted_at.
func (s *Service) organizationSubject() SubjectHooks {
	return SubjectHooks{
		CanAppeal: func(ctx context.Context, userID, entityID pgtype.UUID) (string, error) {
			row, err := s.read.GetOrganizationForAppeal(ctx, entityID)
			if err != nil {
				if errors.Is(err, pgx.ErrNoRows) {
					return "", ErrNotEligible
				}
				return "", fmt.Errorf("fetching organization: %w", err)
			}
			if row.OwnerUserID != userID || (row.Status != "suspended" && !row.DeletedAt.Valid) {
				return "", ErrNotEligible
			}
			return "organization — " + row.Name, nil
		},
		OnOverturn: func(ctx context.Context, a Appeal, _ pgtype.UUID) error {
			var id pgtype.UUID
			if err := id.Scan(a.EntityID); err != nil {
				return err
			}
			_, err := s.write.RestoreOrganizationOnAppeal(ctx, id)
			return err
		},
		ListEligible: func(ctx context.Context, userID pgtype.UUID) ([]EligibleItem, error) {
			rows, err := s.read.ListAppealableOrganizations(ctx, userID)
			if err != nil {
				return nil, err
			}
			out := make([]EligibleItem, 0, len(rows))
			for _, r := range rows {
				out = append(out, EligibleItem{
					EntityID: uuidStr(r.ID),
					Label:    "Suspended/removed organization — " + r.Name,
				})
			}
			return out, nil
		},
	}
}

// spaceSubject covers suspended or soft-deleted spaces. The creator or an
// 'owner'-role member may appeal; overturn restores 'active' + undeletes.
func (s *Service) spaceSubject() SubjectHooks {
	return SubjectHooks{
		CanAppeal: func(ctx context.Context, userID, entityID pgtype.UUID) (string, error) {
			row, err := s.read.GetSpaceForAppeal(ctx, entityID)
			if err != nil {
				if errors.Is(err, pgx.ErrNoRows) {
					return "", ErrNotEligible
				}
				return "", fmt.Errorf("fetching space: %w", err)
			}
			if row.Status != "suspended" && !row.DeletedAt.Valid {
				return "", ErrNotEligible
			}
			if row.CreatorUserID == userID {
				return "space — " + row.Name, nil
			}
			owner, err := s.read.IsSpaceOwnerMember(ctx, gen.IsSpaceOwnerMemberParams{
				SpaceID: entityID,
				UserID:  userID,
			})
			if err != nil {
				return "", fmt.Errorf("checking space membership: %w", err)
			}
			if !owner {
				return "", ErrNotEligible
			}
			return "space — " + row.Name, nil
		},
		OnOverturn: func(ctx context.Context, a Appeal, _ pgtype.UUID) error {
			var id pgtype.UUID
			if err := id.Scan(a.EntityID); err != nil {
				return err
			}
			_, err := s.write.RestoreSpaceOnAppeal(ctx, id)
			return err
		},
		ListEligible: func(ctx context.Context, userID pgtype.UUID) ([]EligibleItem, error) {
			rows, err := s.read.ListAppealableSpaces(ctx, userID)
			if err != nil {
				return nil, err
			}
			out := make([]EligibleItem, 0, len(rows))
			for _, r := range rows {
				out = append(out, EligibleItem{
					EntityID: uuidStr(r.ID),
					Label:    "Suspended/removed space — " + r.Name,
				})
			}
			return out, nil
		},
	}
}

// academyTrackSubject covers rejected academy submissions. The track's
// instructor (via academy_instructor_profiles.user_id) appeals; overturn
// returns the submission to 'pending'.
func (s *Service) academyTrackSubject() SubjectHooks {
	return SubjectHooks{
		CanAppeal: func(ctx context.Context, userID, entityID pgtype.UUID) (string, error) {
			row, err := s.read.GetAcademyTrackForAppeal(ctx, entityID)
			if err != nil {
				if errors.Is(err, pgx.ErrNoRows) {
					return "", ErrNotEligible
				}
				return "", fmt.Errorf("fetching academy track: %w", err)
			}
			if row.InstructorUserID != userID || row.AcademySubmissionStatus != "rejected" {
				return "", ErrNotEligible
			}
			return "rejected academy track — " + row.Title, nil
		},
		OnOverturn: func(ctx context.Context, a Appeal, _ pgtype.UUID) error {
			var id pgtype.UUID
			if err := id.Scan(a.EntityID); err != nil {
				return err
			}
			_, err := s.write.ReopenAcademyTrackOnAppeal(ctx, id)
			return err
		},
		ListEligible: func(ctx context.Context, userID pgtype.UUID) ([]EligibleItem, error) {
			rows, err := s.read.ListAppealableAcademyTracks(ctx, userID)
			if err != nil {
				return nil, err
			}
			out := make([]EligibleItem, 0, len(rows))
			for _, r := range rows {
				out = append(out, EligibleItem{
					EntityID: uuidStr(r.ID),
					Label:    "Rejected academy track — " + r.Title,
				})
			}
			return out, nil
		},
	}
}

// affiliateMemberSubject covers suspended/banned affiliate memberships.
// entity_id IS the member's user_id; overturn returns to 'active'.
func (s *Service) affiliateMemberSubject() SubjectHooks {
	return SubjectHooks{
		CanAppeal: func(ctx context.Context, userID, entityID pgtype.UUID) (string, error) {
			if entityID != userID {
				return "", ErrNotEligible
			}
			row, err := s.read.GetAffiliateMemberForAppeal(ctx, entityID)
			if err != nil {
				if errors.Is(err, pgx.ErrNoRows) {
					return "", ErrNotEligible
				}
				return "", fmt.Errorf("fetching affiliate membership: %w", err)
			}
			return row.Status + " affiliate membership", nil
		},
		OnOverturn: func(ctx context.Context, a Appeal, _ pgtype.UUID) error {
			var id pgtype.UUID
			if err := id.Scan(a.EntityID); err != nil {
				return err
			}
			_, err := s.write.RestoreAffiliateMemberOnAppeal(ctx, id)
			return err
		},
		ListEligible: func(ctx context.Context, userID pgtype.UUID) ([]EligibleItem, error) {
			row, err := s.read.GetAffiliateMemberForAppeal(ctx, userID)
			if err != nil {
				if errors.Is(err, pgx.ErrNoRows) {
					return nil, nil
				}
				return nil, err
			}
			return []EligibleItem{{
				EntityID: uuidStr(row.UserID),
				Label:    row.Status + " affiliate membership",
			}}, nil
		},
	}
}

// referralSubject covers rejected referrals (fraud-review decisions).
// Referrer-only; overturn returns the referral to 'pending' for a fresh
// reward review.
func (s *Service) referralSubject() SubjectHooks {
	return SubjectHooks{
		CanAppeal: func(ctx context.Context, userID, entityID pgtype.UUID) (string, error) {
			row, err := s.read.GetReferralForAppeal(ctx, entityID)
			if err != nil {
				if errors.Is(err, pgx.ErrNoRows) {
					return "", ErrNotEligible
				}
				return "", fmt.Errorf("fetching referral: %w", err)
			}
			if row.ReferrerID != userID || row.Status != "rejected" {
				return "", ErrNotEligible
			}
			return namedLabel("Rejected referral", row.RefereeName), nil
		},
		OnOverturn: func(ctx context.Context, a Appeal, _ pgtype.UUID) error {
			var id pgtype.UUID
			if err := id.Scan(a.EntityID); err != nil {
				return err
			}
			_, err := s.write.ReopenReferralOnAppeal(ctx, id)
			return err
		},
		ListEligible: func(ctx context.Context, userID pgtype.UUID) ([]EligibleItem, error) {
			rows, err := s.read.ListAppealableReferrals(ctx, userID)
			if err != nil {
				return nil, err
			}
			out := make([]EligibleItem, 0, len(rows))
			for _, r := range rows {
				out = append(out, EligibleItem{
					EntityID: uuidStr(r.ID),
					Label:    namedLabel("Rejected referral", r.RefereeName),
				})
			}
			return out, nil
		},
	}
}

// withdrawalSubject covers rejected wallet withdrawals. Owner-only. The
// overturn hook is registered by the server from internal/wallet —
// rejecting a withdrawal refunds its credits, so overturning must
// re-reserve them inside the wallet's ledger transaction rather than
// flipping status here.
func (s *Service) withdrawalSubject() SubjectHooks {
	return SubjectHooks{
		CanAppeal: func(ctx context.Context, userID, entityID pgtype.UUID) (string, error) {
			row, err := s.read.GetWithdrawalForAppeal(ctx, entityID)
			if err != nil {
				if errors.Is(err, pgx.ErrNoRows) {
					return "", ErrNotEligible
				}
				return "", fmt.Errorf("fetching withdrawal: %w", err)
			}
			if row.UserID != userID || row.Status != "rejected" {
				return "", ErrNotEligible
			}
			return fmt.Sprintf("rejected withdrawal — %d credits", row.CreditAmount), nil
		},
		ListEligible: func(ctx context.Context, userID pgtype.UUID) ([]EligibleItem, error) {
			rows, err := s.read.ListAppealableWithdrawals(ctx, userID)
			if err != nil {
				return nil, err
			}
			out := make([]EligibleItem, 0, len(rows))
			for _, r := range rows {
				out = append(out, EligibleItem{
					EntityID: uuidStr(r.ID),
					Label:    fmt.Sprintf("Rejected withdrawal — %d credits", r.CreditAmount),
				})
			}
			return out, nil
		},
	}
}

// adCampaignSubject covers rejected ad campaigns. The creator or a member
// of the owning organization may appeal; overturn returns the campaign to
// 'pending_review'.
func (s *Service) adCampaignSubject() SubjectHooks {
	return SubjectHooks{
		CanAppeal: func(ctx context.Context, userID, entityID pgtype.UUID) (string, error) {
			row, err := s.read.GetAdCampaignForAppeal(ctx, entityID)
			if err != nil {
				if errors.Is(err, pgx.ErrNoRows) {
					return "", ErrNotEligible
				}
				return "", fmt.Errorf("fetching ad campaign: %w", err)
			}
			if row.Status != "rejected" {
				return "", ErrNotEligible
			}
			if row.CreatedBy == userID {
				return "rejected ad campaign — " + row.Name, nil
			}
			if row.OrganizationID.Valid {
				member, err := s.read.IsOrganizationMember(ctx, gen.IsOrganizationMemberParams{
					OrganizationID: row.OrganizationID,
					UserID:         userID,
				})
				if err != nil {
					return "", fmt.Errorf("checking org membership: %w", err)
				}
				if member {
					return "rejected ad campaign — " + row.Name, nil
				}
			}
			return "", ErrNotEligible
		},
		// OnOverturn is replaced at wiring time (server.go) by
		// adsselfserve.ReopenCampaignOnAppeal, which re-escrows the refunded
		// budget_credits under the wallet advisory lock. This fallback keeps
		// whatever budget_credits is still on the row (non-org campaigns and
		// never-refunded rejections lose nothing).
		OnOverturn: func(ctx context.Context, a Appeal, _ pgtype.UUID) error {
			var id pgtype.UUID
			if err := id.Scan(a.EntityID); err != nil {
				return err
			}
			row, err := s.read.GetAdCampaignForAppeal(ctx, id)
			if err != nil {
				return fmt.Errorf("fetching ad campaign: %w", err)
			}
			_, err = s.write.ReopenAdCampaignOnAppeal(ctx, gen.ReopenAdCampaignOnAppealParams{
				ID:            id,
				BudgetCredits: row.BudgetCredits,
			})
			return err
		},
		ListEligible: func(ctx context.Context, userID pgtype.UUID) ([]EligibleItem, error) {
			rows, err := s.read.ListAppealableAdCampaigns(ctx, userID)
			if err != nil {
				return nil, err
			}
			out := make([]EligibleItem, 0, len(rows))
			for _, r := range rows {
				out = append(out, EligibleItem{
					EntityID: uuidStr(r.ID),
					Label:    "Rejected ad campaign — " + r.Name,
				})
			}
			return out, nil
		},
	}
}

// contentReviewSubject covers rejected editorial reviews. The content's
// author (not the reviewer) appeals; overturn returns the review to
// 'pending'.
func (s *Service) contentReviewSubject() SubjectHooks {
	return SubjectHooks{
		CanAppeal: func(ctx context.Context, userID, entityID pgtype.UUID) (string, error) {
			row, err := s.read.GetContentReviewForAppeal(ctx, entityID)
			if err != nil {
				if errors.Is(err, pgx.ErrNoRows) {
					return "", ErrNotEligible
				}
				return "", fmt.Errorf("fetching content review: %w", err)
			}
			if row.AuthorID != userID || row.Status != "rejected" {
				return "", ErrNotEligible
			}
			return namedLabel("Rejected editorial review", row.Title), nil
		},
		OnOverturn: func(ctx context.Context, a Appeal, _ pgtype.UUID) error {
			var id pgtype.UUID
			if err := id.Scan(a.EntityID); err != nil {
				return err
			}
			_, err := s.write.ReopenContentReviewOnAppeal(ctx, id)
			return err
		},
		ListEligible: func(ctx context.Context, userID pgtype.UUID) ([]EligibleItem, error) {
			rows, err := s.read.ListAppealableContentReviews(ctx, userID)
			if err != nil {
				return nil, err
			}
			out := make([]EligibleItem, 0, len(rows))
			for _, r := range rows {
				label := "Rejected editorial review"
				if r.Title != "" {
					label = "Rejected editorial review — " + r.Title
				}
				out = append(out, EligibleItem{EntityID: uuidStr(r.ID), Label: label})
			}
			return out, nil
		},
	}
}

// communityOrgSubject covers deleted or archived community-org
// submissions. Submitter-only; overturn returns the record to 'pending'
// for a fresh moderation pass.
func (s *Service) communityOrgSubject() SubjectHooks {
	return SubjectHooks{
		CanAppeal: func(ctx context.Context, userID, entityID pgtype.UUID) (string, error) {
			row, err := s.read.GetCommunityOrgForAppeal(ctx, entityID)
			if err != nil {
				if errors.Is(err, pgx.ErrNoRows) {
					return "", ErrNotEligible
				}
				return "", fmt.Errorf("fetching community organisation: %w", err)
			}
			if row.SubmittedBy != userID || (!row.DeletedAt.Valid && row.Status != "archived") {
				return "", ErrNotEligible
			}
			return "community organisation — " + row.Name, nil
		},
		OnOverturn: func(ctx context.Context, a Appeal, _ pgtype.UUID) error {
			var id pgtype.UUID
			if err := id.Scan(a.EntityID); err != nil {
				return err
			}
			_, err := s.write.RestoreCommunityOrgOnAppeal(ctx, id)
			return err
		},
		ListEligible: func(ctx context.Context, userID pgtype.UUID) ([]EligibleItem, error) {
			rows, err := s.read.ListAppealableCommunityOrgs(ctx, userID)
			if err != nil {
				return nil, err
			}
			out := make([]EligibleItem, 0, len(rows))
			for _, r := range rows {
				out = append(out, EligibleItem{
					EntityID: uuidStr(r.ID),
					Label:    "Removed community organisation — " + r.Name,
				})
			}
			return out, nil
		},
	}
}

// communityOrgClaimSubject covers rejected community-org claims. Only the
// recorded claimant (claimed_by_user_id) may appeal while
// claim_rejected_at is set — claim_status itself resets to 'unclaimed' so
// another claimant can still try. Overturn approves the claim.
func (s *Service) communityOrgClaimSubject() SubjectHooks {
	return SubjectHooks{
		CanAppeal: func(ctx context.Context, userID, entityID pgtype.UUID) (string, error) {
			row, err := s.read.GetCommunityOrgClaimForAppeal(ctx, entityID)
			if err != nil {
				if errors.Is(err, pgx.ErrNoRows) {
					return "", ErrNotEligible
				}
				return "", fmt.Errorf("fetching community org claim: %w", err)
			}
			if !row.ClaimedByUserID.Valid || row.ClaimedByUserID != userID {
				return "", ErrNotEligible
			}
			return "rejected community organisation claim — " + row.Name, nil
		},
		OnOverturn: func(ctx context.Context, a Appeal, _ pgtype.UUID) error {
			var id pgtype.UUID
			if err := id.Scan(a.EntityID); err != nil {
				return err
			}
			_, err := s.write.ApproveCommunityOrgClaimOnAppeal(ctx, id)
			return err
		},
		ListEligible: func(ctx context.Context, userID pgtype.UUID) ([]EligibleItem, error) {
			rows, err := s.read.ListAppealableCommunityOrgClaims(ctx, userID)
			if err != nil {
				return nil, err
			}
			out := make([]EligibleItem, 0, len(rows))
			for _, r := range rows {
				out = append(out, EligibleItem{
					EntityID: uuidStr(r.ID),
					Label:    "Rejected community organisation claim — " + r.Name,
				})
			}
			return out, nil
		},
	}
}
