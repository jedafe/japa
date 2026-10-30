// lists.go — §125 read surfaces: member sent/received history, public content
// tip summary, admin list/stats. Anonymous senders are masked everywhere
// except the admin path (moderators see real identities for anti-abuse).
package tips

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db/gen"
)

// ListTipsSent returns the member's outgoing tips, newest first. The sender
// already knows their identity — no masking needed on this view.
func (s *Service) ListTipsSent(ctx context.Context, senderID pgtype.UUID, page, limit int32) ([]Tip, error) {
	rows, err := s.store.listTipsSent(ctx, senderID, limit, (page-1)*limit)
	if err != nil {
		return nil, err
	}
	return s.hydrateTips(ctx, rows, false), nil
}

// ListTipsReceived returns tips where the member is the recipient (direct
// user tips only — org tips surface through org reporting, not personal
// history). §125 anonymity rule: the recipient and admins retain the real
// sender — masking applies only to public aggregate surfaces.
func (s *Service) ListTipsReceived(ctx context.Context, userID pgtype.UUID, page, limit int32) ([]Tip, error) {
	rows, err := s.store.listTipsReceived(ctx, userID, limit, (page-1)*limit)
	if err != nil {
		return nil, err
	}
	return s.hydrateTips(ctx, rows, false), nil
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
func (s *Service) AdminListTips(ctx context.Context, status, method, recipientType, q string, page, limit int32) ([]Tip, int32, error) {
	rows, err := s.store.adminListTips(ctx, gen.AdminListTipsParams{
		Status: status, Method: method, RecipientType: recipientType, Q: q,
		PageLimit: limit, PageOffset: (page - 1) * limit,
	})
	if err != nil {
		return nil, 0, err
	}
	total, err := s.store.adminCountTips(ctx, gen.AdminCountTipsParams{
		Status: status, Method: method, RecipientType: recipientType, Q: q,
	})
	if err != nil {
		return nil, 0, err
	}
	return s.hydrateTips(ctx, rows, false), total, nil
}

// GetTip returns a single hydrated tip (admin detail view — real sender).
func (s *Service) GetTip(ctx context.Context, tipID pgtype.UUID) (Tip, error) {
	row, err := s.store.getTip(ctx, tipID)
	if err != nil {
		return Tip{}, err
	}
	out := s.hydrateTips(ctx, []gen.Tip{row}, false)
	return out[0], nil
}

// AdminTipStats returns aggregate stats since the given cutoff plus a
// per-currency fiat breakdown — summing minor units across currencies is
// meaningless, so the aggregate stays credits-focused.
func (s *Service) AdminTipStats(ctx context.Context, since pgtype.Timestamptz) (gen.AdminTipStatsRow, map[string]int64, error) {
	row, err := s.store.adminTipStats(ctx, since)
	if err != nil {
		return gen.AdminTipStatsRow{}, nil, err
	}
	byCur := map[string]int64{}
	if rows, err := s.store.adminTipStatsByCurrency(ctx, since); err == nil {
		for _, r := range rows {
			if r.FiatCurrency != nil {
				byCur[*r.FiatCurrency] = r.Minor
			}
		}
	}
	return row, byCur, nil
}

// hydrateTips maps rows onto Tip DTOs, batch-resolving sender display names
// and masking anonymous senders for non-admin callers.
func (s *Service) hydrateTips(ctx context.Context, rows []gen.Tip, maskAnonymous bool) []Tip {
	out := make([]Tip, 0, len(rows))
	if len(rows) == 0 {
		return out
	}

	// Batch-resolve sender names and recipient names.
	seen := map[string]bool{}
	orgSeen := map[string]bool{}
	var ids []pgtype.UUID
	var orgIDs []pgtype.UUID
	for _, r := range rows {
		k := r.SenderUserID.String()
		if r.SenderUserID.Valid && !seen[k] {
			seen[k] = true
			ids = append(ids, r.SenderUserID)
		}
		if r.RecipientUserID.Valid {
			rk := r.RecipientUserID.String()
			if !seen[rk] {
				seen[rk] = true
				ids = append(ids, r.RecipientUserID)
			}
		}
		if r.RecipientOrganizationID.Valid {
			ok := r.RecipientOrganizationID.String()
			if !orgSeen[ok] {
				orgSeen[ok] = true
				orgIDs = append(orgIDs, r.RecipientOrganizationID)
			}
		}
	}
	names := map[string]string{}
	orgNames := map[string]string{}
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
	if len(orgIDs) > 0 {
		if rows2, err := s.store.getOrgDisplayNames(ctx, orgIDs); err == nil {
			for _, n := range rows2 {
				orgNames[n.ID.String()] = n.Name
			}
		}
	}

	for _, r := range rows {
		t := tipFromRow(r)
		if name, ok := names[r.SenderUserID.String()]; ok {
			t.SenderName = name
		}
		switch r.RecipientType {
		case RecipientUser:
			if name, ok := names[r.RecipientUserID.String()]; ok {
				t.RecipientName = name
			}
		case RecipientOrganization:
			if name, ok := orgNames[r.RecipientOrganizationID.String()]; ok {
				t.RecipientName = name
			}
		case RecipientPlatform:
			t.RecipientName = "JapaConnect"
		}
		if t.IsAnonymous && maskAnonymous {
			t.SenderName = "Someone"
			t.SenderUserID = pgtype.UUID{}
		}
		out = append(out, t)
	}
	return out
}
