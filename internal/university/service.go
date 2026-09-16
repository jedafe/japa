package university

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/valkey-io/valkey-go/valkeycompat"

	"github.com/jedafe/japa/internal/config"
	"github.com/jedafe/japa/internal/db/gen"
	"github.com/jedafe/japa/internal/valkey"
	"github.com/jedafe/japa/internal/worker"
)

// TaskEnqueuer is the minimal interface for enqueueing Asynq tasks.
// The worker.Enqueuer satisfies this.
type TaskEnqueuer interface {
	Enqueue(taskType string, payload []byte, opts ...interface{}) error
}

// MediaUploader downloads content from a URL, stores it to SeaweedFS, and
// returns the media ID. The media.Service implements this via StoreFromURL.
// Used by the university:fetch_logo task (Section 39).
type MediaUploader interface {
	StoreFromURL(ctx context.Context, uploaderID pgtype.UUID, sourceURL, filename string) (pgtype.UUID, error)
}

// SpaceLogoUpdater updates a space's logo_media_id. The spaces.Service or the
// generated Queries satisfy this via UpdateSpaceLogoMediaID.
type SpaceLogoUpdater interface {
	UpdateSpaceLogoMediaID(ctx context.Context, params gen.UpdateSpaceLogoMediaIDParams) (gen.UpdateSpaceLogoMediaIDRow, error)
}

// Cache TTLs for the university Valkey keys (valkey-keys-catalog.md).
const (
	instructorProfileTTL = 1 * time.Hour    // jc:university:instructor:{user_id}:profile
	trackEnrolledTTL     = 10 * time.Minute // jc:university:track:{track_id}:enrolled
	// payoutTTL has no fixed duration — it lives until the period closes.
	// We use a long TTL as a safety net; the key is invalidated on period close.
	payoutAccumulatorTTL = 7 * 24 * time.Hour // jc:university:payout:{the id}:{period}:{instructor_id}
)

// Service holds the business logic for the Japa University creator economy.
// It owns transaction boundaries, split-percentage resolution, and Asynq
// task enqueueing. Settings are read via the hot-reloadable config.Settings.
type Service struct {
	store            *Store
	settings         *config.Settings
	enqueuer         TaskEnqueuer
	rdb              valkeycompat.Cmdable // nil = caching disabled
	mediaUploader    MediaUploader        // nil = fetch_logo task skips media storage
	spaceLogoUpdater SpaceLogoUpdater     // nil = fetch_logo task skips space update
	notifier         Notifier             // nil = notify tasks log only
}

// Notifier dispatches in-app notifications for university events. Implemented
// by the notifications service; nil = the notify_* tasks log only (V1 stub).
type Notifier interface {
	CreateNotification(ctx context.Context, recipientID pgtype.UUID, nType, actorType string, actorID pgtype.UUID, subjectType string, subjectID pgtype.UUID, title, body, actionURL string) error
}

// NewService creates a university service.
func NewService(store *Store, settings *config.Settings, enqueuer TaskEnqueuer) *Service {
	return &Service{store: store, settings: settings, enqueuer: enqueuer}
}

// SetValkey wires the Valkey client for cache reads/writes. Optional — if
// not called, caching is disabled and all operations hit the DB.
// AUDIT-BE-VALKEY-007/008/009: enables the 3 university cache keys.
func (s *Service) SetValkey(rdb valkeycompat.Cmdable) { s.rdb = rdb }

// SetEnqueuer wires (or rewires) the Asynq enqueuer after construction.
func (s *Service) SetEnqueuer(e TaskEnqueuer) { s.enqueuer = e }

// SetNotifier wires the notification dispatcher for the notify_* tasks.
// Optional — if not called, notify_instructor_approved and
// notify_course_approved log only (V1 behavior).
func (s *Service) SetNotifier(n Notifier) { s.notifier = n }

// SetMediaUploader wires the media uploader for the university:fetch_logo
// task (Section 39). Optional — if not called, fetch_logo logs a warning
// and skips media storage.
func (s *Service) SetMediaUploader(m MediaUploader) { s.mediaUploader = m }

// SetSpaceLogoUpdater wires the space logo updater for the
// university:fetch_logo task (Section 39).
func (s *Service) SetSpaceLogoUpdater(u SpaceLogoUpdater) { s.spaceLogoUpdater = u }

// ---------------------------------------------------------------------------
// Instructor lifecycle
// ---------------------------------------------------------------------------

