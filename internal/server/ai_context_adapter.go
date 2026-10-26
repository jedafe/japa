// Package server — ai_context_adapter.go wires per-category user site activity
// into the AI context builder (ADR-025, BUG-001).
//
// The ai package defines the narrow UserContextProvider interface so the
// handlers package does not import wallet, academy, mentorship, forum, or
// referral directly. This adapter lives in the wiring layer where those
// services are already constructed and simply aggregates the data.
//
// Ref: japaconnect-engineering/sections/20-ai-core.md
// Ref: japaconnect-engineering/adr/ADR-025-ai-context-permission-nodes.md
package server

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/academy"
	"github.com/jedafe/japa/internal/ai"
	"github.com/jedafe/japa/internal/db/gen"
	"github.com/jedafe/japa/internal/forum"
	"github.com/jedafe/japa/internal/mentorship"
	"github.com/jedafe/japa/internal/referral"
	"github.com/jedafe/japa/internal/wallet"
)

// userContextAdapter implements ai.UserContextProvider by aggregating real
// per-user data from the relevant services and stores.
type userContextAdapter struct {
	wallet     *wallet.Service
	academy    *academy.Store
	mentorship *mentorship.Service
	forum      *forum.Service
	referral   *referral.Service
	read       *gen.Queries
}

// newUserContextAdapter creates an adapter bound to the real service instances.
func newUserContextAdapter(
	wallet *wallet.Service,
	academy *academy.Store,
	mentorship *mentorship.Service,
	forum *forum.Service,
	referral *referral.Service,
	read *gen.Queries,
) ai.UserContextProvider {
	return &userContextAdapter{
		wallet:     wallet,
		academy:    academy,
		mentorship: mentorship,
		forum:      forum,
		referral:   referral,
		read:       read,
	}
}

// GetUserAIContextData fetches the requested categories and returns them as a
// map keyed by aiContextCategories values (points_balance, course_progress,
// mentorship_activity, forum_activity, referral_activity, moderation_history).
func (a *userContextAdapter) GetUserAIContextData(ctx context.Context, userID pgtype.UUID, categories []string) (map[string]any, error) {
	out := make(map[string]any, len(categories))

	for _, cat := range categories {
		switch cat {
		case "points_balance":
			if a.wallet != nil {
				points, credits, err := a.wallet.GetBalance(ctx, userID)
				if err != nil {
					return nil, fmt.Errorf("points_balance: %w", err)
				}

				out[cat] = map[string]any{"points": points, "credits": credits}
			}

		case "course_progress":
			if a.academy != nil {
				academy_enrollments, err := a.academy.ListEnrollmentsByUser(ctx, userID)
				if err != nil {
					return nil, fmt.Errorf("course_progress: %w", err)
				}

				summary := make([]map[string]any, 0, len(academy_enrollments))
				for _, e := range academy_enrollments {
					summary = append(summary, map[string]any{
						"track_slug":        e.TrackSlug,
						"track_title":       e.TrackTitle,
						"progress_pct":      e.ProgressPct,
						"lessons_completed": e.LessonsCompleted,
						"lessons_total":     e.LessonsTotal,
						"completed_at":      e.CompletedAt,
						"last_activity_at":  e.LastActivityAt,
					})
				}

				out[cat] = summary
			}

		case "mentorship_activity":
			if a.mentorship != nil {
				menteeConns, _, err := a.mentorship.ListAllConnections(ctx, "", "", "", pgtype.UUID{}, userID, 20, 0)
				if err != nil {
					return nil, fmt.Errorf("mentorship_activity mentee: %w", err)
				}

				mentorConns, _, err := a.mentorship.ListAllConnections(ctx, "", "", "", userID, pgtype.UUID{}, 20, 0)
				if err != nil {
					return nil, fmt.Errorf("mentorship_activity mentor: %w", err)
				}

				out[cat] = map[string]any{
					"as_mentee": summarizeConnections(menteeConns),
					"as_mentor": summarizeConnections(mentorConns),
				}
			}

		case "forum_activity":
			if a.forum != nil {
				threads, err := a.forum.ListThreadsByAuthorWithDetails(
					ctx, userID, userID, nil, "", pgtype.Timestamptz{}, pgtype.UUID{}, nil, 20,
				)
				if err != nil {
					return nil, fmt.Errorf("forum_activity: %w", err)
				}

				summary := make([]map[string]any, 0, len(threads))
				for _, t := range threads {
					summary = append(summary, map[string]any{
						"title":       t.Title,
						"slug":        t.Slug,
						"space_slug":  t.SpaceSlug,
						"solved":      t.Solved,
						"reply_count": t.ReplyCount,
						"vote_score":  t.VoteScore,
						"created_at":  t.CreatedAt,
					})
				}

				out[cat] = summary
			}

		case "referral_activity":
			if a.referral != nil {
				stats, err := a.referral.GetReferralStats(ctx, userID)
				if err != nil {
					return nil, fmt.Errorf("referral_activity: %w", err)
				}

				out[cat] = stats
			}

		case "moderation_history":
			if a.read != nil {
				bans, err := a.read.ListActiveUserBans(ctx, userID)
				if err != nil {
					return nil, fmt.Errorf("moderation_history bans: %w", err)
				}

				warnings, err := a.read.ListUserWarnings(ctx, userID)
				if err != nil {
					return nil, fmt.Errorf("moderation_history warnings: %w", err)
				}

				out[cat] = map[string]any{
					"active_bans": summarizeBans(bans),
					"warnings":    summarizeWarnings(warnings),
				}
			}
		}
	}

	return out, nil
}

// summarizeConnections reduces mentorship.Connection rows to a serializable
// summary so the AI context stays focused on status and participants.
func summarizeConnections(conns []mentorship.Connection) []map[string]any {
	out := make([]map[string]any, 0, len(conns))
	for _, c := range conns {
		out = append(out, map[string]any{
			"status":               c.Status,
			"mentor_username":      derefString(c.MentorUsername),
			"mentee_username":      derefString(c.MenteeUsername),
			"created_at":           c.CreatedAt,
			"destination_space_id": c.DestinationSpaceID,
		})
	}

	return out
}

// summarizeBans serializes active user ban rows.
func summarizeBans(bans []gen.ListActiveUserBansRow) []map[string]any {
	out := make([]map[string]any, 0, len(bans))
	for _, b := range bans {
		out = append(out, map[string]any{
			"id":         b.ID,
			"reason":     derefString(b.Reason),
			"expires_at": b.ExpiresAt,
			"created_at": b.CreatedAt,
		})
	}

	return out
}

// summarizeWarnings serializes user warning rows.
func summarizeWarnings(warnings []gen.ListUserWarningsRow) []map[string]any {
	out := make([]map[string]any, 0, len(warnings))
	for _, w := range warnings {
		out = append(out, map[string]any{
			"id":         w.ID,
			"reason":     derefString(w.Reason),
			"created_at": w.CreatedAt,
		})
	}

	return out
}

// derefString returns the string value of a pointer or an empty string.
func derefString(s *string) string {
	if s == nil {
		return ""
	}

	return *s
}
