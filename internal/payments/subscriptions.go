package payments

// Recurring Pro/Business subscription lifecycle (JEOS §17 —
// Recurring Subscription Lifecycle; spec
// docs/superpowers/specs/2026-09-24-recurring-pro-billing-design.md).
//
// Checkout is hosted: POST /subscriptions returns a provider checkout URL
// and creates NO local row. The subscriptions row appears only after a
// verified provider webhook confirms the first charge (webhook is the
// source of truth — §17 decided). Lifecycle events arrive on
// subscription:webhook_process (§11) and land in ProcessSubscriptionEvent.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db/gen"
	errpkg "github.com/jedafe/japa/internal/errors"
)

// Subscription plan codes — business_* join the existing pro_* constants.
const (
	PlanBusinessMonthly = "business_monthly"
	PlanBusinessYearly  = "business_yearly"
)

// Subscription statuses not already declared in payments.go.
const (
	SubscriptionStatusExpired = "expired"
)

// Sentinel errors surfaced to the subscription HTTP handlers.
var (
	ErrAlreadySubscribed = errors.New("already_subscribed")
	ErrPlanNotConfigured = errors.New("plan_not_configured")
	ErrProviderFailure   = errors.New("provider_error")
)

// SubscriptionCheckoutResult is the response to POST /subscriptions — a
// hosted-checkout URL. No subscription row exists until the provider's
// first-charge webhook confirms payment.
type SubscriptionCheckoutResult struct {
	CheckoutURL string `json:"checkout_url"`
}

// TierManager is the seam to the unified tier system (internal/tiers).
// Defined locally so payments does not import tiers.
type TierManager interface {
	SetUserTier(ctx context.Context, userID pgtype.UUID, tier string) error
	ResolveUserTier(ctx context.Context, userID pgtype.UUID) (string, error)
}

// SetTierManager injects the tier grant/revoke seam.
func (s *Service) SetTierManager(t TierManager) { s.tierManager = t }

// SetSiteURL injects the public base URL used to build provider
// success/cancel return URLs (JC_SITE_URL).
func (s *Service) SetSiteURL(u string) { s.siteURL = u }

// planTier maps a subscription plan code to the tier it grants.
func planTier(plan string) string {
	if strings.HasPrefix(plan, "business_") {
		return "business"
	}
	return "pro"
}

// isValidSubscriptionPlan mirrors the subscriptions_plan_check constraint.
func isValidSubscriptionPlan(plan string) bool {
	switch plan {
	case PlanProMonthly, PlanProYearly, "pro_annual",
		PlanBusinessMonthly, PlanBusinessYearly:
		return true
	}
	return false
}

// planBillingCycle splits a plan code into (plan_slug, cycle) for the
// subscription_plans lookup and provider settings key.
func planBillingCycle(plan string) (slug, cycle string, ok bool) {
	if strings.HasPrefix(plan, "pro_") {
		slug = "pro"
	} else if strings.HasPrefix(plan, "business_") {
		slug = "business"
	} else {
		return "", "", false
	}
	switch {
	case strings.HasSuffix(plan, "_monthly"):
		cycle = "monthly"
	case strings.HasSuffix(plan, "_yearly"), strings.HasSuffix(plan, "_annual"):
		cycle = "yearly"
	default:
		return "", "", false
	}
	return slug, cycle, true
}

// priceSettingsKey returns the system_settings key holding the provider-side
// price/plan code for a (provider, plan) pair (settings catalog, Payment).
func priceSettingsKey(provider, plan string) string {
	kind := "plan"
	if provider == ProviderStripe {
		kind = "price"
	}
	return fmt.Sprintf("payments.%s.%s.%s", provider, kind, plan)
}

// settingString unwraps a system_settings value — stored jsonb arrives as a
// JSON-encoded string ("\"PLN_x\""); bare tokens pass through unchanged.
func settingString(v string) string {
	v = strings.TrimSpace(v)
	var s string
	if err := json.Unmarshal([]byte(v), &s); err == nil {
		return strings.TrimSpace(s)
	}
	return v
}

