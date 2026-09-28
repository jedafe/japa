package notifications

import (
	"context"
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/SherClockHolmes/webpush-go"
	errpkg "github.com/jedafe/japa/internal/errors"
)

// newEnabledPushDelivery builds a PushDelivery with freshly generated VAPID
// keys so Send actually performs an HTTP request.
func newEnabledPushDelivery(t *testing.T) *PushDelivery {
	t.Helper()

	priv, pub, err := webpush.GenerateVAPIDKeys()
	if err != nil {
		t.Fatalf("generate VAPID keys: %v", err)
	}

	p := NewPushDeliveryFromConfig(PushConfig{PrivateKey: priv, PublicKey: pub})
	if !p.IsEnabled() {
		t.Fatal("expected push delivery to be enabled with valid VAPID keys")
	}

	return p
}

// newSubscriptionKeys returns a valid p256dh/auth pair for a subscription.
func newSubscriptionKeys(t *testing.T) (p256dh, auth string) {
	t.Helper()

	key, err := ecdh.P256().GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate subscription key: %v", err)
	}

	authBytes := make([]byte, 16)
	if _, err := rand.Read(authBytes); err != nil {
		t.Fatalf("generate auth secret: %v", err)
	}

	return base64.RawURLEncoding.EncodeToString(key.PublicKey().Bytes()),
		base64.RawURLEncoding.EncodeToString(authBytes)
}

// A push service responding 410 Gone must produce an error that wraps
// errpkg.ErrExpired — worker.HandlePushDelivery prunes the subscription only
// when errors.Is(err, ErrExpired) is true.
func TestSend410WrapsErrExpired(t *testing.T) {
	p := newEnabledPushDelivery(t)
	p256dh, auth := newSubscriptionKeys(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusGone)
	}))
	defer srv.Close()

	err := p.Send(context.Background(), srv.URL, p256dh, auth, []byte(`{"title":"t"}`))
	if err == nil {
		t.Fatal("expected error for 410 response")
	}
	if !errors.Is(err, errpkg.ErrExpired) {
		t.Fatalf("expected error to wrap ErrExpired, got: %v", err)
	}
}

// Non-410 push failures (e.g. 500) are transient and must NOT be treated as
// expired subscriptions.
func TestSend500DoesNotWrapErrExpired(t *testing.T) {
	p := newEnabledPushDelivery(t)
	p256dh, auth := newSubscriptionKeys(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	err := p.Send(context.Background(), srv.URL, p256dh, auth, []byte(`{"title":"t"}`))
	if err == nil {
		t.Fatal("expected error for 500 response")
	}
	if errors.Is(err, errpkg.ErrExpired) {
		t.Fatalf("500 must not be treated as expired subscription, got: %v", err)
	}
}

// Disabled push delivery is a graceful no-op, not an error.
func TestSendDisabledIsNoop(t *testing.T) {
	p := NewPushDeliveryFromConfig(PushConfig{})
	if p.IsEnabled() {
		t.Fatal("expected push delivery to be disabled without keys")
	}
	if err := p.Send(context.Background(), "https://example.invalid", "k", "a", nil); err != nil {
		t.Fatalf("disabled send should be nil, got: %v", err)
	}
}