// Apply submits an instructor application.
func (s *Service) Apply(ctx context.Context, userID pgtype.UUID, in ApplyInput) (InstructorProfile, error) {
	if in.DisplayName == "" {
		return InstructorProfile{}, errors.New("display_name is required")
	}

	row, err := s.store.Apply(ctx, userID, in)
	if err != nil {
		return InstructorProfile{}, err
	}

	return profileFromRow(row), nil
}

// GetMyProfile returns the caller's instructor profile.
func (s *Service) GetMyProfile(ctx context.Context, userID pgtype.UUID) (InstructorProfile, error) {
	// AUDIT-BE-VALKEY-007: read-through cache (jc:university:instructor:{user_id}:profile).
	return s.cachedInstructorProfile(ctx, userID)
}

// UpdateMyProfile updates creator-editable fields.
func (s *Service) UpdateMyProfile(ctx context.Context, userID pgtype.UUID, in UpdateSelfInput) (InstructorProfile, error) {
	row, err := s.store.UpdateSelf(ctx, userID, in)
	if err != nil {
		return InstructorProfile{}, err
	}
	// AUDIT-BE-VALKEY-007: invalidate the cached profile on mutation.
	s.invalidateInstructorProfileCache(ctx, userID)

	return profileFromRow(row), nil
}

// ApproveInstructor transitions a pending application to approved and
// enqueues the approval notification.
func (s *Service) ApproveInstructor(ctx context.Context, instructorID, reviewer pgtype.UUID, tier string, commissionOverride *float64) (InstructorProfile, error) {
	if tier == "" {
		tier = "standard"
	}

	row, err := s.store.Approve(ctx, instructorID, reviewer, tier, commissionOverride)
	if err != nil {
		return InstructorProfile{}, err
	}
	// AUDIT-BE-VALKEY-007: invalidate the cached profile on status change.
	if row.UserID.Valid {
		s.invalidateInstructorProfileCache(ctx, row.UserID)
	}

	s.enqueueNotifyInstructorApproved(ctx, row.ID)

	return profileFromRow(row), nil
}

// RejectInstructor transitions a pending application to rejected.
func (s *Service) RejectInstructor(ctx context.Context, instructorID, reviewer pgtype.UUID) (InstructorProfile, error) {
	row, err := s.store.Reject(ctx, instructorID, reviewer)
	if err != nil {
		return InstructorProfile{}, err
	}
	// AUDIT-BE-VALKEY-007: invalidate the cached profile on status change.
	if row.UserID.Valid {
		s.invalidateInstructorProfileCache(ctx, row.UserID)
	}

	return profileFromRow(row), nil
}

// SuspendInstructor transitions an approved instructor to suspended.
func (s *Service) SuspendInstructor(ctx context.Context, instructorID, reviewer pgtype.UUID) (InstructorProfile, error) {
	row, err := s.store.Suspend(ctx, instructorID, reviewer)
	if err != nil {
		return InstructorProfile{}, err
	}
	// AUDIT-BE-VALKEY-007: invalidate the cached profile on status change.
	if row.UserID.Valid {
		s.invalidateInstructorProfileCache(ctx, row.UserID)
	}

	return profileFromRow(row), nil
}

// ReinstateInstructor transitions a suspended instructor back to approved.
func (s *Service) ReinstateInstructor(ctx context.Context, instructorID, reviewer pgtype.UUID) (InstructorProfile, error) {
	row, err := s.store.Reinstate(ctx, instructorID, reviewer)
	if err != nil {
		return InstructorProfile{}, err
	}
	// AUDIT-BE-VALKEY-007: invalidate the cached profile on status change.
	if row.UserID.Valid {
		s.invalidateInstructorProfileCache(ctx, row.UserID)
	}

	return profileFromRow(row), nil
}

// DeleteInstructor permanently removes an instructor profile/application.
// Blocked while status='approved' (reject or suspend the instructor
// first) so an active creator's identity cannot disappear out from under
// their live marketplace courses. Returns pgx.ErrNoRows if the profile
// does not exist or is currently approved.
func (s *Service) DeleteInstructor(ctx context.Context, instructorID pgtype.UUID) error {
	row, err := s.store.Delete(ctx, instructorID)
	if err != nil {
		return err
	}
	// AUDIT-BE-VALKEY-007: invalidate the cached profile on deletion.
	if row.UserID.Valid {
		s.invalidateInstructorProfileCache(ctx, row.UserID)
	}

	return nil
}

// ---------------------------------------------------------------------------
// Creator track management
// ---------------------------------------------------------------------------