// providerPriceCode resolves the Stripe Price ID / Paystack plan_code for a
// plan from settings, unwrapping the stored JSON string.
func (s *Service) providerPriceCode(ctx context.Context, provider, plan string) string {
	if s.settings == nil {
		return ""
	}
	v, err := s.settings.GetSetting(ctx, priceSettingsKey(provider, plan))
	if err != nil {
		return ""
	}
	return settingString(v)
}

// planPeriodBounds returns (start, end) for a plan cycle — used when a
// webhook payload lacks explicit period bounds. Bounds sync to provider-truth
// on the next lifecycle event.
func planPeriodBounds(plan string, now time.Time) (start, end time.Time) {
	_, cycle, _ := planBillingCycle(plan)
	if cycle == "yearly" {
		return now, now.AddDate(1, 0, 0)
	}
	return now, now.AddDate(0, 1, 0)
}

// ---------------------------------------------------------------------------
// Create — hosted checkout
// ---------------------------------------------------------------------------

// CreateSubscription starts a hosted recurring checkout (Stripe
// mode=subscription Checkout Session, or Paystack initialize-with-plan) and
// returns the redirect URL. Per §17, no subscriptions row is created here —
// the provider's first-charge webhook creates it (deferred creation).
func (s *Service) CreateSubscription(ctx context.Context, userID pgtype.UUID, input CreateSubscriptionInput) (SubscriptionCheckoutResult, error) {
	if input.Plan == "" {
		return SubscriptionCheckoutResult{}, fmt.Errorf("%w: plan", errpkg.ErrRequired)
	}
	if input.Provider == "" {
		return SubscriptionCheckoutResult{}, fmt.Errorf("%w: provider", errpkg.ErrRequired)
	}

	// JEOS §17 / ADR-012: recurring billing is Stripe or Paystack only.
	if input.Provider != ProviderStripe && input.Provider != ProviderPaystack {
		return SubscriptionCheckoutResult{}, fmt.Errorf("%w: provider %s cannot do recurring billing", errpkg.ErrInvalid, input.Provider)
	}
	if !isValidSubscriptionPlan(input.Plan) {
		return SubscriptionCheckoutResult{}, fmt.Errorf("%w: plan %s", errpkg.ErrInvalid, input.Plan)
	}

	// One active subscription per user — V1 upgrade path is cancel+resubscribe.
	if existing, err := s.store.getActiveSubscriptionByUser(ctx, userID); err == nil && existing.ID.Valid {
		return SubscriptionCheckoutResult{}, fmt.Errorf("%w: cancel the current subscription before starting a new one", ErrAlreadySubscribed)
	}

	// Resolve the provider-side price/plan code from settings — missing means
	// the plan isn't configured for this provider: fail loud, never grant.
	priceCode := s.providerPriceCode(ctx, input.Provider, input.Plan)
	if priceCode == "" {
		return SubscriptionCheckoutResult{}, fmt.Errorf("%w: %s", ErrPlanNotConfigured, priceSettingsKey(input.Provider, input.Plan))
	}

	// Amount comes from subscription_plans (Rule 3 — no hardcoded prices).
	slug, cycle, _ := planBillingCycle(input.Plan)
	var amount int64
	var currency string
	if planRow, err := s.store.getSubscriptionPlanBySlug(ctx, slug); err == nil {
		if cycle == "yearly" {
			amount = planRow.YearlyAmount
		} else {
			amount = planRow.MonthlyAmount
		}
		currency = planRow.Currency
	}
	if amount <= 0 {
		return SubscriptionCheckoutResult{}, fmt.Errorf("%w: subscription_plans row for %s", ErrPlanNotConfigured, slug)
	}
	if currency == "" && s.settings != nil {
		if v, err := s.settings.GetSetting(ctx, "payment.currency"); err == nil {
			currency = settingString(v)
		}
	}

	// Paystack requires the customer email on initialize.
	var email string
	if em, err := s.store.getUserEmail(ctx, userID); err == nil {
		email = em
	}

	subProvider, ok := s.providers[input.Provider].(SubscriptionProvider)
	if !ok {
		return SubscriptionCheckoutResult{}, fmt.Errorf("%w: %s is not a subscription provider", ErrProviderFailure, input.Provider)
	}

	returnURL := strings.TrimRight(s.siteURL, "/") + "/settings/subscription"
	resp, err := subProvider.CreateSubscriptionCheckout(ctx, SubscriptionRequest{
		UserID:    userID.String(),
		Email:     email,
		Plan:      input.Plan,
		Amount:    amount,
		Currency:  currency,
		PriceCode: priceCode,
		ReturnURL: returnURL,
		Metadata:  map[string]interface{}{"user_id": userID.String(), "plan": input.Plan},
	})
	if err != nil {
		return SubscriptionCheckoutResult{}, fmt.Errorf("%w: %v", ErrProviderFailure, err)
	}
	if resp.CheckoutURL == "" {
		return SubscriptionCheckoutResult{}, fmt.Errorf("%w: empty checkout URL", ErrProviderFailure)
	}

	return SubscriptionCheckoutResult{CheckoutURL: resp.CheckoutURL}, nil
}

