// service_mentorship.go — the 000824 compat surface the legacy
// mentorship.Service delegates to. Two responsibilities:
//
//  1. Mentor shadow listing lifecycle — PATCH /me/mentor-profile calls
//     EnsureMentorShadowListing (opt-in / field refresh) or
//     RetireMentorShadowListing (opt-out). The shadow is a spaceless
//     contents row + listing_kind='mentorship' meta, exactly the shape
//     the 000824 migration wrote.
//  2. Booking reads/transitions for the mentorship vocabulary — kind-
//     filtered list rows (mentee outbox, mentor inbox, admin list) plus
//     the mentee-complete and provider-cancel arms the shared
//     RespondToBooking/CancelMyBooking methods don't already cover.
//
// Ref: docs/superpowers/plans/2026-10-09-services-mentorship-unification.md
package servicedir

import (
	"context"
	"errors"
	"fmt"

	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/content"
)

// errShadowSlugSquat marks a mentor shadow whose canonical slug chain was
// fully occupied by unrelated rows (EnsureMentorShadowListing exhausted
// its retries) — an internal edge, never a client-facing error.
var errShadowSlugSquat = errors.New("mentor shadow slug chain exhausted")

// MentorShadowInput carries the display fields the shadow listing mirrors
// from the mentor's profile (PATCH /me/mentor-profile upsert).
type MentorShadowInput struct {
	Username    string // slug suffix — 'mentor-<username>'
	DisplayName string // title prefix — '<display_name> — Mentorship'
	Bio         string // listing body + content_hash source
}

// MentorshipListingContentID resolves a mentor's live shadow listing
// content id — the booking-create anchor. Returns the wrapped
// pgx.ErrNoRows when the mentor has no shadow (never opted in, or opted
// out and the shadow was soft-deleted).
func (s *Service) MentorshipListingContentID(ctx context.Context, mentorID pgtype.UUID) (pgtype.UUID, error) {
	row, err := s.store.GetMentorshipListing(ctx, mentorID)
	if err != nil {
		return pgtype.UUID{}, err // wrapped pgx.ErrNoRows — callers errors.Is it
	}

	return row.ID, nil
}

// EnsureMentorShadowListing creates or refreshes the mentor's shadow
// listing and returns its content id. Called after the user_profiles
// write when is_mentor is (or stays) true: an existing live or draft
// shadow is updated in place (the query republishes only a preserved
// draft — an archived/moderated status is respected) and its slug is
// re-asserted to the canonical mentor-<username>; a missing shadow is
// created and linked on user_profiles.mentor_listing_content_id.
//
// Slug/title/hash follow the 000824 migration verbatim:
// slug 'mentor-<username>', title '<display_name> — Mentorship'
// (username when display_name is blank), body = mentor bio, hash =
// ComputeContentHashWithDistinguisher(bio, "mentor:<user_id>").
//
// mentor- is a reserved prefix (Create re-suffixes generated mentor-*
// slugs for non-mentorship listings), but a pre-existing squat can't be
// ruled out — when the canonical slug is occupied by a row that is NOT
// this author's shadow, the create retries mentor-<username>-2, -3, …
// instead of failing or hijacking the squatter's row.
func (s *Service) EnsureMentorShadowListing(ctx context.Context, authorID pgtype.UUID, in MentorShadowInput) (pgtype.UUID, error) {
	title := in.DisplayName
	if title == "" {
		title = in.Username
	}
	title += " — Mentorship"

	slug := mentorSlugPrefix + in.Username
	hash := content.ComputeContentHashWithDistinguisher(in.Bio, "mentor:"+authorID.String())

	existing, err := s.store.GetMentorshipListing(ctx, authorID)
	if err == nil {
		if err := s.store.UpdateMentorShadow(ctx, existing.ID, authorID, title, slug, in.Bio, hash); err != nil {
			return pgtype.UUID{}, fmt.Errorf("servicedir: ensure mentor shadow: %w", err)
		}

		return existing.ID, nil
	}

	if !errors.Is(err, pgx.ErrNoRows) {
		return pgtype.UUID{}, fmt.Errorf("servicedir: ensure mentor shadow: %w", err)
	}

	// Insert race / squat disambiguation: CreateMentorshipShadowContent is
	// ON CONFLICT DO NOTHING over both partial uniques (slug, content_hash)
	// so a conflict returns pgx.ErrNoRows. The hash carries the
	// mentor:<user_id> distinguisher — if it conflicted, the winning row
	// IS this author's shadow (adopt it). If no shadow of ours exists, the
	// slug is squatted by an unrelated row — retry with a numeric suffix.
	const maxSquatRetries = 6
	for attempt := 1; attempt <= maxSquatRetries; attempt++ {
		candidate := slug
		if attempt > 1 {
			candidate = fmt.Sprintf("%s-%d", slug, attempt)
		}

		contentID, err := s.store.CreateMentorShadow(ctx, authorID, title, candidate, in.Bio, hash)
		if err == nil {
			return contentID, nil
		}

		if !errors.Is(err, pgx.ErrNoRows) {
			return pgtype.UUID{}, fmt.Errorf("servicedir: ensure mentor shadow: %w", err)
		}

		if winner, rerr := s.store.GetMentorshipListing(ctx, authorID); rerr == nil {
			// Lost an insert race (another ensure created the shadow
			// first) — adopt the winner so the link lands on the live row.
			if uerr := s.store.UpdateMentorShadow(ctx, winner.ID, authorID, title, slug, in.Bio, hash); uerr == nil {
				return winner.ID, nil
			}

			return winner.ID, nil
		}
	}

	return pgtype.UUID{}, fmt.Errorf("servicedir: ensure mentor shadow: %w: slug %s* exhausted %d retries", errShadowSlugSquat, slug, maxSquatRetries)
}

