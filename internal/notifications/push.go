package notifications

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"

	"github.com/SherClockHolmes/webpush-go"
)

// PushDelivery sends web push notifications via the Web Push API (VAPID).
// Gracefully degrades if VAPID keys are not configured.
type PushDelivery struct {
	privateKey string
	publicKey  string
	subject    string
	enabled    bool
}

// NewPushDelivery creates a PushDelivery from environment variables.
// If JC_VAPID_PRIVATE_KEY or JC_VAPID_PUBLIC_KEY are empty, push is disabled.
// Keys are validated for correct VAPID format (base64url-encoded P-256
// ECDSA keys); invalid keys disable push with a warning log.
func NewPushDelivery() *PushDelivery {
	privKey := os.Getenv("JC_VAPID_PRIVATE_KEY")
	pubKey := os.Getenv("JC_VAPID_PUBLIC_KEY")

	subject := os.Getenv("JC_VAPID_SUBJECT")
	if subject == "" {
		// Default to a generic subject if not configured
		subject = "mailto:noreply@example.com"
	}

	if privKey == "" || pubKey == "" {
		slog.Warn("push_delivery: VAPID keys not configured, push notifications disabled")
		return &PushDelivery{enabled: false}
	}

	// Validate VAPID key formats before enabling.
	// Private key: base64url-encoded 32-byte P-256 scalar.
	// Public key: base64url-encoded 65-byte uncompressed P-256 point
	// (0x04 prefix + 32 bytes X + 32 bytes Y).
	if err := validateVAPIDKeys(privKey, pubKey); err != nil {
		slog.Error("push_delivery: invalid VAPID key format, push notifications disabled", "error", err)
		return &PushDelivery{enabled: false}
	}

	return &PushDelivery{
		privateKey: privKey,
		publicKey:  pubKey,
		subject:    subject,
		enabled:    true,
	}
}

// validateVAPIDKeys checks that the provided VAPID keys are correctly
// formatted base64url-encoded P-256 ECDSA keys with the expected byte
// lengths. This catches misconfiguration early instead of failing per-send.
func validateVAPIDKeys(privKey, pubKey string) error {
	privBytes, err := base64.RawURLEncoding.DecodeString(privKey)
	if err != nil {
		// Try standard base64 as fallback (some setups use padding).
		privBytes, err = base64.URLEncoding.DecodeString(privKey)
		if err != nil {
			return fmt.Errorf("VAPID private key is not valid base64url: %w", err)
		}
	}
	if len(privBytes) != 32 {
		return fmt.Errorf("VAPID private key must be 32 bytes (got %d)", len(privBytes))
	}

	pubBytes, err := base64.RawURLEncoding.DecodeString(pubKey)
	if err != nil {
		pubBytes, err = base64.URLEncoding.DecodeString(pubKey)
		if err != nil {
			return fmt.Errorf("VAPID public key is not valid base64url: %w", err)
		}
	}
	// Uncompressed P-256 public key: 0x04 + 32 (X) + 32 (Y) = 65 bytes.
	if len(pubBytes) != 65 {
		return fmt.Errorf("VAPID public key must be 65 bytes uncompressed (got %d)", len(pubBytes))
	}
	if pubBytes[0] != 0x04 {
		return errors.New("VAPID public key must be uncompressed (prefix 0x04)")
	}

	return nil
}

// IsEnabled reports whether VAPID keys are configured.
func (p *PushDelivery) IsEnabled() bool {
	return p.enabled
}

// PublicKey returns the VAPID public key for client-side subscription.
func (p *PushDelivery) PublicKey() string {
	return p.publicKey
}

// Send delivers a web push notification to the given browser subscription.
func (p *PushDelivery) Send(_ context.Context, endpoint, p256dh, auth string, payload []byte) error {
	if !p.enabled {
		return nil
	}

	subscription := &webpush.Subscription{
		Endpoint: endpoint,
		Keys: webpush.Keys{
			P256dh: p256dh,
			Auth:   auth,
		},
	}

	resp, err := webpush.SendNotification(payload, subscription, &webpush.Options{
		VAPIDPrivateKey: p.privateKey,
		VAPIDPublicKey:  p.publicKey,
		Subscriber:      p.subject,
		TTL:             60, // 60 seconds
	})
	if err != nil {
		return fmt.Errorf("sending web push: %w", err)
	}

	defer func() { _ = resp.Body.Close() }()

	// 410 Gone: subscription expired or revoked — caller should remove it.
	if resp.StatusCode == http.StatusGone {
		return errors.New("subscription expired (410 Gone)")
	}

	if resp.StatusCode >= 400 {
		return fmt.Errorf("web push returned status %d", resp.StatusCode)
	}

	return nil
}