// ---------------------------------------------------------------------------
// Lifecycle events (worker: subscription:webhook_process)
// ---------------------------------------------------------------------------

// ProcessSubscriptionEvent applies one provider subscription lifecycle event.
// Called from the subscription:webhook_process worker task after the HTTP
// handler has verified the signature and recorded the dedupe row.
func (s *Service) ProcessSubscriptionEvent(ctx context.Context, provider, eventType, eventID string, raw []byte) error {
	var payload map[string]interface{}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return fmt.Errorf("subscription webhook: bad payload: %w", err)
	}
	event := WebhookEvent{Provider: provider, EventID: eventID, EventType: eventType, Payload: payload, RawPayload: raw}
	if provider == ProviderStripe {
		return s.processStripeSubscriptionEvent(ctx, event)
	}
	if provider == ProviderPaystack {
		return s.processPaystackSubscriptionEvent(ctx, event)
	}
	return fmt.Errorf("unsupported subscription provider: %s", provider)
}

// --- Stripe ---

func (s *Service) processStripeSubscriptionEvent(ctx context.Context, event WebhookEvent) error {
	data, _ := event.Payload["data"].(map[string]interface{})
	obj, _ := data["object"].(map[string]interface{})
	if obj == nil {
		slog.Warn("subscription webhook: stripe event missing data.object", "event_type", event.EventType)
		return nil
	}

	switch event.EventType {
	case "checkout.session.completed", "customer.subscription.created":
		return s.stripeActivate(ctx, obj)
	case "invoice.paid":
		return s.stripeInvoicePaid(ctx, obj)
	case "invoice.payment_failed":
		return s.markSubPastDueByRef(ctx, str(obj, "subscription"))
	case "customer.subscription.updated":
		return s.stripeSyncSubscription(ctx, obj)
	case "customer.subscription.deleted":
		return s.stripeCancelNow(ctx, obj)
	default:
		return nil
	}
}

