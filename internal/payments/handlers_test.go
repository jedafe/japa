package payments

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
)

// stubProvider is a no-op Provider for registration-matrix tests — only
// Name() matters; the other methods are unreachable in these tests.
type stubProvider struct{ name string }

func (p *stubProvider) Name() string { return p.name }

func (p *stubProvider) InitiatePayment(context.Context, PaymentRequest) (PaymentResponse, error) {
	return PaymentResponse{}, nil
}

func (p *stubProvider) VerifyWebhookSignature([]byte, string) error { return nil }

func (p *stubProvider) ProcessWebhookEvent(context.Context, WebhookEvent) error { return nil }

func (p *stubProvider) RefundPayment(context.Context, string, int64) (string, error) {
	return "", ErrRefundNotSupported
}

// GET /payment-providers must mirror boot-time registration — the same
// map CreatePayment dispatches on — never per-field credential presence.
// Regression: the Enabled flags previously re-derived availability from
// handler credential fields and drifted (NOWPayments checked the IPN key
// alone; Stripe ignored the webhook secret), advertising providers that
// would fail InitiatePayment at checkout.
func TestListPaymentProvidersMirrorsRegistration(t *testing.T) {
	t.Parallel()

	svc := &Service{}
	svc.SetProvider(&stubProvider{name: ProviderPaystack})
	// Stripe + NOWPayments deliberately NOT registered — while the
	// handler's credential fields below say they are configured. This is
	// the drift case: registration is the truth.

	h := NewHandlers(svc)
	h.StripeSecretKey = "sk_live_set"
	h.StripeWebhookSecret = "" // half-configured — cannot receive webhooks
	h.NOWPaymentsIPNKey = "ipn_set"
	h.PaystackSecretKey = "" // credential field unset yet provider usable (tests)

	rec := httptest.NewRecorder()
	h.ListPaymentProviders(rec, httptest.NewRequest(http.MethodGet, "/api/v1/payment-providers", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}

	var body struct {
		Data []PaymentProviderInfo `json:"data"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}

	enabled := map[string]bool{}
	for _, p := range body.Data {
		enabled[p.ID] = p.Enabled
	}

	for _, want := range []struct {
		id      string
		enabled bool
	}{
		{ProviderPaystack, true},
		{ProviderStripe, false},
		{ProviderNOWPayments, false},
		{ProviderCredit, true},
	} {
		got, ok := enabled[want.id]
		if !ok {
			t.Fatalf("provider %q missing from response", want.id)
		}
		if got != want.enabled {
			t.Errorf("provider %q enabled = %v, want %v", want.id, got, want.enabled)
		}
	}
}

// CreatePayment on a valid-but-unregistered provider returns the
// ErrProviderNotConfigured sentinel — handlers map it to 503
// provider_not_configured, never a bare 500.
func TestCreatePaymentUnregisteredProviderSentinel(t *testing.T) {
	t.Parallel()

	svc := &Service{}

	_, err := svc.CreatePayment(t.Context(), pgtype.UUID{}, CreatePaymentInput{
		Provider:    ProviderStripe, // valid enum, not registered
		PurposeType: PurposeDonation,
		Amount:      500,
		Currency:    "NGN",
	})
	if !errors.Is(err, ErrProviderNotConfigured) {
		t.Fatalf("err = %v, want ErrProviderNotConfigured", err)
	}
}