// RetireMentorShadowListing removes the mentor from the directory: the
// shadow contents row is soft-deleted (Rule 11) and
// user_profiles.mentor_listing_content_id is cleared. Migrated and live
// bookings keep their listing_meta_id — the meta row survives.
func (s *Service) RetireMentorShadowListing(ctx context.Context, authorID pgtype.UUID) error {
	return s.store.RetireMentorShadow(ctx, authorID)
}

// ---------------------------------------------------------------------
// Compat reads — mentorship-kind bookings in legacy connection shape.
// ---------------------------------------------------------------------

// MentorshipBookingRow returns the joined booking row for a single
// mentorship request, kind-checked. pgx.ErrNoRows when the id is not a
// mentorship-kind booking — the compat layer maps that to the legacy
// "connection not found" sentinel. A miss also probes the legacy-id
// fallback: migrated rows keep their pre-000824 connection UUID in
// meta->>'connection_id', so a caller holding the old id still resolves
// the booking.
func (s *Service) MentorshipBookingRow(ctx context.Context, bookingID pgtype.UUID) (BookingRow, error) {
	row, err := s.store.GetBookingDetail(ctx, bookingID)
	if err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			return BookingRow{}, err
		}

		resolved, rerr := s.store.GetMentorshipBookingIDByConnectionID(ctx, bookingID.String())
		if rerr != nil {
			return BookingRow{}, err // keep the original not-found
		}

		row, err = s.store.GetBookingDetail(ctx, resolved)
		if err != nil {
			return BookingRow{}, err
		}
	}

	if row.ListingKind != ListingKindMentorship {
		return BookingRow{}, pgx.ErrNoRows
	}

	return row, nil
}

// ListMentorshipBookingsByBooker is the mentee outbox (legacy
// ListOutgoingConnections): the caller's mentorship-kind bookings.
// status is already in booking vocabulary — the compat layer translates.
func (s *Service) ListMentorshipBookingsByBooker(ctx context.Context, bookerID pgtype.UUID, status string, limit, offset int32) ([]BookingRow, error) {
	rows, _, err := s.store.ListBookingsByUser(ctx, bookerID, BookingsByUserFilter{
		Status: status, Kind: ListingKindMentorship, Limit: limit, Offset: offset,
	})
	if err != nil {
		return nil, err
	}

	return rows, nil
}

// ListMentorshipBookingsByProvider is the mentor inbox (legacy
// ListIncomingConnections): mentorship-kind bookings where
// provider_user_id = the caller.
func (s *Service) ListMentorshipBookingsByProvider(ctx context.Context, providerID pgtype.UUID, status string, limit, offset int32) ([]BookingRow, error) {
	rows, _, err := s.store.ListBookingsByProviderUser(ctx, providerID, BookingsByProviderFilter{
		Status: status, Kind: ListingKindMentorship, Limit: limit, Offset: offset,
	})
	if err != nil {
		return nil, err
	}

	return rows, nil
}

