package tips

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/db/gen"
)

// tipStore is the persistence boundary for tips. *Store satisfies it against
// Postgres; unit tests substitute fakes (mentorship BookingBackend pattern).
type tipStore interface {
	getSettings(ctx context.Context, ownerType string, ownerID pgtype.UUID) (gen.TipSetting, error)
	upsertSettings(ctx context.Context, p gen.UpsertTipSettingsParams) (gen.TipSetting, error)
	getSettingsByOCWebhookSecret(ctx context.Context, secret string) (gen.TipSetting, error)
	setOCCredentials(ctx context.Context, p gen.SetOCCredentialsParams) error
	clearOCCredentials(ctx context.Context, ownerType string, ownerID pgtype.UUID) error
	listSettingsWithOCToken(ctx context.Context) ([]gen.TipSetting, error)

	createLink(ctx context.Context, p gen.CreateTipExternalLinkParams) (gen.TipExternalLink, error)
	updateLink(ctx context.Context, p gen.UpdateTipExternalLinkParams) (gen.TipExternalLink, error)
	deleteLink(ctx context.Context, id, settingsID pgtype.UUID) error
	listLinks(ctx context.Context, settingsID pgtype.UUID) ([]gen.TipExternalLink, error)

	getTip(ctx context.Context, id pgtype.UUID) (gen.Tip, error)
	getTipByIdempotencyKey(ctx context.Context, senderID pgtype.UUID, key string) (gen.Tip, error)
	getTipByPaymentID(ctx context.Context, paymentID pgtype.UUID) (gen.Tip, error)
	getTipByOCContributionID(ctx context.Context, ocID string) (gen.Tip, error)
	insertTip(ctx context.Context, p gen.InsertTipParams) (gen.Tip, error)
	updateTipStatus(ctx context.Context, id pgtype.UUID, status string) error
	attachTipPayment(ctx context.Context, id, paymentID pgtype.UUID, checkoutURL string) error
	settleTip(ctx context.Context, id pgtype.UUID, netCredits int64) error
	// failTip marks a pending tip failed and releases its idempotency key.
	failTip(ctx context.Context, id pgtype.UUID) error
	// insertAuditLog writes a moderation audit trail row (admin refund).
	insertAuditLog(ctx context.Context, actorID pgtype.UUID, action, targetType, targetID, metadata string) error

	listTipsSent(ctx context.Context, senderID pgtype.UUID, limit, offset int32) ([]gen.Tip, error)
	listTipsReceived(ctx context.Context, userID pgtype.UUID, limit, offset int32) ([]gen.Tip, error)
	countTipsReceived(ctx context.Context, userID pgtype.UUID) (int32, error)
	sumTipperPointsToday(ctx context.Context, senderID pgtype.UUID) (int64, error)
	sumRecipientPointsToday(ctx context.Context, recipientID pgtype.UUID) (int64, error)
	getContentTitle(ctx context.Context, contentID pgtype.UUID) (string, error)
	contentTipSummary(ctx context.Context, contentID pgtype.UUID) (gen.ContentTipSummaryRow, error)
	adminListTips(ctx context.Context, p gen.AdminListTipsParams) ([]gen.Tip, error)
	adminCountTips(ctx context.Context, p gen.AdminCountTipsParams) (int32, error)
	adminTipStats(ctx context.Context, since pgtype.Timestamptz) (gen.AdminTipStatsRow, error)
	adminTipStatsByCurrency(ctx context.Context, since pgtype.Timestamptz) ([]gen.AdminTipStatsByCurrencyRow, error)
	listStalePendingTips(ctx context.Context, hours, lim int32) ([]pgtype.UUID, error)
	getUserDisplayNames(ctx context.Context, ids []pgtype.UUID) ([]gen.GetUserDisplayNamesRow, error)
	getOrgDisplayNames(ctx context.Context, ids []pgtype.UUID) ([]gen.GetOrgDisplayNamesRow, error)
	isMentor(ctx context.Context, userID pgtype.UUID) (bool, error)
	getCurrencyByCode(ctx context.Context, code string) (gen.Currency, error)
	getContentAuthorID(ctx context.Context, contentID pgtype.UUID) (pgtype.UUID, error)
	getUserIDByUsername(ctx context.Context, username string) (pgtype.UUID, error)
	getUserIDByEmail(ctx context.Context, email string) (pgtype.UUID, error)
	getUserEmail(ctx context.Context, userID pgtype.UUID) (string, error)
	getOrgOwnerID(ctx context.Context, orgID pgtype.UUID) (pgtype.UUID, error)

	// settleWalletTip atomically debits sender + credits recipient on the
	// wallet_ledger (advisory-locked pair, deterministic lock order) and
	// inserts the completed tip row — the §125 points/credits path.
	settleWalletTip(ctx context.Context, p settleWalletParams) (gen.Tip, error)
	// settleFiatTip credits the recipient's credits balance and marks the
	// tip completed — the post-payment branch of the fiat path.
	settleFiatTip(ctx context.Context, tipID pgtype.UUID, netCredits int64, walletOwnerID pgtype.UUID) error
	// clawbackTip debits the recipient's credits balance (clawback op) and
	// marks the tip refunded — the §125 refund path.
	clawbackTip(ctx context.Context, tipID pgtype.UUID, walletOwnerID pgtype.UUID) error
}