// stripeActivate creates the local row from checkout.session.completed or
// customer.subscription.created — whichever arrives first wins; the
// provider_sub_id UNIQUE constraint makes the pair idempotent.
func (s *Service) stripeActivate(ctx context.Context, obj map[string]interface{}) error {
	// checkout.session.completed nests the subscription id on `subscription`
	// and user/plan on `metadata`; customer.subscription.created IS the
	// subscription object (id + metadata + period bounds).
	subID := str(obj, "subscription")
	if subID == "" {
		subID = str(obj, "id") // subscription object itself
	}
	if subID == "" || !strings.HasPrefix(subID, "sub_") {
		return nil // payment-mode session or malformed event — not ours
	}
	meta, _ := obj["metadata"].(map[string]interface{})
	userID, plan := str(meta, "user_id"), str(meta, "plan")
	if userID == "" || plan == "" {
		// checkout.session.completed can also carry it under
		// subscription_data.metadata (what CreateSubscriptionCheckout sets).
		if sd, ok := obj["subscription_data"].(map[string]interface{}); ok {
			sm, _ := sd["metadata"].(map[string]interface{})
			if userID == "" {
				userID = str(sm, "user_id")
			}
			if plan == "" {
				plan = str(sm, "plan")
			}
		}
	}
	if userID == "" || plan == "" {
		slog.Warn("subscription webhook: stripe activation missing metadata", "sub", subID)
		return nil
	}
	var uid pgtype.UUID
	if err := uid.Scan(userID); err != nil {
		return fmt.Errorf("subscription webhook: bad user_id %q: %w", userID, err)
	}

	now := time.Now()
	start, end := planPeriodBounds(plan, now)
	// Prefer provider-truth bounds when present (subscription object).
	if ps := unixTs(obj, "current_period_start"); !ps.IsZero() {
		start = ps
	}
	if pe := unixTs(obj, "current_period_end"); !pe.IsZero() {
		end = pe
	}

	status := SubscriptionStatusActive
	if st := str(obj, "status"); st == "trialing" {
		status = SubscriptionStatusTrialing
	}

	sub, err := s.upsertSubscriptionByRef(ctx, uid, plan, ProviderStripe, subID, status, start, end, boolFrom(obj, "cancel_at_period_end"))
	if err != nil {
		return err
	}
	return s.grantPlanTier(ctx, sub.UserID, sub.Plan)
}

// stripeInvoicePaid extends the period on a subscription_cycle invoice and
// writes a renewal payments row for accounting + affiliate consistency.
func (s *Service) stripeInvoicePaid(ctx context.Context, obj map[string]interface{}) error {
	subID := str(obj, "subscription")
	if subID == "" {
		return nil // one-off invoice — handled by payment_webhook_process
	}
	sub, err := s.store.getSubscriptionByProviderSubID(ctx, subID)
	if err != nil {
		slog.Warn("subscription webhook: invoice.paid for unknown sub", "sub", subID)
		return nil
	}
	// Renewal period bounds: invoice line item period → fallback to plan cycle.
	now := time.Now()
	_, end := planPeriodBounds(sub.Plan, now)
	linesMap, _ := obj["lines"].(map[string]interface{})
	if lines, ok := linesMap["data"].([]interface{}); ok && len(lines) > 0 {
		if line, ok := lines[0].(map[string]interface{}); ok {
			if period, ok := line["period"].(map[string]interface{}); ok {
				if pe := unixTs(period, "end"); !pe.IsZero() {
					end = pe
				}
			}
		}
	}
	if _, err := s.store.renewSubscriptionPeriod(ctx, sub.ID, now, end); err != nil {
		return err
	}
	s.recordRenewalPayment(ctx, sub, str(obj, "id"), int64From(obj, "amount_paid"), str(obj, "currency"))
	return s.grantPlanTier(ctx, sub.UserID, sub.Plan)
}

// stripeSyncSubscription mirrors customer.subscription.updated — bounds and
// the pending-cancel flag; status changes arrive via their own events.
func (s *Service) stripeSyncSubscription(ctx context.Context, obj map[string]interface{}) error {
	subID := str(obj, "id")
	sub, err := s.store.getSubscriptionByProviderSubID(ctx, subID)
	if err != nil {
		return nil // unknown sub — nothing to sync
	}
	start, end := unixTs(obj, "current_period_start"), unixTs(obj, "current_period_end")
	return s.store.updateSubscriptionPeriodBounds(ctx, sub.ID, start, end, boolFrom(obj, "cancel_at_period_end"))
}

