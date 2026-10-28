package tips

import (
	"context"
	"net/url"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db/gen"
)

// toSettings maps a tip_settings row to the service Settings view. The OC
// token ciphertext is intentionally dropped — only presence (HasOCToken) and
// the derived webhook URL are exposed.
func (s *Service) toSettings(row gen.TipSetting) Settings {
	out := Settings{
		ID:               row.ID,
		OwnerType:        row.OwnerType,
		OwnerID:          row.OwnerID,
		TipsEnabled:      row.TipsEnabled,
		AcceptPoints:     row.AcceptPoints,
		AcceptCredits:    row.AcceptCredits,
		AcceptFiat:       row.AcceptFiat,
		AcceptExternal:   row.AcceptExternal,
		OCCollectiveSlug: row.OcCollectiveSlug,
		HasOCToken:       row.OcApiTokenEncrypted != nil && *row.OcApiTokenEncrypted != "",
		MinTipAmount:     row.MinTipAmount,
		ThankYouMessage:  row.ThankYouMessage,
		row:              row,
		hasRow:           true,
	}
	if row.OcWebhookSecret != nil && *row.OcWebhookSecret != "" && s.siteURL != "" {
		u := strings.TrimRight(s.siteURL, "/") + "/api/v1/tips/webhook/opencollective?s=" + *row.OcWebhookSecret
		out.OCWebhookURL = &u
	}
	return out
}

// emptySettings synthesizes the not-yet-configured default for an owner.
func emptySettings(owner Owner) Settings {
	return Settings{
		OwnerType:     owner.Type,
		OwnerID:       owner.ID,
		AcceptPoints:  true,
		AcceptCredits: true,
		AcceptFiat:    true,
	}
}

// GetSettings returns the owner's settings; a never-configured owner gets the
// disabled-by-default synthesis rather than an error.
func (s *Service) GetSettings(ctx context.Context, owner Owner) (Settings, error) {
	row, err := s.store.getSettings(ctx, owner.Type, owner.ID)
	if err != nil {
		if err == ErrNotFound {
			return emptySettings(owner), nil
		}
		return Settings{}, err
	}
	return s.toSettings(row), nil
}

// UpsertSettings persists the owner-editable settings, preserving OC
// credentials already on the row.
func (s *Service) UpsertSettings(ctx context.Context, owner Owner, in UpdateSettingsInput) (Settings, error) {
	if in.MinTipAmount < 0 {
		return Settings{}, ErrInvalidAmount
	}
	row, err := s.store.upsertSettings(ctx, gen.UpsertTipSettingsParams{
		OwnerType:       owner.Type,
		OwnerID:         owner.ID,
		TipsEnabled:     in.TipsEnabled,
		AcceptPoints:    in.AcceptPoints,
		AcceptCredits:   in.AcceptCredits,
		AcceptFiat:      in.AcceptFiat,
		AcceptExternal:  in.AcceptExternal,
		MinTipAmount:    in.MinTipAmount,
		ThankYouMessage: in.ThankYouMessage,
	})
	if err != nil {
		return Settings{}, err
	}
	return s.toSettings(row), nil
}

// CanReceive reports whether a user may enable tips — tips.receive permission
// or an active mentor profile (§125 RBAC, implicit mentor grant).
func (s *Service) CanReceive(ctx context.Context, userID pgtype.UUID) (bool, error) {
	if s.perms != nil && s.perms.Can(ctx, userID, "tips.receive") {
		return true, nil
	}
	return s.store.isMentor(ctx, userID)
}

// ensureWritable authorizes settings writes: users write their own row, org
// rows need owner/admin membership, the platform row needs tips.moderate.
func (s *Service) ensureWritable(ctx context.Context, owner Owner, actorID pgtype.UUID) error {
	switch owner.Type {
	case RecipientUser:
		if owner.ID != actorID {
			return ErrNotAuthorized
		}
		eligible, err := s.CanReceive(ctx, actorID)
		if err != nil {
			return err
		}
		if !eligible {
			return ErrNotEligible
		}
		return nil
	case RecipientOrganization:
		ownerID, err := s.store.getOrgOwnerID(ctx, owner.ID)
		if err != nil {
			return err
		}
		if ownerID == actorID {
			return nil
		}
		if s.orgs != nil {
			role, err := s.orgs.MemberRole(ctx, owner.ID, actorID)
			if err == nil && role == "admin" {
				return nil
			}
		}
		return ErrNotAuthorized
	case RecipientPlatform:
		if s.perms != nil && s.perms.Can(ctx, actorID, "tips.moderate") {
			return nil
		}
		return ErrNotAuthorized
	default:
		return ErrInvalidRecipient
	}
}

// UpdateSettingsFor authorizes then upserts settings on behalf of actorID.
func (s *Service) UpdateSettingsFor(ctx context.Context, owner Owner, actorID pgtype.UUID, in UpdateSettingsInput) (Settings, error) {
	if err := s.ensureWritable(ctx, owner, actorID); err != nil {
		return Settings{}, err
	}
	return s.UpsertSettings(ctx, owner, in)
}