// Store provides low-level database access for tip operations.
type Store struct {
	read  *gen.Queries
	write *gen.Queries
	db    *db.DB
}

// NewStore creates a Store backed by the given database pool.
func NewStore(database *db.DB) *Store {
	return &Store{
		read:  gen.New(database.Reader),
		write: gen.New(database.Writer),
		db:    database,
	}
}

// isBlocked runs the §41 user_blocks EXISTS check (concrete *Store only —
// test fakes don't reach this path).
func (s *Store) isBlocked(ctx context.Context, a, b pgtype.UUID) (bool, error) {
	return s.read.IsBlocked(ctx, gen.IsBlockedParams{BlockerID: a, BlockedID: b})
}

func (s *Store) getSettings(ctx context.Context, ownerType string, ownerID pgtype.UUID) (gen.TipSetting, error) {
	row, err := s.read.GetTipSettings(ctx, gen.GetTipSettingsParams{OwnerType: ownerType, OwnerID: ownerID})
	if err != nil {
		return gen.TipSetting{}, mapNotFound(err, ErrNotFound)
	}
	return row, nil
}

func (s *Store) upsertSettings(ctx context.Context, p gen.UpsertTipSettingsParams) (gen.TipSetting, error) {
	row, err := s.write.UpsertTipSettings(ctx, p)
	if err != nil {
		return gen.TipSetting{}, fmt.Errorf("upserting tip settings: %w", err)
	}
	return row, nil
}

func (s *Store) getSettingsByOCWebhookSecret(ctx context.Context, secret string) (gen.TipSetting, error) {
	row, err := s.read.GetTipSettingsByOCWebhookSecret(ctx, &secret)
	if err != nil {
		return gen.TipSetting{}, mapNotFound(err, ErrNotFound)
	}
	return row, nil
}

func (s *Store) setOCCredentials(ctx context.Context, p gen.SetOCCredentialsParams) error {
	return s.write.SetOCCredentials(ctx, p)
}

func (s *Store) clearOCCredentials(ctx context.Context, ownerType string, ownerID pgtype.UUID) error {
	return s.write.ClearOCCredentials(ctx, gen.ClearOCCredentialsParams{OwnerType: ownerType, OwnerID: ownerID})
}

func (s *Store) listSettingsWithOCToken(ctx context.Context) ([]gen.TipSetting, error) {
	return s.read.ListTipSettingsWithOCToken(ctx)
}

func (s *Store) createLink(ctx context.Context, p gen.CreateTipExternalLinkParams) (gen.TipExternalLink, error) {
	return s.write.CreateTipExternalLink(ctx, p)
}

func (s *Store) updateLink(ctx context.Context, p gen.UpdateTipExternalLinkParams) (gen.TipExternalLink, error) {
	row, err := s.write.UpdateTipExternalLink(ctx, p)
	if err != nil {
		return gen.TipExternalLink{}, mapNotFound(err, ErrNotFound)
	}
	return row, nil
}

func (s *Store) deleteLink(ctx context.Context, id, settingsID pgtype.UUID) error {
	return s.write.DeleteTipExternalLink(ctx, gen.DeleteTipExternalLinkParams{ID: id, TipSettingsID: settingsID})
}