// stripeCancelNow handles customer.subscription.deleted — the provider has
// ended the subscription (period reached after pending cancel, or immediate
// cancel). Local row → canceled, tier downgraded if we granted it.
func (s *Service) stripeCancelNow(ctx context.Context, obj map[string]interface{}) error {
	subID := str(obj, "id")
	sub, err := s.store.getSubscriptionByProviderSubID(ctx, subID)
	if err != nil {
		return nil
	}
	if _, err := s.store.updateSubscriptionStatus(ctx, sub.ID, SubscriptionStatusCanceled); err != nil {
		return err
	}
	return s.revokePlanTier(ctx, sub.UserID, sub.Plan)
}

// --- Paystack ---

func (s *Service) processPaystackSubscriptionEvent(ctx context.Context, event WebhookEvent) error {
	data, _ := event.Payload["data"].(map[string]interface{})
	if data == nil {
		return nil
	}

	switch event.EventType {
	case "subscription.create":
		return s.paystackActivate(ctx, data)
	case "subscription.not_renew":
		return s.setSubCancelFlagByRef(ctx, str(data, "subscription_code"), true)
	case "subscription.disable":
		return s.paystackDisabled(ctx, data)
	case "invoice.payment_failed", "invoice.create":
		// invoice.create fires pre-charge; only payment_failed moves status.
		if event.EventType == "invoice.payment_failed" {
			return s.markSubPastDueByRef(ctx, str(data, "subscription_code"))
		}
		return nil
	case "charge.success":
		// A charge carrying a subscription code is a recurring renewal.
		if code := str(data, "subscription_code"); code != "" {
			return s.paystackRenewal(ctx, code, data)
		}
		if sub, ok := data["subscription"].(map[string]interface{}); ok {
			return s.paystackRenewal(ctx, str(sub, "subscription_code"), data)
		}
		return nil // one-time charge — payment_webhook_process owns it
	default:
		return nil
	}
}

// paystackActivate handles subscription.create. The payload carries the
// customer email and plan code — user_id resolves via email, plan via the
// reverse settings lookup (Paystack does not echo initialize metadata).
func (s *Service) paystackActivate(ctx context.Context, data map[string]interface{}) error {
	subCode := str(data, "subscription_code")
	if subCode == "" {
		return nil
	}
	customer, _ := data["customer"].(map[string]interface{})
	email := str(customer, "email")
	planObj, _ := data["plan"].(map[string]interface{})
	planCode := str(planObj, "plan_code")
	if email == "" || planCode == "" {
		slog.Warn("subscription webhook: paystack subscription.create missing email/plan", "sub", subCode)
		return nil
	}

	uid, err := s.store.getUserIDByEmail(ctx, strings.ToLower(email))
	if err != nil {
		slog.Warn("subscription webhook: paystack subscription for unknown email", "sub", subCode)
		return nil
	}
	plan := s.planFromPaystackCode(ctx, planCode)
	if plan == "" {
		slog.Warn("subscription webhook: paystack plan_code not in settings", "plan_code", planCode)
		return nil
	}

	now := time.Now()
	start, end := planPeriodBounds(plan, now)
	if npd := str(data, "next_payment_date"); npd != "" {
		if t, err := time.Parse(time.RFC3339, npd); err == nil {
			end = t
		}
	}

	sub, err := s.upsertSubscriptionByRef(ctx, uid, plan, ProviderPaystack, subCode, SubscriptionStatusActive, start, end, false)
	if err != nil {
		return err
	}
	return s.grantPlanTier(ctx, sub.UserID, sub.Plan)
}