// CreateTrack creates a marketplace track owned by an approved instructor.
func (s *Service) CreateTrack(ctx context.Context, instructorID pgtype.UUID, in CreateTrackInput) (gen.CreateUniversityTrackRow, error) {
	if in.Title == "" || in.Slug == "" {
		return gen.CreateUniversityTrackRow{}, errors.New("title and slug are required")
	}

	if in.Difficulty == "" {
		in.Difficulty = "beginner"
	}

	in.InstructorID = &instructorID

	return s.store.CreateTrack(ctx, in)
}

// UpdateTrack updates a creator's own track.
func (s *Service) UpdateTrack(ctx context.Context, trackID, instructorID pgtype.UUID, in UpdateTrackInput) (gen.UpdateUniversityTrackRow, error) {
	return s.store.UpdateTrack(ctx, trackID, instructorID, in)
}

// SubmitTrack submits a track for marketplace approval.
func (s *Service) SubmitTrack(ctx context.Context, trackID, instructorID pgtype.UUID) (gen.SubmitUniversityTrackRow, error) {
	return s.store.SubmitTrack(ctx, trackID, instructorID)
}

// ApproveTrack publishes a submitted track to the marketplace and enqueues
// the course-approved notification + search index sync.
func (s *Service) ApproveTrack(ctx context.Context, trackID pgtype.UUID) (gen.ApproveUniversityTrackRow, error) {
	row, err := s.store.ApproveTrack(ctx, trackID)
	if err != nil {
		return gen.ApproveUniversityTrackRow{}, err
	}

	s.enqueueNotifyCourseApproved(ctx, trackID)
	// Enqueue search index sync so the track appears in the marketplace index.
	s.enqueue(ctx, worker.TypeSearchIndexSync, map[string]string{
		"entity_id":   trackID.String(),
		"entity_type": "learning_track",
	})

	return row, nil
}

// ---------------------------------------------------------------------------
// Enrollment + revenue split
// ---------------------------------------------------------------------------

// ProcessEnrollment is the idempotent handler for university:process_enrollment.
// It is called after a course_access payment succeeds. It:
// 1. Guards against double-credit via HasSplitForPayment (AUDIT-BE-005).
// 2. Upserts the enrollment row with the payment link.
// 3. Increments the track's cached total_enrolled counter.
// 4. Writes an immutable revenue_splits row using the resolved split.
// // The split resolution order is: instructor override → tier default → system
// default. Pro-stream enrollments accrue at the configured stream rate with
// no payment link.
func (s *Service) ProcessEnrollment(ctx context.Context, userID, trackID, paymentID pgtype.UUID, source string, grossKobo int64) error {
	if source == "" {
		source = "purchase"
	}

	track, err := s.store.GetTrackByID(ctx, trackID)
	if err != nil {
		return fmt.Errorf("university: process enrollment: track lookup: %w", err)
	}

	if !track.InstructorID.Valid {
		// Platform-authored course — no creator split to record.
		return nil
	}

	// Idempotency: a paid enrollment is keyed on payment_id.
	// Pro-stream enrollments have no payment_id, so they are keyed on
	// (instructor_id, track_id, user_id, source='pro_stream') (BUG-008).
	if source == "purchase" && paymentID.Valid {
		already, err := s.store.HasSplitForPayment(ctx, paymentID)
		if err != nil {
			return fmt.Errorf("university: idempotency check: %w", err)
		}

		if already {
			slog.Info("university: process_enrollment: split already recorded, skipping", "payment_id", paymentID.String())
			return nil
		}
	} else if source == "pro_stream" {
		already, err := s.store.HasProStreamSplit(ctx, track.InstructorID, trackID, userID)
		if err != nil {
			return fmt.Errorf("university: pro_stream idempotency check: %w", err)
		}

		if already {
			slog.Info("university: process_enrollment: pro_stream split already recorded, skipping", "track_id", trackID.String(), "user_id", userID.String())
			return nil
		}
		// Read the configured per-stream rate from system_settings (BUG-008).
		// Setting key: university.pro_stream_rate_kobo (default 5000 = 50.00 NGN,
		// matching system-settings-catalog.md and migration 000169).
		if s.settings != nil {
			grossKobo = int64(s.settings.GetIntOr(ctx, "university.pro_stream_rate_kobo", 5000))
		}
	}

	// Resolve the enrollment (upsert).
	var payRef *pgtype.UUID
	if paymentID.Valid {
		payRef = &paymentID
	}

	accessType := "purchase"
	if source == "pro_stream" {
		accessType = "subscription"
	}

	enrollment, err := s.store.UpsertEnrollment(ctx, userID, trackID, accessType, payRef)
	if err != nil {
		return fmt.Errorf("university: upsert enrollment: %w", err)
	}

	// Increment the cached counter (best-effort; non-fatal).
	if err := s.store.IncrementTotalEnrolled(ctx, trackID); err != nil {
		slog.Warn("university: increment total_enrolled", "track_id", trackID.String(), "error", err)
	}
	// AUDIT-BE-VALKEY-008: increment the Valkey enrollment counter too.
	s.incrementTrackEnrolledCache(ctx, trackID)

	// Resolve the creator/platform split percentages.
	creatorPct, platformPct := s.resolveSplit(ctx, track.InstructorID)
	creatorKobo, platformKobo := splitAmount(grossKobo, creatorPct, platformPct)

	var enrollRef pgtype.UUID
	if enrollment.ID.Valid {
		enrollRef = enrollment.ID
	}

	var payRef2 pgtype.UUID
	if paymentID.Valid {
		payRef2 = paymentID
	}

	_, err = s.store.InsertRevenueSplit(ctx, gen.InsertRevenueSplitParams{

		InstructorID: track.InstructorID,
		TrackID:      trackID,
		EnrollmentID: enrollRef,
		PaymentID:    payRef2,
		Source:       source,
		GrossKobo:    grossKobo,
		CreatorPct:   numericFromFloat(creatorPct),
		PlatformPct:  numericFromFloat(platformPct),
		CreatorKobo:  creatorKobo,
		PlatformKobo: platformKobo,
		Currency:     "NGN",
	})
	if err != nil {
		return fmt.Errorf("university: insert revenue split: %w", err)
	}

	// AUDIT-BE-VALKEY-009: invalidate the payout accumulator for the current
	// period so the next payout calculation picks up the new split.
	now := time.Now()
	periodLabel := now.Format("2006-01")
	s.invalidatePayoutAccumulatorCache(ctx, track.InstructorID, periodLabel)

	return nil
}

