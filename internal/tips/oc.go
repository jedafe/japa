// oc.go — §125 Open Collective integration: collective connect/disconnect,
// contribution import (dedupe by oc_contribution_id), and the scheduled
// collective sweep used by the tips:oc_sync task.
package tips

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"regexp"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/crypto"
	"github.com/jedafe/japa/internal/db/gen"
)

// ocSlugPattern bounds collective slugs to OC's URL-safe charset — the slug
// is interpolated into outbound opencollective.com URLs.
var ocSlugPattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]{0,99}$`)

// ConnectOC links an owner's Open Collective collective. A personal API token
// is verified against OC before being stored AES-256-GCM encrypted (same
// treatment as BYOK keys); a slug-only connect renders as a plain outbound
// link without sync. A fresh webhook secret is generated on every connect.
func (s *Service) ConnectOC(ctx context.Context, owner Owner, actorID pgtype.UUID, slug, token string, rotateSecret bool) (*Settings, error) {
	if err := s.ensureWritable(ctx, owner, actorID); err != nil {
		return nil, err
	}
	if !ocSlugPattern.MatchString(slug) {
		return nil, ErrInvalidLink
	}

	var encrypted *string
	if token != "" {
		if s.oc == nil {
			return nil, ErrOCNotConnected
		}
		if _, err := s.oc.VerifyToken(ctx, token); err != nil {
			return nil, ErrOCAccess
		}
		// The token must belong to an account that administers/members the
		// collective — otherwise anyone could attach a popular collective and
		// import its donations as their own tips.
		ok, err := s.oc.VerifyCollectiveAccess(ctx, token, slug)
		if err != nil {
			return nil, fmt.Errorf("verifying collective access: %w", err)
		}
		if !ok {
			return nil, ErrOCAccess
		}
		enc, err := crypto.Encrypt(token)
		if err != nil {
			return nil, fmt.Errorf("encrypting OC token: %w", err)
		}
		encrypted = &enc
	}

	// Ensure the settings row exists so the UPDATE lands.
	st, err := s.GetSettings(ctx, owner)
	if err != nil {
		return nil, err
	}
	if !st.hasRow {
		if _, err := s.UpsertSettings(ctx, owner, UpdateSettingsInput{}); err != nil {
			return nil, err
		}
	}

	// Reuse an existing webhook secret — regenerating it silently breaks a
	// webhook the owner already configured in the OC dashboard — unless the
	// caller explicitly asks to rotate (compromised/lost URL).
	secret := ""
	if st.row.OcWebhookSecret != nil && !rotateSecret {
		secret = *st.row.OcWebhookSecret
	}
	if secret == "" {
		secretBytes := make([]byte, 16)
		if _, err := rand.Read(secretBytes); err != nil {
			return nil, fmt.Errorf("generating webhook secret: %w", err)
		}
		secret = hex.EncodeToString(secretBytes)
	}

	// A slug-only reconnect preserves a stored token — wiping it on every
	// connect would break sync for owners who only meant to rotate the
	// secret or fix the slug.
	if encrypted == nil {
		if row, err := s.store.getSettings(ctx, owner.Type, owner.ID); err == nil && row.OcApiTokenEncrypted != nil {
			encrypted = row.OcApiTokenEncrypted
		}
	}

	if err := s.store.setOCCredentials(ctx, gen.SetOCCredentialsParams{
		OwnerType:           owner.Type,
		OwnerID:             owner.ID,
		OcCollectiveSlug:    &slug,
		OcApiTokenEncrypted: encrypted,
		OcWebhookSecret:     &secret,
	}); err != nil {
		return nil, err
	}

	out, err := s.GetSettings(ctx, owner)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// DisconnectOC removes the collective link and stored token.
func (s *Service) DisconnectOC(ctx context.Context, owner Owner, actorID pgtype.UUID) error {
	if err := s.ensureWritable(ctx, owner, actorID); err != nil {
		return err
	}
	return s.store.clearOCCredentials(ctx, owner.Type, owner.ID)
}

// ImportContribution records one OC contribution as a completed tip.
// Idempotent on oc_contribution_id; sender_user_id stays NULL when no
// platform account matches the OC contributor email.
func (s *Service) ImportContribution(ctx context.Context, owner Owner, c OCContribution) (bool, error) {
	if _, err := s.store.getTipByOCContributionID(ctx, c.ID); err == nil {
		return false, nil // already imported
	} else if !errors.Is(err, ErrNotFound) {
		return false, err
	}

	// OC does not expose contributor emails to personal tokens, so this
	// match is normally a no-op — sender_user_id stays NULL per §125. The
	// lookup remains for sources that do supply an email.
	var senderID pgtype.UUID
	if c.FromEmail != "" {
		if id, err := s.store.getUserIDByEmail(ctx, c.FromEmail); err == nil {
			senderID = id
		}
	}

	params := gen.InsertTipParams{
		SenderUserID:       senderID,
		RecipientType:      owner.Type,
		Method:             MethodOpenCollective,
		FiatAmount:         &c.AmountCents,
		FiatCurrency:       &c.Currency,
		OcContributionID:   &c.ID,
		ExternalSenderName: strPtrOrNil(c.FromName),
		Status:             StatusCompleted,
	}
	if owner.Type == RecipientUser {
		params.RecipientUserID = owner.ID
	} else if owner.Type == RecipientOrganization {
		params.RecipientOrganizationID = owner.ID
	}

	if _, err := s.store.insertTip(ctx, params); err != nil {
		return false, fmt.Errorf("importing OC contribution: %w", err)
	}
	return true, nil
}

// ImportOCWebhook resolves the owner from the per-collective webhook secret
// and imports the contribution. Returns ErrNotFound for an unknown secret so
// the handler can 401; duplicate imports are idempotent (the UNIQUE index on
// oc_contribution_id also catches replays).
func (s *Service) ImportOCWebhook(ctx context.Context, secret string, c OCContribution) error {
	row, err := s.store.getSettingsByOCWebhookSecret(ctx, secret)
	if err != nil {
		return err // ErrNotFound → 401 at the handler
	}
	owner := Owner{Type: row.OwnerType, ID: row.OwnerID}
	_, err = s.ImportContribution(ctx, owner, c)
	return err
}

// SyncAll iterates settings rows holding an OC token and imports new
// contributions for each collective. Called by the tips:oc_sync task
// (queue 'low'); per-owner failures are logged and skipped so one bad
// collective can't stall the sweep.
func (s *Service) SyncAll(ctx context.Context) error {
	if s.oc == nil || !s.ocEnabled(ctx) {
		return nil
	}
	rows, err := s.store.listSettingsWithOCToken(ctx)
	if err != nil {
		return err
	}
	for _, row := range rows {
		if err := s.syncOne(ctx, row); err != nil {
			slog.Warn("tips: oc sync failed", "owner", row.OwnerType+":"+row.OwnerID.String(), "error", err)
		}
	}
	// Platform collective: slug + token live in sensitive system_settings
	// (tips.oc_platform_collective / tips.oc_platform_token), not on a
	// tip_settings row — sync it as the platform owner.
	if s.settings != nil && s.oc != nil {
		slug := s.settings.GetStringOr(ctx, "tips.oc_platform_collective", "")
		token := s.settings.GetStringOr(ctx, "tips.oc_platform_token", "")
		if slug != "" && token != "" {
			if err := s.syncCollective(ctx, PlatformOwner(), slug, token); err != nil {
				slog.Warn("tips: platform oc sync failed", "error", err)
			}
		}
	}
	return nil
}

func (s *Service) syncOne(ctx context.Context, row gen.TipSetting) error {
	if row.OcCollectiveSlug == nil || row.OcApiTokenEncrypted == nil {
		return nil
	}
	token, err := crypto.Decrypt(*row.OcApiTokenEncrypted)
	if err != nil {
		return fmt.Errorf("decrypting OC token: %w", err)
	}
	return s.syncCollective(ctx, Owner{Type: row.OwnerType, ID: row.OwnerID}, *row.OcCollectiveSlug, token)
}

// syncCollective pages a collective's CREDIT transactions and imports new
// contributions under a per-owner sync lock.
func (s *Service) syncCollective(ctx context.Context, owner Owner, slug, token string) error {
	// jc:tips:oc_sync_lock:{owner_id} — one sync per owner at a time. Nil
	// locker (tests) proceeds unlocked. Unlock releases on completion so the
	// next cron cycle isn't delayed by the TTL; only released when TryLock
	// actually acquired — a fail-open path must never delete another
	// process's lock.
	lockKey := "tips:oc_sync_lock:" + owner.ID.String()
	if s.locker != nil {
		ok, err := s.locker.TryLock(ctx, lockKey, 900)
		if err == nil {
			if !ok {
				return nil
			}
			defer func() { _ = s.locker.Unlock(ctx, lockKey) }()
		}
	}

	// Transactions arrive newest-first. Once a full page yields zero new
	// imports everything older is already deduped — stop instead of walking
	// the collective's entire history every cycle.
	for offset := 0; ; offset += 100 {
		contribs, err := s.oc.ListContributions(ctx, slug, token, 100, offset)
		if err != nil {
			return err
		}
		newCount := 0
		for _, c := range contribs {
			imported, err := s.ImportContribution(ctx, owner, OCContribution{
				ID:          c.ID,
				AmountCents: c.AmountCents,
				Currency:    c.Currency,
				FromName:    c.FromName,
				FromSlug:    c.FromSlug,
				FromEmail:   c.FromEmail,
			})
			if err != nil {
				return err
			}
			if imported {
				newCount++
			}
		}
		if len(contribs) < 100 || newCount == 0 {
			return nil
		}
	}
}