// paystackRenewal extends the period on a subscription charge.success.
func (s *Service) paystackRenewal(ctx context.Context, subCode string, data map[string]interface{}) error {
	if subCode == "" {
		return nil
	}
	sub, err := s.store.getSubscriptionByProviderSubID(ctx, subCode)
	if err != nil {
		return nil // may be the first charge before subscription.create landed
	}
	now := time.Now()
	_, end := planPeriodBounds(sub.Plan, now)
	if npd := str(data, "next_payment_date"); npd != "" {
		if t, err := time.Parse(time.RFC3339, npd); err == nil {
			end = t
		}
	}
	if _, err := s.store.renewSubscriptionPeriod(ctx, sub.ID, now, end); err != nil {
		return err
	}
	s.recordRenewalPayment(ctx, sub, str(data, "reference"), int64From(data, "amount"), str(data, "currency"))
	return s.grantPlanTier(ctx, sub.UserID, sub.Plan)
}

// paystackDisabled handles subscription.disable — Paystack stops renewals
// immediately; entitlement ends at current_period_end via the expiry sweep.
func (s *Service) paystackDisabled(ctx context.Context, data map[string]interface{}) error {
	return s.setSubCancelFlagByRef(ctx, str(data, "subscription_code"), true)
}

// planFromPaystackCode reverse-maps a Paystack plan_code to a local plan by
// scanning the configured payments.paystack.plan.* settings keys.
func (s *Service) planFromPaystackCode(ctx context.Context, planCode string) string {
	if s.settings == nil {
		return ""
	}
	for _, plan := range []string{PlanProMonthly, PlanProYearly, "pro_annual", PlanBusinessMonthly, PlanBusinessYearly} {
		if s.providerPriceCode(ctx, ProviderPaystack, plan) == planCode {
			return plan
		}
	}
	return ""
}

// ---------------------------------------------------------------------------
// Shared lifecycle helpers
// ---------------------------------------------------------------------------

// upsertSubscriptionByRef creates the row on first sight of a provider
// subscription, or refreshes bounds if a later lifecycle event already
// created it (events can arrive out of order).
func (s *Service) upsertSubscriptionByRef(ctx context.Context, userID pgtype.UUID, plan, provider, subID, status string, start, end time.Time, cancelAtEnd bool) (gen.Subscription, error) {
	if existing, err := s.store.getSubscriptionByProviderSubID(ctx, subID); err == nil {
		_ = s.store.updateSubscriptionPeriodBounds(ctx, existing.ID, start, end, cancelAtEnd)
		return s.store.updateSubscriptionStatus(ctx, existing.ID, status)
	}
	return s.store.createSubscription(ctx, gen.CreateSubscriptionParams{
		UserID:             userID,
		Plan:               plan,
		Provider:           provider,
		ProviderSubID:      &subID,
		Status:             status,
		CurrentPeriodStart: pgtype.Timestamptz{Time: start, Valid: true},
		CurrentPeriodEnd:   pgtype.Timestamptz{Time: end, Valid: true},
	})
}

// recordRenewalPayment writes a payments row for a subscription renewal so
// accounting and affiliate commission see every charge (§17).
func (s *Service) recordRenewalPayment(ctx context.Context, sub gen.Subscription, providerRef string, amount int64, currency string) {
	if providerRef == "" || amount <= 0 {
		return
	}
	if currency == "" {
		currency = "NGN"
	}
	subID := sub.ID
	if _, err := s.store.createPayment(ctx, gen.CreatePaymentParams{
		UserID:         sub.UserID,
		Provider:       sub.Provider,
		ProviderRef:    &providerRef,
		PurposeType:    PurposeProMembership,
		PurposeID:      subID,
		Amount:         amount,
		Currency:       strings.ToUpper(currency),
		Status:         PaymentStatusSucceeded,
		Metadata:       []byte(`{"source":"subscription_renewal"}`),
		SubscriptionID: subID,
	}); err != nil {
		slog.Warn("subscription webhook: renewal payment row failed", "sub", providerRef, "error", err)
	}
}