// resolveSplit returns the creator and platform percentages for an instructor.
// Order: instructor override → system default. (Tier-based defaults are a
// future extension; today tier only affects override eligibility, not the
// percentage, per ADR-063 §6.)
func (s *Service) resolveSplit(ctx context.Context, instructorID pgtype.UUID) (creatorPct, platformPct float64) {
	creatorDefault := 75.0
	platformDefault := 25.0

	if s.settings != nil {
		creatorDefault = float64(s.settings.GetIntOr(ctx, "university.creator_percentage_default", 75))
		platformDefault = float64(s.settings.GetIntOr(ctx, "university.platform_percentage_default", 25))
	}

	creatorPct = creatorDefault
	platformPct = platformDefault

	prof, err := s.store.GetByID(ctx, instructorID)
	if err == nil && prof.CommissionPctOverride.Valid {
		if f, e := prof.CommissionPctOverride.Float64Value(); e == nil && f.Valid {
			creatorPct = f.Float64
			platformPct = 100.0 - creatorPct
		}
	}

	return creatorPct, platformPct
}

// splitAmount divides grossKobo into creator and platform shares by the
// given percentages. The platform share absorbs the rounding remainder so
// the two halves always sum to grossKobo (no lost kobo).
func splitAmount(grossKobo int64, creatorPct, platformPct float64) (creatorKobo, platformKobo int64) {
	if grossKobo <= 0 {
		return 0, 0
	}

	total := creatorPct + platformPct
	if total <= 0 {
		return 0, grossKobo
	}

	creator := math.Round(float64(grossKobo) * creatorPct / total)
	creatorKobo = int64(creator)
	platformKobo = grossKobo - creatorKobo

	return creatorKobo, platformKobo
}

// numericFromFloat builds a pgtype.Numeric from a float64 for insertion.
func numericFromFloat(f float64) pgtype.Numeric {
	var n pgtype.Numeric
	if err := n.Scan(fmt.Sprintf("%.2f", f)); err != nil {
		return pgtype.Numeric{}
	}

	return n
}

// ---------------------------------------------------------------------------
// Payout aggregation + execution
// ---------------------------------------------------------------------------