func (s *Store) listLinks(ctx context.Context, settingsID pgtype.UUID) ([]gen.TipExternalLink, error) {
	return s.read.ListTipExternalLinks(ctx, settingsID)
}

func (s *Store) getTip(ctx context.Context, id pgtype.UUID) (gen.Tip, error) {
	row, err := s.read.GetTip(ctx, id)
	if err != nil {
		return gen.Tip{}, mapNotFound(err, ErrNotFound)
	}
	return row, nil
}

func (s *Store) getTipByIdempotencyKey(ctx context.Context, senderID pgtype.UUID, key string) (gen.Tip, error) {
	row, err := s.read.GetTipByIdempotencyKey(ctx, gen.GetTipByIdempotencyKeyParams{SenderUserID: senderID, IdempotencyKey: &key})
	if err != nil {
		return gen.Tip{}, mapNotFound(err, ErrNotFound)
	}
	return row, nil
}

func (s *Store) getTipByPaymentID(ctx context.Context, paymentID pgtype.UUID) (gen.Tip, error) {
	row, err := s.read.GetTipByPaymentID(ctx, paymentID)
	if err != nil {
		return gen.Tip{}, mapNotFound(err, ErrNotFound)
	}
	return row, nil
}

func (s *Store) getTipByOCContributionID(ctx context.Context, ocID string) (gen.Tip, error) {
	row, err := s.read.GetTipByOCContributionID(ctx, &ocID)
	if err != nil {
		return gen.Tip{}, mapNotFound(err, ErrNotFound)
	}
	return row, nil
}

func (s *Store) insertTip(ctx context.Context, p gen.InsertTipParams) (gen.Tip, error) {
	t, err := s.write.InsertTip(ctx, p)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return gen.Tip{}, ErrDuplicateKey
		}
		return gen.Tip{}, err
	}
	return t, nil
}

func (s *Store) updateTipStatus(ctx context.Context, id pgtype.UUID, status string) error {
	return s.write.UpdateTipStatus(ctx, gen.UpdateTipStatusParams{ID: id, Status: status})
}

func (s *Store) attachTipPayment(ctx context.Context, id, paymentID pgtype.UUID, checkoutURL string) error {
	return s.write.AttachTipPayment(ctx, gen.AttachTipPaymentParams{ID: id, PaymentID: paymentID, CheckoutUrl: &checkoutURL})
}

func (s *Store) listStalePendingTips(ctx context.Context, hours int32, lim int32) ([]pgtype.UUID, error) {
	return s.read.ListStalePendingTips(ctx, gen.ListStalePendingTipsParams{Hours: hours, Lim: lim})
}

func (s *Store) adminTipStatsByCurrency(ctx context.Context, since pgtype.Timestamptz) ([]gen.AdminTipStatsByCurrencyRow, error) {
	return s.read.AdminTipStatsByCurrency(ctx, since)
}

func (s *Store) settleTip(ctx context.Context, id pgtype.UUID, netCredits int64) error {
	return s.write.SettleTip(ctx, gen.SettleTipParams{ID: id, NetCredits: netCredits})
}

func (s *Store) failTip(ctx context.Context, id pgtype.UUID) error {
	return s.write.FailTip(ctx, id)
}

// insertAuditLog records the admin refund in audit_logs (same shape as the
// other moderation writes — actor, action, target, jsonb metadata).
func (s *Store) insertAuditLog(ctx context.Context, actorID pgtype.UUID, action, targetType, targetID, metadata string) error {
	_, err := s.db.Writer.Exec(ctx, `
		INSERT INTO audit_logs (actor_user_id, action_type, target_type, target_id, metadata)
		VALUES ($1, $2, $3, $4::uuid, $5::jsonb)`,
		actorID, action, targetType, targetID, metadata)
	return err
}

func (s *Store) listTipsSent(ctx context.Context, senderID pgtype.UUID, limit, offset int32) ([]gen.Tip, error) {
	return s.read.ListTipsSent(ctx, gen.ListTipsSentParams{SenderUserID: senderID, PageLimit: limit, PageOffset: offset})
}

func (s *Store) listTipsReceived(ctx context.Context, userID pgtype.UUID, limit, offset int32) ([]gen.Tip, error) {
	return s.read.ListTipsReceived(ctx, gen.ListTipsReceivedParams{RecipientUserID: userID, PageLimit: limit, PageOffset: offset})
}