// grantPlanTier assigns the plan's tier to the subscriber.
func (s *Service) grantPlanTier(ctx context.Context, userID pgtype.UUID, plan string) error {
	if s.tierManager == nil {
		slog.Warn("subscription: no tier manager — tier not granted", "plan", plan)
		return nil
	}
	if err := s.tierManager.SetUserTier(ctx, userID, planTier(plan)); err != nil {
		return fmt.Errorf("granting %s tier: %w", planTier(plan), err)
	}
	return nil
}

// revokePlanTier downgrades to regular — but only if the user's current tier
// still equals the tier this plan granted. An admin-granted business tier is
// never clobbered by an expiring pro subscription (spec §3).
func (s *Service) revokePlanTier(ctx context.Context, userID pgtype.UUID, plan string) error {
	if s.tierManager == nil {
		return nil
	}
	current, err := s.tierManager.ResolveUserTier(ctx, userID)
	if err != nil {
		return fmt.Errorf("resolving tier before revoke: %w", err)
	}
	if current != planTier(plan) {
		return nil // user holds a different (e.g. admin-granted) tier — leave it
	}
	if err := s.tierManager.SetUserTier(ctx, userID, "regular"); err != nil {
		return fmt.Errorf("revoking tier: %w", err)
	}
	return nil
}

func (s *Service) markSubPastDueByRef(ctx context.Context, subID string) error {
	if subID == "" {
		return nil
	}
	sub, err := s.store.getSubscriptionByProviderSubID(ctx, subID)
	if err != nil {
		return nil
	}
	_, err = s.store.markSubscriptionPastDue(ctx, sub.ID)
	return err
}

func (s *Service) setSubCancelFlagByRef(ctx context.Context, subID string, flag bool) error {
	if subID == "" {
		return nil
	}
	sub, err := s.store.getSubscriptionByProviderSubID(ctx, subID)
	if err != nil {
		return nil
	}
	return s.store.setSubscriptionCancelAtPeriodEnd(ctx, sub.ID, flag)
}

// ---------------------------------------------------------------------------
// Cancel / reactivate — provider first, then local (§17)
// ---------------------------------------------------------------------------

// CancelSubscription schedules cancellation at the provider, then marks the
// local row. Provider failure aborts before any local change.
func (s *Service) CancelSubscription(ctx context.Context, id pgtype.UUID) (gen.Subscription, error) {
	sub, err := s.store.getSubscriptionByID(ctx, id)
	if err != nil {
		return gen.Subscription{}, err
	}
	if sp, ok := s.providers[sub.Provider].(SubscriptionProvider); ok && sub.ProviderSubID != nil {
		if err := sp.CancelSubscription(ctx, *sub.ProviderSubID); err != nil {
			return gen.Subscription{}, fmt.Errorf("%w: %v", ErrProviderFailure, err)
		}
	}
	return s.store.cancelSubscription(ctx, id)
}

// ReactivateSubscription undoes a pending cancel at the provider, then
// reactivates the caller's local row.
func (s *Service) ReactivateSubscription(ctx context.Context, userID pgtype.UUID) (gen.Subscription, error) {
	// Resolve the row that reactivateSubscription will target — the most
	// recently updated canceled / pending-cancel row for this user.
	subs, err := s.store.listSubscriptionsByUser(ctx, userID, 10, 0)
	if err != nil {
		return gen.Subscription{}, err
	}
	var target *gen.Subscription
	for i := range subs {
		s := subs[i]
		if s.Status == SubscriptionStatusCanceled || (s.Status == SubscriptionStatusActive && s.CancelAtPeriodEnd) {
			target = &s
			break
		}
	}
	if target == nil {
		return gen.Subscription{}, errpkg.ErrNotFound
	}
	if sp, ok := s.providers[target.Provider].(SubscriptionProvider); ok && target.ProviderSubID != nil {
		if err := sp.ReactivateSubscription(ctx, *target.ProviderSubID); err != nil {
			return gen.Subscription{}, fmt.Errorf("%w: %v", ErrProviderFailure, err)
		}
	}
	return s.store.reactivateSubscription(ctx, userID)
}