// CalculatePeriodPayouts is the scheduled handler for
// university:calculate_period_payouts. It walks every approved instructor with
// open splits in the period, creates an instructor_payouts row when the
// accrued net meets the configured minimum, and stamps the splits.
func (s *Service) CalculatePeriodPayouts(ctx context.Context, periodStart, periodEnd time.Time) error {
	minPayout := int64(500000)
	if s.settings != nil {
		minPayout = int64(s.settings.GetIntOr(ctx, "university.min_payout_kobo", 500000))
	}

	// List all approved instructors via the status filter (no space scope —
	// the periodic payout run is platform-wide, not per-university).
	profiles, err := s.store.ListByStatus(ctx, "approved", pgtype.UUID{}, 1000, 0)
	if err != nil {
		return fmt.Errorf("university: list instructors for payout: %w", err)
	}

	// Batch-fetch open split totals for ALL instructors in one query to avoid
	// an N+1 (AUDIT-BE-DB-005). Per-instructor cache lookups still happen
	// below; on a cache hit the batch row is simply unused for that instructor.
	batchSums, err := s.store.SumOpenSplitsByAllInstructors(ctx)
	if err != nil {
		return fmt.Errorf("university: batch sum open splits: %w", err)
	}

	batchByInstructor := make(map[pgtype.UUID]payoutSummary, len(batchSums))
	for _, b := range batchSums {
		batchByInstructor[b.InstructorID] = payoutSummary{
			GrossKoboTotal:    b.GrossKoboTotal,
			PlatformKoboTotal: b.PlatformKoboTotal,
			CreatorKoboTotal:  b.CreatorKoboTotal,
		}
	}

	ps := pgtype.Date{Time: periodStart, Valid: true}
	pe := pgtype.Date{Time: periodEnd, Valid: true}

	// AUDIT-BE-VALKEY-009: use the payout accumulator cache key. The period
	// label is the ISO date of periodStart (e.g. "2026-07-01").
	periodLabel := periodStart.Format("2006-01-02")

	for _, p := range profiles {
		// AUDIT-BE-VALKEY-009: read-through cache (jc:university:payout:{}:{period}:{instructor_id}).
		cached, err := s.cachedPayoutAccumulator(ctx, p.ID, periodLabel, batchByInstructor)
		if err != nil {
			slog.Error("university: sum open splits", "instructor_id", p.ID.String(), "error", err)
			continue
		}

		netKobo := cached.CreatorKoboTotal
		if netKobo < minPayout {
			continue
		}

		payout, err := s.store.CreatePayout(ctx, gen.CreateInstructorPayoutParams{

			InstructorID:           p.ID,
			PeriodStart:            ps,
			PeriodEnd:              pe,
			GrossKobo:              cached.GrossKoboTotal,
			PlatformCommissionKobo: cached.PlatformKoboTotal,
			NetKobo:                netKobo,
			Currency:               "NGN",
		})
		if err != nil {
			slog.Error("university: create payout", "instructor_id", p.ID.String(), "error", err)
			continue
		}

		if !payout.ID.Valid {
			// ON CONFLICT DO NOTHING — a payout already exists for this period.
			continue
		}

		if _, err := s.store.StampSplitsWithPayout(ctx, p.ID, payout.ID, ps, pe); err != nil {
			slog.Error("university: stamp splits with payout", "payout_id", payout.ID.String(), "error", err)
			continue
		}
		// AUDIT-BE-VALKEY-009: invalidate the payout accumulator after stamping.
		s.invalidatePayoutAccumulatorCache(ctx, p.ID, periodLabel)
		slog.Info("university: payout created", "payout_id", payout.ID.String(), "net_kobo", netKobo)
	}

	return nil
}

// SendInstructorPayout is the handler for university:send_instructor_payout.
// It marks the payout as processing, delegates the transfer to the payment
// provider abstraction (via the worker), and stamps the final status. The
// actual provider call is intentionally minimal here — the provider layer
// (Section 17) executes the transfer; this method records the outcome.
func (s *Service) SendInstructorPayout(ctx context.Context, payoutID pgtype.UUID) error {
	payout, err := s.store.GetPayout(ctx, payoutID)
	if err != nil {
		return fmt.Errorf("university: get payout: %w", err)
	}

	if payout.Status != "pending" && payout.Status != "failed" {
		return nil // already processing/paid
	}

	// Mark processing.
	if _, err := s.store.UpdatePayoutStatus(ctx, payoutID, "processing", nil, nil); err != nil {
		return fmt.Errorf("university: mark payout processing: %w", err)
	}

	// The provider transfer is executed by the payment provider abstraction.
	// In V1 this is a manual/recorded transfer: we mark paid with a generated
	// reference. A future integration calls provider.InitiatePayout here.
	ref := fmt.Sprintf("JC-PAYOUT-%s-%d", payout.ID.String()[:8], time.Now().Unix())
	if _, err := s.store.UpdatePayoutStatus(ctx, payoutID, "paid", &ref, nil); err != nil {
		return fmt.Errorf("university: mark payout paid: %w", err)
	}

	slog.Info("university: payout sent", "payout_id", payoutID.String(), "reference", ref)

	return nil
}