func (s *Store) countTipsReceived(ctx context.Context, userID pgtype.UUID) (int32, error) {
	return s.read.CountTipsReceived(ctx, userID)
}

func (s *Store) sumTipperPointsToday(ctx context.Context, senderID pgtype.UUID) (int64, error) {
	return s.read.SumTipperPointsToday(ctx, senderID)
}

func (s *Store) sumRecipientPointsToday(ctx context.Context, recipientID pgtype.UUID) (int64, error) {
	return s.read.SumRecipientPointsToday(ctx, recipientID)
}

func (s *Store) getContentTitle(ctx context.Context, contentID pgtype.UUID) (string, error) {
	t, err := s.read.GetContentTitle(ctx, contentID)
	if err != nil {
		return "", mapNotFound(err, ErrNotFound)
	}
	return t, nil
}

func (s *Store) contentTipSummary(ctx context.Context, contentID pgtype.UUID) (gen.ContentTipSummaryRow, error) {
	return s.read.ContentTipSummary(ctx, contentID)
}

func (s *Store) adminListTips(ctx context.Context, p gen.AdminListTipsParams) ([]gen.Tip, error) {
	return s.read.AdminListTips(ctx, p)
}

func (s *Store) adminCountTips(ctx context.Context, p gen.AdminCountTipsParams) (int32, error) {
	return s.read.AdminCountTips(ctx, p)
}

func (s *Store) adminTipStats(ctx context.Context, since pgtype.Timestamptz) (gen.AdminTipStatsRow, error) {
	return s.read.AdminTipStats(ctx, since)
}

func (s *Store) getUserDisplayNames(ctx context.Context, ids []pgtype.UUID) ([]gen.GetUserDisplayNamesRow, error) {
	return s.read.GetUserDisplayNames(ctx, ids)
}

func (s *Store) getOrgDisplayNames(ctx context.Context, ids []pgtype.UUID) ([]gen.GetOrgDisplayNamesRow, error) {
	return s.read.GetOrgDisplayNames(ctx, ids)
}

func (s *Store) isMentor(ctx context.Context, userID pgtype.UUID) (bool, error) {
	v, err := s.read.IsMentorProfile(ctx, userID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil
		}
		return false, err
	}
	return v, nil
}

func (s *Store) getCurrencyByCode(ctx context.Context, code string) (gen.Currency, error) {
	row, err := s.read.GetCurrencyByCode(ctx, code)
	if err != nil {
		return gen.Currency{}, mapNotFound(err, ErrNotFound)
	}
	return row, nil
}

func (s *Store) getContentAuthorID(ctx context.Context, contentID pgtype.UUID) (pgtype.UUID, error) {
	id, err := s.read.GetContentAuthorID(ctx, contentID)
	if err != nil {
		return pgtype.UUID{}, mapNotFound(err, ErrNotFound)
	}
	return id, nil
}

func (s *Store) getUserIDByUsername(ctx context.Context, username string) (pgtype.UUID, error) {
	row, err := s.read.GetUserByUsername(ctx, username)
	if err != nil {
		return pgtype.UUID{}, mapNotFound(err, ErrNotFound)
	}
	return row.UserID, nil
}

func (s *Store) getUserIDByEmail(ctx context.Context, email string) (pgtype.UUID, error) {
	id, err := s.read.GetUserIDByEmail(ctx, email)
	if err != nil {
		return pgtype.UUID{}, mapNotFound(err, ErrNotFound)
	}
	return id, nil
}

func (s *Store) getUserEmail(ctx context.Context, userID pgtype.UUID) (string, error) {
	return s.read.GetUserEmail(ctx, userID)
}

func (s *Store) getOrgOwnerID(ctx context.Context, orgID pgtype.UUID) (pgtype.UUID, error) {
	org, err := s.read.GetOrganizationByID(ctx, orgID)
	if err != nil {
		return pgtype.UUID{}, mapNotFound(err, ErrNotFound)
	}
	return org.OwnerUserID, nil
}

// mapNotFound converts pgx.ErrNoRows into the package sentinel while
// preserving other errors.
func mapNotFound(err error, sentinel error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return sentinel
	}
	return err
}
