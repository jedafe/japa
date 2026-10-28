// Package tips implements the Donations & Tips system (Section 125).
// Three recipient kinds — platform, member (creator/mentor), organization —
// settle over four methods: points/credits wallet-ledger pairs
// (source='tip'), fiat via §17 payments (purpose_type='donation') settling
// to recipient credits, and imported Open Collective contributions.
package tips

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db/gen"
)

// Recipient kinds.
const (
	RecipientPlatform     = "platform"
	RecipientUser         = "user"
	RecipientOrganization = "organization"
)

// Settle methods (tips.method CHECK mirror).
const (
	MethodPoints         = "points"
	MethodCredits        = "credits"
	MethodOpenCollective = "opencollective"
)

// Tip statuses.
const (
	StatusPending   = "pending"
	StatusCompleted = "completed"
	StatusFailed    = "failed"
	StatusRefunded  = "refunded"
)

// LedgerSourceTip is the wallet_ledger.source value for tip settlement pairs.
const LedgerSourceTip = "tip"

// PlatformOwnerID is the zero-uuid sentinel owning the platform tip_settings
// row (spec §Concepts — platform owner).
const PlatformOwnerID = "00000000-0000-0000-0000-000000000000"

// maxExternalLinks caps outbound links per owner (spec: display-only links).
const maxExternalLinks = 10

var (
	ErrTipsDisabled        = errors.New("tips are disabled")
	ErrRecipientNoTips     = errors.New("recipient does not accept tips")
	ErrMethodNotAccepted   = errors.New("recipient does not accept this tip method")
	ErrBelowMinimum        = errors.New("tip below recipient minimum")
	ErrAboveMaximum        = errors.New("tip above maximum")
	ErrSelfTip             = errors.New("cannot tip yourself")
	ErrInsufficientBalance = errors.New("insufficient balance")
	ErrDailyPointsLimit    = errors.New("daily points tip limit reached")
	ErrNotEligible         = errors.New("tips.receive permission or mentor profile required")
	ErrNotFound            = errors.New("tip or settings not found")
	ErrInvalidMethod       = errors.New("invalid tip method")
	ErrInvalidRecipient    = errors.New("invalid tip recipient")
	ErrInvalidAmount       = errors.New("invalid tip amount")
	ErrInvalidLink         = errors.New("invalid external link")
	ErrTooManyLinks        = errors.New("too many external links")
	ErrNotAuthorized       = errors.New("not authorized for this organization")
	ErrOCNotConnected      = errors.New("open collective not connected")
	ErrAlreadySettled      = errors.New("tip already settled")
	ErrDuplicateKey        = errors.New("duplicate idempotency key")
	ErrInvalidStatus       = errors.New("invalid tip status for this operation")
)

// fiatMethods are the §17 payment providers usable for fiat tips.
var fiatMethods = map[string]bool{
	"stripe": true, "paystack": true, "nowpayments": true, "manual_transfer": true,
}

// IsFiatMethod reports whether m settles via a payment provider.
func IsFiatMethod(m string) bool { return fiatMethods[m] }

// IsValidMethod reports whether m is a tips.method enum value.
func IsValidMethod(m string) bool {
	return m == MethodPoints || m == MethodCredits || m == MethodOpenCollective || fiatMethods[m]
}

// Owner identifies a tip_settings row's owner.
type Owner struct {
	Type string
	ID   pgtype.UUID
}

// UserOwner returns the owner handle for a member.
func UserOwner(id pgtype.UUID) Owner { return Owner{Type: RecipientUser, ID: id} }

// OrgOwner returns the owner handle for an organization.
func OrgOwner(id pgtype.UUID) Owner { return Owner{Type: RecipientOrganization, ID: id} }

// PlatformOwner returns the zero-uuid platform owner handle.
func PlatformOwner() Owner {
	var id pgtype.UUID
	_ = id.Scan(PlatformOwnerID)
	return Owner{Type: RecipientPlatform, ID: id}
}

// Settings is the service-level view of a tip_settings row. The OC token
// ciphertext never leaves the service — HasOCToken derives presence.
type Settings struct {
	ID               pgtype.UUID `json:"id"`
	OwnerType        string      `json:"owner_type"`
	OwnerID          pgtype.UUID `json:"owner_id"`
	TipsEnabled      bool        `json:"tips_enabled"`
	AcceptPoints     bool        `json:"accept_points"`
	AcceptCredits    bool        `json:"accept_credits"`
	AcceptFiat       bool        `json:"accept_fiat"`
	AcceptExternal   bool        `json:"accept_external"`
	OCCollectiveSlug *string     `json:"oc_collective_slug,omitempty"`
	HasOCToken       bool        `json:"has_oc_token"`
	OCWebhookURL     *string     `json:"oc_webhook_url,omitempty"`
	MinTipAmount     int64       `json:"min_tip_amount"`
	ThankYouMessage  *string     `json:"thank_you_message,omitempty"`
	row              gen.TipSetting
	hasRow           bool
}