// FetchLogo downloads a university logo from a remote URL, stores it to
// SeaweedFS via the media service, and updates the space's logo_media_id.
// This is the handler for the university:fetch_logo Asynq task (Section 39,
// low tier), enqueued when an admin sets/changes a university's logo_url.
// uploaderID is the acting admin's user ID, attributed as the media
// uploader — media.uploader_user_id is a NOT NULL FK to users(id) and there
// is no generic "system user" row, so an invalid uploaderID (missing or
// unparseable) causes the fetch to be skipped rather than risking an
// FK-violation retry loop. If the media uploader or space logo updater is
// not wired, it logs a warning and returns nil (graceful degradation in
// dev/test).
func (s *Service) FetchLogo(ctx context.Context, spaceID pgtype.UUID, logoURL string, uploaderID pgtype.UUID) error {
	if s.mediaUploader == nil {
		slog.Warn("university:fetch_logo: no media uploader wired, skipping", "space_id", spaceID.String(), "logo_url", logoURL)
		return nil
	}

	if s.spaceLogoUpdater == nil {
		slog.Warn("university:fetch_logo: no space logo updater wired, skipping", "space_id", spaceID.String(), "logo_url", logoURL)
		return nil
	}

	if !uploaderID.Valid {
		slog.Warn("university:fetch_logo: no valid uploaded_by, skipping", "space_id", spaceID.String(), "logo_url", logoURL)
		return nil
	}

	filename := "university-logo-" + spaceID.String()

	mediaID, err := s.mediaUploader.StoreFromURL(ctx, uploaderID, logoURL, filename)
	if err != nil {
		return fmt.Errorf("university:fetch_logo: storing logo from %s: %w", logoURL, err)
	}

	if _, err := s.spaceLogoUpdater.UpdateSpaceLogoMediaID(ctx, gen.UpdateSpaceLogoMediaIDParams{
		ID: spaceID,

		LogoMediaID: mediaID,
	}); err != nil {
		return fmt.Errorf("university:fetch_logo: updating space logo_media_id: %w", err)
	}

	slog.Info("university:fetch_logo: logo stored and space updated", "space_id", spaceID.String(), "media_id", mediaID.String())

	return nil
}

// ---------------------------------------------------------------------------
// Asynq enqueue helpers (fire-and-forget; errors logged, never returned)
// ---------------------------------------------------------------------------

func (s *Service) enqueueNotifyInstructorApproved(ctx context.Context, instructorID pgtype.UUID) {
	s.enqueue(ctx, worker.TypeUniversityNotifyInstructorApproved, map[string]string{
		"instructor_id": instructorID.String(),
	})
}

func (s *Service) enqueueNotifyCourseApproved(ctx context.Context, trackID pgtype.UUID) {
	s.enqueue(ctx, worker.TypeUniversityNotifyCourseApproved, map[string]string{
		"track_id": trackID.String(),
	})
}

func (s *Service) enqueue(ctx context.Context, taskType string, payload map[string]string) {
	if s.enqueuer == nil {
		return
	}

	body, err := json.Marshal(payload)
	if err != nil {
		slog.Warn("university: marshal task payload", "task", taskType, "error", err)
		return
	}

	if err := s.enqueuer.Enqueue(taskType, body); err != nil {
		slog.Warn("university: enqueue task", "task", taskType, "error", err)
	}
}

// ---------------------------------------------------------------------------
// Notification dispatch (called by the worker processor for notify_* tasks)
// ---------------------------------------------------------------------------

// NotifyInstructorApprovedDispatch creates an in-app notification for the
// creator whose instructor application was approved. Called by the
// university:notify_instructor_approved task handler.
// Ref: japaconnect-engineering/sections/35-notification-system.md.
func (s *Service) NotifyInstructorApprovedDispatch(ctx context.Context, instructorID pgtype.UUID) error {
	if s.notifier == nil {
		slog.Info("university:notify_instructor_approved: no notifier wired, logging only", "instructor_id", instructorID)
		return nil
	}

	// Load the instructor profile to get the user_id (notification recipient).
	profile, err := s.store.GetByID(ctx, instructorID)
	if err != nil {
		return fmt.Errorf("university: notify instructor approved: load profile: %w", err)
	}

	if !profile.UserID.Valid {
		slog.Warn("university:notify_instructor_approved: instructor has no backing user", "instructor_id", instructorID)
		return nil
	}

	return s.notifier.CreateNotification(
		ctx,
		profile.UserID,
		"university_instructor_approved", // notification type key
		"system",                         // actor type
		pgtype.UUID{},                    // actor ID (system)
		"instructor",                     // subject type
		instructorID,                     // subject ID
		"Instructor Application Approved",
		"Your instructor application has been approved. You can now publish tracks on Japa University.",
		"/university/instructor/dashboard",
	)
}

