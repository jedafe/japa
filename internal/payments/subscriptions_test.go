package payments

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
)

// ---------------------------------------------------------------------------
// Plan mapping
// ---------------------------------------------------------------------------

func TestPlanBillingCycle(t *testing.T) {
	cases := []struct {
		plan     string
		wantSlug string
		wantCycle string
		wantOK   bool
	}{
		{"pro_monthly", "pro", "monthly", true},
		{"pro_yearly", "pro", "yearly", true},
		{"pro_annual", "pro", "yearly", true},
		{"business_monthly", "business", "monthly", true},
		{"business_yearly", "business", "yearly", true},
		{"free", "", "", false},
		{"pro", "", "", false},
		{"weekly_pro", "", "", false},
		{"", "", "", false},
	}
	for _, c := range cases {
		slug, cycle, ok := planBillingCycle(c.plan)
		if ok != c.wantOK || slug != c.wantSlug || cycle != c.wantCycle {
			t.Errorf("planBillingCycle(%q) = (%q,%q,%v), want (%q,%q,%v)",
				c.plan, slug, cycle, ok, c.wantSlug, c.wantCycle, c.wantOK)
		}
	}
}

func TestPlanTier(t *testing.T) {
	if got := planTier("pro_monthly"); got != "pro" {
		t.Errorf("planTier(pro_monthly) = %q", got)
	}
	if got := planTier("business_yearly"); got != "business" {
		t.Errorf("planTier(business_yearly) = %q", got)
	}
	if got := planTier("pro_annual"); got != "pro" {
		t.Errorf("planTier(pro_annual) = %q", got)
	}
}

func TestIsValidSubscriptionPlan(t *testing.T) {
	for _, p := range []string{"pro_monthly", "pro_yearly", "pro_annual", "business_monthly", "business_yearly"} {
		if !isValidSubscriptionPlan(p) {
			t.Errorf("expected %q to be valid", p)
		}
	}
	for _, p := range []string{"", "free", "pro_weekly", "enterprise"} {
		if isValidSubscriptionPlan(p) {
			t.Errorf("expected %q to be invalid", p)
		}
	}
}

func TestPriceSettingsKey(t *testing.T) {
	if got := priceSettingsKey(ProviderStripe, "pro_monthly"); got != "payments.stripe.price.pro_monthly" {
		t.Errorf("unexpected stripe key %q", got)
	}
	if got := priceSettingsKey(ProviderPaystack, "business_yearly"); got != "payments.paystack.plan.business_yearly" {
		t.Errorf("unexpected paystack key %q", got)
	}
}

// ---------------------------------------------------------------------------
// Event routing — the payment/subscription split at ProcessWebhook
// ---------------------------------------------------------------------------