// ---------------------------------------------------------------------------
// Expiry sweep — subscription:expiry_sweep (webhook-miss safety net)
// ---------------------------------------------------------------------------

// RunExpirySweep expires subscriptions whose entitlement window has ended:
// active+cancel_at_period_end rows past current_period_end, and past_due rows
// past current_period_end + the configured grace window. Each expiry revokes
// the granted tier (with the same-tier safety check).
func (s *Service) RunExpirySweep(ctx context.Context) (int, error) {
	graceHours := int64(72)
	sweepLimit := int32(100)
	if s.settings != nil {
		if v, err := s.settings.GetSetting(ctx, "subscription.past_due_grace_hours"); err == nil && v != "" {
			if n, err := strconv.ParseInt(settingString(v), 10, 64); err == nil && n > 0 {
				graceHours = n
			}
		}
	}
	cutoff := time.Now().Add(-time.Duration(graceHours) * time.Hour)

	rows, err := s.store.listSubscriptionsDueForExpiry(ctx, cutoff, sweepLimit)
	if err != nil {
		return 0, err
	}
	for _, sub := range rows {
		if _, err := s.store.expireSubscription(ctx, sub.ID); err != nil {
			slog.Error("subscription sweep: expire failed", "id", sub.ID, "error", err)
			continue
		}
		if err := s.revokePlanTier(ctx, sub.UserID, sub.Plan); err != nil {
			slog.Error("subscription sweep: tier revoke failed", "id", sub.ID, "error", err)
		}
		s.emitWebhook(ctx, "subscription.expired", map[string]any{
			"subscription_id": sub.ID.String(),
			"user_id":         sub.UserID.String(),
			"plan":            sub.Plan,
		})
	}
	return len(rows), nil
}

// ---------------------------------------------------------------------------
// Payload field helpers — webhook payloads are loosely typed maps.
// ---------------------------------------------------------------------------

func str(m map[string]interface{}, key string) string {
	if m == nil {
		return ""
	}
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}

func boolFrom(m map[string]interface{}, key string) bool {
	v, _ := m[key].(bool)
	return v
}

func unixTs(m map[string]interface{}, key string) time.Time {
	switch v := m[key].(type) {
	case float64:
		return time.Unix(int64(v), 0)
	case string:
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			return time.Unix(n, 0)
		}
	}
	return time.Time{}
}

func int64From(m map[string]interface{}, key string) int64 {
	switch v := m[key].(type) {
	case float64:
		return int64(v)
	case string:
		n, _ := strconv.ParseInt(v, 10, 64)
		return n
	}
	return 0
}

// isSubscriptionLifecycleEvent reports whether a verified provider event
// belongs on subscription:webhook_process rather than payment:process_webhook.
// Ambiguous event types are disambiguated on the payload: Stripe's
// checkout.session.completed is ours only in subscription mode, and
// Paystack's charge.success is a renewal only when it carries a
// subscription reference.
func isSubscriptionLifecycleEvent(provider, eventType string, payload map[string]interface{}) bool {
	data, _ := payload["data"].(map[string]interface{})
	obj, _ := data["object"].(map[string]interface{})
	if obj == nil {
		obj = data // Paystack nests fields directly under data
	}

	switch provider {
	case ProviderStripe:
		switch eventType {
		case "customer.subscription.created", "customer.subscription.updated",
			"customer.subscription.deleted", "invoice.payment_failed":
			return true
		case "invoice.paid":
			return str(obj, "subscription") != ""
		case "checkout.session.completed":
			return str(obj, "mode") == "subscription"
		}
	case ProviderPaystack:
		switch eventType {
		case "subscription.create", "subscription.not_renew", "subscription.disable",
			"invoice.payment_failed":
			return true
		case "charge.success":
			return str(obj, "subscription_code") != "" || obj["subscription"] != nil
		}
	}
	return false
}