// ListMentorshipBookingsAdmin is the admin connections list (legacy
// ListAllConnections): every mentorship-kind booking, optional mentor
// (provider_user_id) / mentee (booker_user_id) filters.
func (s *Service) ListMentorshipBookingsAdmin(ctx context.Context, status string, providerID, bookerID pgtype.UUID, limit, offset int32) ([]BookingRow, error) {
	rows, _, err := s.store.AdminListBookings(ctx, BookingsAdminFilter{
		Status:         status,
		Kind:           ListingKindMentorship,
		ProviderUserID: providerID,
		BookerUserID:   bookerID,
		Limit:          limit,
		Offset:         offset,
	})
	if err != nil {
		return nil, err
	}

	return rows, nil
}

// ---------------------------------------------------------------------
// Compat transitions — the arms the shared state machine doesn't cover.
// ---------------------------------------------------------------------

// CompleteMyBooking lets the booker mark a user-provider (mentorship)
// booking fulfilled — the mentee arm of the legacy "either party
// completes" rule. confirmed → fulfilled only.
func (s *Service) CompleteMyBooking(ctx context.Context, bookingID, userID pgtype.UUID) (BookingDTO, error) {
	row, err := s.store.GetBookingDetail(ctx, bookingID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return BookingDTO{}, pgx.ErrNoRows
		}

		return BookingDTO{}, fmt.Errorf("servicedir: complete: get booking: %w", err)
	}

	// Booker-only, and only on user-provider rows — an org-owned booking's
	// fulfill decision belongs to the org (OrgTransition), not the booker.
	if row.BookerUserID != userID || !row.ProviderUserID.Valid {
		return BookingDTO{}, ErrNotOwner
	}

	if _, err := s.store.FulfillBooking(ctx, bookingID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return BookingDTO{}, ErrInvalidBookingTransition
		}

		return BookingDTO{}, fmt.Errorf("servicedir: complete booking: %w", err)
	}

	// Booker-initiated — notify the provider side, not the booker
	// (audience=provider → provider_user_id on mentorship rows, org
	// members on org-owned rows).
	s.enqueueStatusChangedAudience(row.ID, BookingStatusFulfilled, "provider")

	if row.ListingKind == ListingKindMentorship {
		// Leaving 'confirmed' dropped active_mentee_count via the trigger.
		s.enqueueMentorMeilisearch(row.ProviderUserID)
	}

	return s.GetBooking(ctx, bookingID, userID, false)
}

// CancelProviderBooking lets the provider user cancel their own
// mentorship booking — the mentor arm of the legacy "either party
// cancels" rule. pending|confirmed → cancelled.
func (s *Service) CancelProviderBooking(ctx context.Context, providerID, bookingID pgtype.UUID, reason string) (BookingDTO, error) {
	row, err := s.store.GetBookingDetail(ctx, bookingID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return BookingDTO{}, pgx.ErrNoRows
		}

		return BookingDTO{}, fmt.Errorf("servicedir: provider cancel: get booking: %w", err)
	}

	if !row.ProviderUserID.Valid || row.ProviderUserID != providerID {
		return BookingDTO{}, ErrNotOwner
	}

	if utf8.RuneCountInString(reason) > maxBookingTextLen {
		return BookingDTO{}, ErrInputTooLong
	}

	var reasonPtr *string
	if reason != "" {
		reasonPtr = &reason
	}

	if _, err := s.store.OrgCancelBooking(ctx, bookingID, nil, reasonPtr); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return BookingDTO{}, ErrInvalidBookingTransition
		}

		return BookingDTO{}, fmt.Errorf("servicedir: provider cancel booking: %w", err)
	}

	s.enqueueStatusChanged(row.ID, BookingStatusCancelled)

	if row.ListingKind == ListingKindMentorship && row.Status == BookingStatusConfirmed {
		s.enqueueMentorMeilisearch(row.ProviderUserID)
	}

	return s.GetBooking(ctx, bookingID, providerID, false)
}