func TestIsSubscriptionLifecycleEvent(t *testing.T) {
	stripeSub := map[string]interface{}{
		"data": map[string]interface{}{
			"object": map[string]interface{}{"mode": "subscription"},
		},
	}
	stripePayment := map[string]interface{}{
		"data": map[string]interface{}{
			"object": map[string]interface{}{"mode": "payment"},
		},
	}
	stripeInvoiceWithSub := map[string]interface{}{
		"data": map[string]interface{}{
			"object": map[string]interface{}{"subscription": "sub_123"},
		},
	}
	stripeInvoiceNoSub := map[string]interface{}{
		"data": map[string]interface{}{
			"object": map[string]interface{}{},
		},
	}
	paystackSubCharge := map[string]interface{}{
		"data": map[string]interface{}{"subscription_code": "SUB_abc"},
	}
	paystackOneTime := map[string]interface{}{
		"data": map[string]interface{}{"reference": "ref_1"},
	}

	cases := []struct {
		name      string
		provider  string
		eventType string
		payload   map[string]interface{}
		want      bool
	}{
		{"stripe sub-mode checkout", ProviderStripe, "checkout.session.completed", stripeSub, true},
		{"stripe payment-mode checkout", ProviderStripe, "checkout.session.completed", stripePayment, false},
		{"stripe subscription.created", ProviderStripe, "customer.subscription.created", nil, true},
		{"stripe subscription.deleted", ProviderStripe, "customer.subscription.deleted", nil, true},
		{"stripe invoice.paid w/ sub", ProviderStripe, "invoice.paid", stripeInvoiceWithSub, true},
		{"stripe invoice.paid no sub", ProviderStripe, "invoice.paid", stripeInvoiceNoSub, false},
		{"stripe payment_intent ok", ProviderStripe, "payment_intent.succeeded", stripePayment, false},
		{"paystack subscription.create", ProviderPaystack, "subscription.create", nil, true},
		{"paystack subscription.disable", ProviderPaystack, "subscription.disable", nil, true},
		{"paystack charge.success w/ sub", ProviderPaystack, "charge.success", paystackSubCharge, true},
		{"paystack charge.success one-time", ProviderPaystack, "charge.success", paystackOneTime, false},
		{"paystack invoice.payment_failed", ProviderPaystack, "invoice.payment_failed", nil, true},
		{"nowpayments never subscription", ProviderNOWPayments, "finished", nil, false},
	}
	for _, c := range cases {
		if got := isSubscriptionLifecycleEvent(c.provider, c.eventType, c.payload); got != c.want {
			t.Errorf("%s: isSubscriptionLifecycleEvent = %v, want %v", c.name, got, c.want)
		}
	}
}

// ---------------------------------------------------------------------------
// Tier revoke safety — never clobber a different (e.g. admin-granted) tier
// ---------------------------------------------------------------------------

type fakeTierManager struct {
	tier    string
	setTo   []string
	failErr error
}

func (f *fakeTierManager) SetUserTier(_ context.Context, _ pgtype.UUID, tier string) error {
	if f.failErr != nil {
		return f.failErr
	}
	f.tier = tier
	f.setTo = append(f.setTo, tier)
	return nil
}

func (f *fakeTierManager) ResolveUserTier(_ context.Context, _ pgtype.UUID) (string, error) {
	return f.tier, nil
}

func TestRevokePlanTier_OnlyRevokesGrantedTier(t *testing.T) {
	var uid pgtype.UUID
	uid.Scan("11111111-1111-1111-1111-111111111111")

	// User still holds the tier the plan granted → revoked to regular.
	tm := &fakeTierManager{tier: "pro"}
	s := &Service{tierManager: tm}
	if err := s.revokePlanTier(context.Background(), uid, "pro_monthly"); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	if tm.tier != "regular" {
		t.Errorf("expected downgrade to regular, tier=%q", tm.tier)
	}

	// User holds a different tier (e.g. admin-granted business) → untouched.
	tm2 := &fakeTierManager{tier: "business"}
	s2 := &Service{tierManager: tm2}
	if err := s2.revokePlanTier(context.Background(), uid, "pro_yearly"); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	if len(tm2.setTo) != 0 {
		t.Errorf("admin-granted tier was clobbered: %v", tm2.setTo)
	}
}

// ---------------------------------------------------------------------------
// Payload field helpers
// ---------------------------------------------------------------------------

func TestPayloadHelpers(t *testing.T) {
	m := map[string]interface{}{
		"id":                     "sub_1",
		"cancel_at_period_end":   true,
		"current_period_end":     float64(1800000000),
		"amount_paid":            float64(500000),
		"amount_str":             "2500",
	}
	if str(m, "id") != "sub_1" {
		t.Error("str failed")
	}
	if !boolFrom(m, "cancel_at_period_end") {
		t.Error("boolFrom failed")
	}
	if unixTs(m, "current_period_end").Unix() != 1800000000 {
		t.Error("unixTs failed")
	}
	if int64From(m, "amount_paid") != 500000 || int64From(m, "amount_str") != 2500 {
		t.Error("int64From failed")
	}
	if str(nil, "x") != "" || boolFrom(nil, "x") || !unixTs(nil, "x").IsZero() {
		t.Error("nil-map helpers must be safe")
	}
}