// UpdateSettingsInput is the owner-editable settings patch. Nil fields keep
// their current values (PUT semantics: body sends all fields).
type UpdateSettingsInput struct {
	TipsEnabled     bool    `json:"tips_enabled"`
	AcceptPoints    bool    `json:"accept_points"`
	AcceptCredits   bool    `json:"accept_credits"`
	AcceptFiat      bool    `json:"accept_fiat"`
	AcceptExternal  bool    `json:"accept_external"`
	MinTipAmount    int64   `json:"min_tip_amount"`
	ThankYouMessage *string `json:"thank_you_message"`
}

// ExternalLink is a display-only outbound tip option.
type ExternalLink struct {
	ID       pgtype.UUID `json:"id"`
	Kind     string      `json:"kind"`
	Label    string      `json:"label"`
	URL      string      `json:"url"`
	Position int32       `json:"position"`
}

// LinkInput is the create/update payload for an external link.
type LinkInput struct {
	Kind     string `json:"kind"`
	Label    string `json:"label"`
	URL      string `json:"url"`
	Position int32  `json:"position"`
}

// PublicConfig is what unauthenticated viewers learn about a recipient.
type PublicConfig struct {
	Enabled          bool           `json:"enabled"`
	Methods          []string       `json:"methods"`
	MinTipAmount     int64          `json:"min_tip_amount"`
	OCCollectiveSlug *string        `json:"oc_collective_slug,omitempty"`
	ExternalLinks    []ExternalLink `json:"external_links"`
	ThankYouMessage  *string        `json:"thank_you_message,omitempty"`
}

// Tip is the service-level tip record.
type Tip struct {
	ID              pgtype.UUID        `json:"id"`
	SenderUserID    pgtype.UUID        `json:"sender_user_id,omitempty"`
	SenderName      string             `json:"sender_name"`
	RecipientType   string             `json:"recipient_type"`
	RecipientUserID pgtype.UUID        `json:"recipient_user_id,omitempty"`
	RecipientOrgID  pgtype.UUID        `json:"recipient_organization_id,omitempty"`
	RecipientName   string             `json:"recipient_name,omitempty"`
	ContentID       pgtype.UUID        `json:"content_id,omitempty"`
	Method          string             `json:"method"`
	PointsAmount    int64              `json:"points_amount"`
	CreditsAmount   int64              `json:"credits_amount"`
	FiatAmount      *int64             `json:"fiat_amount,omitempty"`
	FiatCurrency    *string            `json:"fiat_currency,omitempty"`
	FeeAmount       int64              `json:"fee_amount"`
	NetCredits      int64              `json:"net_credits"`
	PaymentID       pgtype.UUID        `json:"payment_id,omitempty"`
	Message         *string            `json:"message,omitempty"`
	IsAnonymous     bool               `json:"is_anonymous"`
	Status          string             `json:"status"`
	CreatedAt       pgtype.Timestamptz `json:"created_at"`
}

// CreateTipInput is the POST /tips payload.
type CreateTipInput struct {
	RecipientType   string      `json:"recipient_type"` // user|organization|platform
	RecipientUserID pgtype.UUID `json:"-"`
	RecipientOrgID  pgtype.UUID `json:"-"`
	ContentID       pgtype.UUID `json:"-"`
	Method          string      `json:"method"`
	PointsAmount    int64       `json:"points_amount,omitempty"`
	CreditsAmount   int64       `json:"credits_amount,omitempty"`
	FiatAmount      int64       `json:"fiat_amount,omitempty"`
	FiatCurrency    string      `json:"fiat_currency,omitempty"`
	Message         string      `json:"message,omitempty"`
	IsAnonymous     bool        `json:"is_anonymous"`
}

// CreateTipResult carries the tip plus a checkout URL for fiat methods.
type CreateTipResult struct {
	Tip         Tip    `json:"tip"`
	CheckoutURL string `json:"checkout_url,omitempty"`
	Duplicate   bool   `json:"-"`
}

// --- Dependency seams -------------------------------------------------------

// SettingsReader reads global system_settings with fallbacks.
type SettingsReader interface {
	GetIntOr(ctx context.Context, key string, fallback int) int
	GetBoolOr(ctx context.Context, key string, fallback bool) bool
	GetStringOr(ctx context.Context, key, fallback string) string
}

// PermChecker resolves RBAC permission for a user (rbac resolver).
type PermChecker interface {
	Can(ctx context.Context, userID pgtype.UUID, node string) bool
}