// NotifyCourseApprovedDispatch creates an in-app notification for the
// creator whose track was approved to the marketplace. Called by the
// university:notify_course_approved task handler.
// Ref: japaconnect-engineering/sections/35-notification-system.md.
func (s *Service) NotifyCourseApprovedDispatch(ctx context.Context, trackID pgtype.UUID) error {
	if s.notifier == nil {
		slog.Info("university:notify_course_approved: no notifier wired, logging only", "track_id", trackID)
		return nil
	}

	// Load the track to get the instructor (and thus the user_id).
	track, err := s.store.GetTrackByID(ctx, trackID)
	if err != nil {
		return fmt.Errorf("university: notify course approved: load track: %w", err)
	}

	// Load the instructor profile to get the user_id (notification recipient).
	profile, err := s.store.GetByID(ctx, track.InstructorID)
	if err != nil {
		return fmt.Errorf("university: notify course approved: load instructor: %w", err)
	}

	if !profile.UserID.Valid {
		slog.Warn("university:notify_course_approved: instructor has no backing user", "instructor_id", track.InstructorID)
		return nil
	}

	return s.notifier.CreateNotification(
		ctx,
		profile.UserID,
		"university_course_approved", // notification type key
		"system",                     // actor type
		pgtype.UUID{},                // actor ID (system)
		"track",                      // subject type
		trackID,                      // subject ID
		"Track Approved",
		"Your track \""+track.Title+"\" has been approved and is now live on the Japa University marketplace.",
		"/university/tracks/"+trackID.String(),
	)
}

// ---------------------------------------------------------------------------
// Valkey cache helpers (AUDIT-BE-VALKEY-007/008/009)
// ---------------------------------------------------------------------------

// instructorProfileCacheKey builds jc:university:{}:instructor:{user_id}:profile.
// Platform-scoped to prevent cross-platform data leakage in multi-platform deployments (BUG-016).
func instructorProfileCacheKey(userID pgtype.UUID) string {
	return valkey.MustKey("university::instructor:%s:profile", userID.String())
}

// trackEnrolledCacheKey builds jc:university:{}:track:{track_id}:enrolled.
// Platform-scoped to prevent cross-platform data leakage (BUG-016).
func trackEnrolledCacheKey(trackID pgtype.UUID) string {
	return valkey.MustKey("university::track:%s:enrolled", trackID.String())
}

// payoutAccumulatorCacheKey builds jc:university:payout:{}:{period}:{instructor_id}.
func payoutAccumulatorCacheKey(instructorID pgtype.UUID, period string) string {
	return valkey.MustKey("university:payout::%s:%s", period, instructorID.String())
}

// cachedInstructorProfile returns the instructor profile from Valkey cache,
// or falls back to the store. On cache miss, the DB result is cached.
// AUDIT-BE-VALKEY-007.
func (s *Service) cachedInstructorProfile(ctx context.Context, userID pgtype.UUID) (InstructorProfile, error) {
	if s.rdb != nil {
		key := instructorProfileCacheKey(userID)
		if cached, err := s.rdb.Get(ctx, key).Bytes(); err == nil && len(cached) > 0 {
			var p InstructorProfile
			if json.Unmarshal(cached, &p) == nil {
				return p, nil
			}
		}
	}

	row, err := s.store.GetByUser(ctx, userID)
	if err != nil {
		return InstructorProfile{}, err
	}

	p := profileFromRow(row)

	if s.rdb != nil {
		key := instructorProfileCacheKey(userID)
		if data, err := json.Marshal(p); err == nil {
			if err := s.rdb.Set(ctx, key, data, instructorProfileTTL).Err(); err != nil {
				slog.Warn("university: cache instructor profile", "user_id", userID.String(), "error", err)
			}
		}
	}

	return p, nil
}