// acceptedMethods derives the public method list from a settings row.
func acceptedMethods(st Settings) []string {
	out := make([]string, 0, 4)
	if st.AcceptPoints {
		out = append(out, MethodPoints)
	}
	if st.AcceptCredits {
		out = append(out, MethodCredits)
	}
	if st.AcceptFiat {
		for m := range fiatMethods {
			out = append(out, m)
		}
	}
	if st.OCCollectiveSlug != nil && *st.OCCollectiveSlug != "" {
		out = append(out, MethodOpenCollective)
	}
	return out
}

// PublicConfig returns the unauthenticated view of a recipient's tip config.
func (s *Service) PublicConfig(ctx context.Context, owner Owner) (PublicConfig, error) {
	st, err := s.GetSettings(ctx, owner)
	if err != nil {
		return PublicConfig{}, err
	}
	cfg := PublicConfig{MinTipAmount: st.MinTipAmount}
	if !st.TipsEnabled {
		return cfg, nil
	}
	cfg.Enabled = true
	cfg.Methods = acceptedMethods(st)
	cfg.OCCollectiveSlug = st.OCCollectiveSlug
	cfg.ThankYouMessage = st.ThankYouMessage
	if st.AcceptExternal && st.hasRow {
		links, err := s.listLinks(ctx, st.ID)
		if err != nil {
			return PublicConfig{}, err
		}
		cfg.ExternalLinks = links
	}
	return cfg, nil
}

// --- External links ---------------------------------------------------------

var validLinkKinds = map[string]bool{
	"paypal": true, "kofi": true, "buymeacoffee": true, "opencollective": true, "other": true,
}

func validateLinkInput(in LinkInput) error {
	if !validLinkKinds[in.Kind] {
		return ErrInvalidLink
	}
	if strings.TrimSpace(in.Label) == "" {
		return ErrInvalidLink
	}
	u, err := url.Parse(in.URL)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
		return ErrInvalidLink
	}
	return nil
}

func (s *Service) listLinks(ctx context.Context, settingsID pgtype.UUID) ([]ExternalLink, error) {
	rows, err := s.store.listLinks(ctx, settingsID)
	if err != nil {
		return nil, err
	}
	out := make([]ExternalLink, 0, len(rows))
	for _, r := range rows {
		out = append(out, ExternalLink{ID: r.ID, Kind: r.Kind, Label: r.Label, URL: r.Url, Position: r.Position})
	}
	return out, nil
}

// ListLinks returns the owner's external links (dashboard view).
func (s *Service) ListLinks(ctx context.Context, owner Owner) ([]ExternalLink, error) {
	st, err := s.GetSettings(ctx, owner)
	if err != nil {
		return nil, err
	}
	if !st.hasRow {
		return []ExternalLink{}, nil
	}
	return s.listLinks(ctx, st.ID)
}

// CreateLink authorizes + validates + appends a link.
func (s *Service) CreateLink(ctx context.Context, owner Owner, actorID pgtype.UUID, in LinkInput) (ExternalLink, error) {
	if err := s.ensureWritable(ctx, owner, actorID); err != nil {
		return ExternalLink{}, err
	}
	if err := validateLinkInput(in); err != nil {
		return ExternalLink{}, err
	}
	st, err := s.GetSettings(ctx, owner)
	if err != nil {
		return ExternalLink{}, err
	}
	if !st.hasRow {
		return ExternalLink{}, ErrNotFound
	}
	existing, err := s.listLinks(ctx, st.ID)
	if err != nil {
		return ExternalLink{}, err
	}
	if len(existing) >= maxExternalLinks {
		return ExternalLink{}, ErrTooManyLinks
	}
	row, err := s.store.createLink(ctx, gen.CreateTipExternalLinkParams{
		TipSettingsID: st.ID,
		Kind:          in.Kind,
		Label:         in.Label,
		Url:           in.URL,
		Position:      in.Position,
	})
	if err != nil {
		return ExternalLink{}, err
	}
	return ExternalLink{ID: row.ID, Kind: row.Kind, Label: row.Label, URL: row.Url, Position: row.Position}, nil
}

// UpdateLink edits one of the owner's links.
func (s *Service) UpdateLink(ctx context.Context, owner Owner, actorID, linkID pgtype.UUID, in LinkInput) (ExternalLink, error) {
	if err := s.ensureWritable(ctx, owner, actorID); err != nil {
		return ExternalLink{}, err
	}
	if err := validateLinkInput(in); err != nil {
		return ExternalLink{}, err
	}
	st, err := s.GetSettings(ctx, owner)
	if err != nil {
		return ExternalLink{}, err
	}
	if !st.hasRow {
		return ExternalLink{}, ErrNotFound
	}
	row, err := s.store.updateLink(ctx, gen.UpdateTipExternalLinkParams{
		ID: linkID, TipSettingsID: st.ID, Kind: in.Kind, Label: in.Label, Url: in.URL, Position: in.Position,
	})
	if err != nil {
		return ExternalLink{}, err
	}
	return ExternalLink{ID: row.ID, Kind: row.Kind, Label: row.Label, URL: row.Url, Position: row.Position}, nil
}

// DeleteLink removes one of the owner's links.
func (s *Service) DeleteLink(ctx context.Context, owner Owner, actorID, linkID pgtype.UUID) error {
	if err := s.ensureWritable(ctx, owner, actorID); err != nil {
		return err
	}
	st, err := s.GetSettings(ctx, owner)
	if err != nil {
		return err
	}
	if !st.hasRow {
		return ErrNotFound
	}
	return s.store.deleteLink(ctx, linkID, st.ID)
}