// PaymentCreator creates §17 payment rows for fiat tips (purpose_type='donation').
type PaymentCreator interface {
	CreatePayment(ctx context.Context, userID pgtype.UUID, input PaymentCreateInput) (PaymentCreateResult, error)
}

// PaymentCreateInput mirrors payments.CreatePaymentInput.
type PaymentCreateInput struct {
	Provider    string
	PurposeType string
	PurposeID   string
	Amount      int64
	Currency    string
	Email       string // payer email — required by Paystack /transaction/initialize
	Metadata    map[string]interface{}
}

// PaymentCreateResult mirrors the payment fields the settle path needs.
type PaymentCreateResult struct {
	ID          string
	Provider    string
	ProviderRef string
	CheckoutURL string
	Status      string
	Amount      int64
	Currency    string
}

// OrgRoleChecker resolves an org membership role (organizations.MemberRole).
type OrgRoleChecker interface {
	MemberRole(ctx context.Context, orgID, userID pgtype.UUID) (string, error)
	OrganizationOwner(ctx context.Context, orgID pgtype.UUID) (pgtype.UUID, error)
}

// Notifier fans out tip.received notifications (tips:notify_received task).
type Notifier interface {
	NotifyTipReceived(ctx context.Context, tipID pgtype.UUID) error
}

// OCClient is the Open Collective GraphQL API boundary (internal/opencollective).
type OCClient interface {
	VerifyToken(ctx context.Context, token string) (string, error)
	ListContributions(ctx context.Context, slug, token string, limit, offset int) ([]OCContribution, error)
}

// OCContribution mirrors opencollective.Contribution without the import.
type OCContribution struct {
	ID          string
	AmountCents int64
	Currency    string
	FromName    string
	FromSlug    string
	FromEmail   string
}

// tipEnqueuer is the subset of worker.Enqueuer tips needs.
type tipEnqueuer interface {
	Enqueue(taskType string, payload []byte, opts ...interface{}) error
}

// Service coordinates tip settings, settlement, and history. Transactional
// boundaries (wallet ledger pairs) live inside *Store; tests fake tipStore.
type Service struct {
	store    tipStore
	settings SettingsReader
	perms    PermChecker
	payments PaymentCreator
	features FeatureChecker
	orgs     OrgRoleChecker
	oc       OCClient
	notifier Notifier
	enqueuer tipEnqueuer
	siteURL  string
}

// SetSettings injects the global system_settings reader.
func (s *Service) SetSettings(r SettingsReader) { s.settings = r }

// SetPermChecker injects the RBAC resolver adapter.
func (s *Service) SetPermChecker(p PermChecker) { s.perms = p }

// SetPaymentsService injects the §17 payment creator for fiat tips.
func (s *Service) SetPaymentsService(p PaymentCreator) { s.payments = p }

// SetOrgChecker injects the organizations role/owner seam.
func (s *Service) SetOrgChecker(o OrgRoleChecker) { s.orgs = o }

// SetOCClient injects the Open Collective API client.
func (s *Service) SetOCClient(c OCClient) { s.oc = c }

// SetNotifier injects the tip.received notification fan-out.
func (s *Service) SetNotifier(n Notifier) { s.notifier = n }

// SetEnqueuer injects the Asynq enqueuer.
func (s *Service) SetEnqueuer(e tipEnqueuer) { s.enqueuer = e }

// SetSiteURL sets the public site URL for building webhook URLs.
func (s *Service) SetSiteURL(u string) { s.siteURL = u }

// FeatureChecker reads the feature_flags table (features.Checker). The
// admin UI toggles tips.enabled/tips.opencollective there — NOT in
// system_settings — so this seam is required for the flag to work.
type FeatureChecker interface {
	IsEnabled(ctx context.Context, key string) bool
}

// SetFeatureChecker injects the feature-flag checker.
func (s *Service) SetFeatureChecker(f FeatureChecker) { s.features = f }

func (s *Service) featureEnabled(ctx context.Context) bool {
	if s.features != nil {
		return s.features.IsEnabled(ctx, "tips.enabled")
	}
	// Fallback keeps unit tests working without a features seam.
	return s.settings != nil && s.settings.GetBoolOr(ctx, "tips.enabled", false)
}

func (s *Service) ocEnabled(ctx context.Context) bool {
	if s.features != nil {
		return s.features.IsEnabled(ctx, "tips.opencollective")
	}
	return s.settings != nil && s.settings.GetBoolOr(ctx, "tips.opencollective", false)
}

func (s *Service) feePercent(ctx context.Context) int {
	if s.settings == nil {
		return 0
	}
	pct := s.settings.GetIntOr(ctx, "tips.fee_percent", 0)
	if pct < 0 {
		return 0
	}
	if pct > 100 {
		return 100
	}
	return pct
}