// invalidateInstructorProfileCache deletes the cached instructor profile.
// Called on approve/suspend/reinstate/update.
func (s *Service) invalidateInstructorProfileCache(ctx context.Context, userID pgtype.UUID) {
	if s.rdb == nil {
		return
	}

	if err := s.rdb.Del(ctx, instructorProfileCacheKey(userID)).Err(); err != nil {
		slog.Warn("university: invalidate instructor profile cache", "user_id", userID.String(), "error", err)
	}
}

// cachedTrackEnrolled returns the cached enrollment count for a track,
// or falls back to the store. On cache miss, the DB result is cached.
// AUDIT-BE-VALKEY-008.
func (s *Service) cachedTrackEnrolled(ctx context.Context, trackID pgtype.UUID) (int32, error) {
	if s.rdb != nil {
		key := trackEnrolledCacheKey(trackID)
		if cached, err := s.rdb.Get(ctx, key).Int(); err == nil {
			// #nosec G115 // value bounded by caller
			return int32(cached), nil
		}
	}

	track, err := s.store.GetTrackByID(ctx, trackID)
	if err != nil {
		return 0, err
	}

	count := track.TotalEnrolled

	if s.rdb != nil {
		key := trackEnrolledCacheKey(trackID)
		if err := s.rdb.Set(ctx, key, count, trackEnrolledTTL).Err(); err != nil {
			slog.Warn("university: cache track enrolled", "track_id", trackID.String(), "error", err)
		}
	}

	return count, nil
}

// incrementTrackEnrolledCache increments the cached enrollment counter.
// Called after a successful enrollment. Best-effort; non-fatal.
func (s *Service) incrementTrackEnrolledCache(ctx context.Context, trackID pgtype.UUID) {
	if s.rdb == nil {
		return
	}

	key := trackEnrolledCacheKey(trackID)
	if err := s.rdb.Incr(ctx, key).Err(); err != nil {
		// Key may not exist yet — set it from DB on next read. Non-fatal.
		slog.Warn("university: increment track enrolled cache", "track_id", trackID.String(), "error", err)
	}
}

// invalidateTrackEnrolledCache deletes the cached enrollment count.
// Called when the track is deleted or the counter is recounted.
func (s *Service) invalidateTrackEnrolledCache(ctx context.Context, trackID pgtype.UUID) {
	if s.rdb == nil {
		return
	}

	if err := s.rdb.Del(ctx, trackEnrolledCacheKey(trackID)).Err(); err != nil {
		slog.Warn("university: invalidate track enrolled cache", "track_id", trackID.String(), "error", err)
	}
}

// payoutSummary is the cached payout accumulator value.
type payoutSummary struct {
	GrossKoboTotal    int64 `json:"gross_kobo_total"`
	PlatformKoboTotal int64 `json:"platform_kobo_total"`
	CreatorKoboTotal  int64 `json:"creator_kobo_total"`
}

// cachedPayoutAccumulator returns the cached payout accumulator for an
// instructor in a period, or falls back to the batch-fetched sums. On cache
// miss, the batch result is cached. AUDIT-BE-VALKEY-009, AUDIT-BE-DB-005.
func (s *Service) cachedPayoutAccumulator(ctx context.Context, instructorID pgtype.UUID, period string, batch map[pgtype.UUID]payoutSummary) (payoutSummary, error) {
	if s.rdb != nil {
		key := payoutAccumulatorCacheKey(instructorID, period)
		if cached, err := s.rdb.Get(ctx, key).Bytes(); err == nil && len(cached) > 0 {
			var ps payoutSummary
			if json.Unmarshal(cached, &ps) == nil {
				return ps, nil
			}
		}
	}

	ps, ok := batch[instructorID]
	if !ok {
		// No open splits for this instructor — return zeros.
		ps = payoutSummary{}
	}

	if s.rdb != nil {
		key := payoutAccumulatorCacheKey(instructorID, period)
		if data, err := json.Marshal(ps); err == nil {
			if err := s.rdb.Set(ctx, key, data, payoutAccumulatorTTL).Err(); err != nil {
				slog.Warn("university: cache payout accumulator", "instructor_id", instructorID.String(), "error", err)
			}
		}
	}

	return ps, nil
}

// invalidatePayoutAccumulatorCache deletes the cached payout accumulator.
// Called when a new split is inserted or a payout is processed.
func (s *Service) invalidatePayoutAccumulatorCache(ctx context.Context, instructorID pgtype.UUID, period string) {
	if s.rdb == nil {
		return
	}

	if err := s.rdb.Del(ctx, payoutAccumulatorCacheKey(instructorID, period)).Err(); err != nil {
		slog.Warn("university: invalidate payout accumulator cache", "instructor_id", instructorID.String(), "error", err)
	}
}
