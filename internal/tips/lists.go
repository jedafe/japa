// lists.go — §125 read surfaces: member sent/received history, public content
// tip summary, admin list/stats. Anonymous senders are masked everywhere
// except the admin path (moderators see real identities for anti-abuse).
package tips

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db/gen"
)

// ListTipsSent returns the member's outgoing tips, newest first.
func (s *Service) ListTipsSent(ctx context.Context, senderID pgtype.UUID, page, limit int32) ([]Tip, error) {
	rows, err := s.store.listTipsSent(ctx, senderID, limit, (page-1)*limit)
	if err != nil {
		return nil, err
	}
	return s.hydrateTips(ctx, rows, true), nil
}

// ListTipsReceived returns tips where the member is the recipient (direct
// user tips only — org tips surface through org reporting, not personal
// history). Anonymous tips keep their real sender visible to the recipient.
func (s *Service) ListTipsReceived(ctx context.Context, userID pgtype.UUID, page, limit int32) ([]Tip, error) {
	rows, err := s.store.listTipsReceived(ctx, userID, limit, (page-1)*limit)
	if err != nil {
		return nil, err
	}
	return s.hydrateTips(ctx, rows, true), nil
}

// CountTipsReceived totals the member's received tips for the stats widget.
func (s *Service) CountTipsReceived(ctx context.Context, userID pgtype.UUID) (int32, error) {
	return s.store.countTipsReceived(ctx, userID)
}

// ContentTipSummary returns the public aggregate for a content item's
// "Supported" affordance — counts only, no donor identities.
func (s *Service) ContentTipSummary(ctx context.Context, contentID pgtype.UUID) (gen.ContentTipSummaryRow, error) {
	return s.store.contentTipSummary(ctx, contentID)
}

// AdminListTips filters the full ledger for moderation.
func (s *Service) AdminListTips(ctx context.Context, status, method string, page, limit int32) ([]Tip, int32, error) {
	rows, err := s.store.adminListTips(ctx, gen.AdminListTipsParams{
		Status: status, Method: method,
		PageLimit: limit, PageOffset: (page - 1) * limit,
	})
	if err != nil {
		return nil, 0, err
	}
	total, err := s.store.adminCountTips(ctx, gen.AdminCountTipsParams{
		Status: status, Method: method,
	})
	if err != nil {
		return nil, 0, err
	}
	return s.hydrateTips(ctx, rows, false), total, nil
}

// AdminTipStats returns aggregate stats since the given cutoff.
func (s *Service) AdminTipStats(ctx context.Context, since pgtype.Timestamptz) (gen.AdminTipStatsRow, error) {
	return s.store.adminTipStats(ctx, since)
}

// hydrateTips maps rows onto Tip DTOs, batch-resolving sender display names
// and masking anonymous senders for non-admin callers.
func (s *Service) hydrateTips(ctx context.Context, rows []gen.Tip, maskAnonymous bool) []Tip {
	out := make([]Tip, 0, len(rows))
	if len(rows) == 0 {
		return out
	}

	// Batch-resolve sender names.
	seen := map[string]bool{}
	var ids []pgtype.UUID
	for _, r := range rows {
		k := r.SenderUserID.String()
		if r.SenderUserID.Valid && !seen[k] {
			seen[k] = true
			ids = append(ids, r.SenderUserID)
		}
	}
	names := map[string]string{}
	if len(ids) > 0 {
		if rows2, err := s.store.getUserDisplayNames(ctx, ids); err == nil {
			for _, n := range rows2 {
				if n.DisplayName != nil && *n.DisplayName != "" {
					names[n.ID.String()] = *n.DisplayName
				} else {
					names[n.ID.String()] = n.Username
				}
			}
		}
	}

	for _, r := range rows {
		t := tipFromRow(r)
		if name, ok := names[r.SenderUserID.String()]; ok {
			t.SenderName = name
		}
		if t.IsAnonymous && maskAnonymous {
			t.SenderName = "Someone"
			t.SenderUserID = pgtype.UUID{}
		}
		out = append(out, t)
	}
	return out
}
