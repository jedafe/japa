// kofibmc.go — §125 Ko-fi (inbound webhook) and Buy Me a Coffee (scheduled
// supporters poll) donation imports. Both share the OC contribution-insert
// path via importContribution; dedupe ids are namespaced
// ('kofi:<message_id>', 'bmc:<support_id>') into oc_contribution_id.
package tips

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/crypto"
	"github.com/jedafe/japa/internal/db/gen"
)

func (s *Service) kofiEnabled(ctx context.Context) bool {
	if s.features != nil {
		return s.features.IsEnabled(ctx, "tips.kofi")
	}
	return s.settings != nil && s.settings.GetBoolOr(ctx, "tips.kofi", false)
}

func (s *Service) bmcEnabled(ctx context.Context) bool {
	if s.features != nil {
		return s.features.IsEnabled(ctx, "tips.buymeacoffee")
	}
	return s.settings != nil && s.settings.GetBoolOr(ctx, "tips.buymeacoffee", false)
}

// ConnectKofi generates (or rotates) the owner's Ko-fi webhook secret. The
// returned Settings carries kofi_webhook_url + kofi_verification_token —
// both go into the owner's Ko-fi dashboard.
func (s *Service) ConnectKofi(ctx context.Context, owner Owner, actorID pgtype.UUID, rotate bool) (*Settings, error) {
	if err := s.ensureWritable(ctx, owner, actorID); err != nil {
		return nil, err
	}
	if err := s.ensureExternalEligible(ctx, owner, actorID); err != nil {
		return nil, err
	}
	st, err := s.GetSettings(ctx, owner)
	if err != nil {
		return nil, err
	}
	if !st.hasRow {
		if _, err := s.UpsertSettings(ctx, owner, UpdateSettingsInput{}); err != nil {
			return nil, err
		}
	}

	secret := ""
	if st.row.KofiWebhookSecret != nil && !rotate {
		secret = *st.row.KofiWebhookSecret
	}
	if secret == "" {
		b := make([]byte, 16)
		if _, err := rand.Read(b); err != nil {
			return nil, fmt.Errorf("generating kofi secret: %w", err)
		}
		secret = hex.EncodeToString(b)
	}
	if err := s.store.setKofiSecret(ctx, owner.Type, owner.ID, secret); err != nil {
		return nil, err
	}
	out, err := s.GetSettings(ctx, owner)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// DisconnectKofi clears the owner's webhook secret.
func (s *Service) DisconnectKofi(ctx context.Context, owner Owner, actorID pgtype.UUID) error {
	if err := s.ensureWritable(ctx, owner, actorID); err != nil {
		return err
	}
	return s.store.clearKofiSecret(ctx, owner.Type, owner.ID)
}

// ConnectBMC verifies a personal access token against the BMC API, stores it
// AES-256-GCM encrypted, and saves the optional page slug.
func (s *Service) ConnectBMC(ctx context.Context, owner Owner, actorID pgtype.UUID, token, slug string) (*Settings, error) {
	if err := s.ensureWritable(ctx, owner, actorID); err != nil {
		return nil, err
	}
	if err := s.ensureExternalEligible(ctx, owner, actorID); err != nil {
		return nil, err
	}
	if s.bmc == nil {
		return nil, ErrOCNotConnected
	}
	if _, _, err := s.bmc.ListSupporters(ctx, token, 1); err != nil {
		return nil, ErrOCAccess
	}
	enc, err := crypto.Encrypt(token)
	if err != nil {
		return nil, fmt.Errorf("encrypting BMC token: %w", err)
	}
	st, err := s.GetSettings(ctx, owner)
	if err != nil {
		return nil, err
	}
	if !st.hasRow {
		if _, err := s.UpsertSettings(ctx, owner, UpdateSettingsInput{}); err != nil {
			return nil, err
		}
	}
	var slugPtr *string
	if slug != "" {
		slugPtr = &slug
	} else if st.row.BmcPageSlug != nil {
		slugPtr = st.row.BmcPageSlug // preserve on token-only reconnect
	}
	if err := s.store.setBMCCredentials(ctx, owner.Type, owner.ID, slugPtr, &enc); err != nil {
		return nil, err
	}
	out, err := s.GetSettings(ctx, owner)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// DisconnectBMC clears stored credentials.
func (s *Service) DisconnectBMC(ctx context.Context, owner Owner, actorID pgtype.UUID) error {
	if err := s.ensureWritable(ctx, owner, actorID); err != nil {
		return err
	}
	return s.store.clearBMCCredentials(ctx, owner.Type, owner.ID)
}

// ImportKofiWebhook resolves the owner by verification token and imports the
// donation. token must equal the stored secret (URL + payload both carry it).
func (s *Service) ImportKofiWebhook(ctx context.Context, secret, token string, c OCContribution) error {
	if !s.kofiEnabled(ctx) {
		return ErrNotFound
	}
	row, err := s.store.getSettingsByKofiWebhookSecret(ctx, secret)
	if err != nil {
		return err // ErrNotFound → 401
	}
	if row.KofiWebhookSecret == nil || *row.KofiWebhookSecret != token {
		return ErrNotFound
	}
	_, err = s.importContribution(ctx, Owner{Type: row.OwnerType, ID: row.OwnerID}, MethodKofi, c)
	return err
}

// SyncBMC sweeps every settings row holding a BMC token and imports new
// supporters — the tips:bmc_sync task path (queue 'low').
func (s *Service) SyncBMC(ctx context.Context) error {
	if s.bmc == nil || !s.bmcEnabled(ctx) {
		return nil
	}
	rows, err := s.store.listSettingsWithBMCToken(ctx)
	if err != nil {
		return err
	}
	for _, row := range rows {
		if err := s.syncBMCOne(ctx, row); err != nil {
			slog.Warn("tips: bmc sync failed", "owner", row.OwnerType+":"+row.OwnerID.String(), "error", err)
		}
	}
	return nil
}

func (s *Service) syncBMCOne(ctx context.Context, row gen.TipSetting) error {
	if row.BmcApiTokenEncrypted == nil || *row.BmcApiTokenEncrypted == "" {
		return nil
	}
	token, err := crypto.Decrypt(*row.BmcApiTokenEncrypted)
	if err != nil {
		return fmt.Errorf("decrypting BMC token: %w", err)
	}
	owner := Owner{Type: row.OwnerType, ID: row.OwnerID}
	lockKey := "tips:bmc_sync_lock:" + owner.ID.String()
	if s.locker != nil {
		if ok, lerr := s.locker.TryLock(ctx, lockKey, 900); lerr == nil {
			if !ok {
				return nil
			}
			defer func() { _ = s.locker.Unlock(ctx, lockKey) }()
		}
	}
	for page := 1; ; page++ {
		supporters, hasMore, err := s.bmc.ListSupporters(ctx, token, page)
		if err != nil {
			return err
		}
		newCount := 0
		for _, sp := range supporters {
			imported, err := s.importContribution(ctx, owner, MethodBuyMeACoffee, OCContribution{
				ID:          "bmc:" + sp.ID,
				AmountCents: sp.AmountCents,
				Currency:    sp.Currency,
				FromName:    sp.Name,
				FromEmail:   sp.Email,
			})
			if err != nil {
				return err
			}
			if imported {
				newCount++
			}
		}
		if !hasMore || newCount == 0 {
			return nil
		}
	}
}
