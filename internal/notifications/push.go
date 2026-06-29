package notifications

import (
	"context"
	"fmt"
	"log/slog"
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
func NewPushDelivery() *PushDelivery {
	privKey := os.Getenv("JC_VAPID_PRIVATE_KEY")
	pubKey := os.Getenv("JC_VAPID_PUBLIC_KEY")
	subject := os.Getenv("JC_VAPID_SUBJECT")
	if subject == "" {
		subject = "mailto:admin@japaconnect.com"
	}

	if privKey == "" || pubKey == "" {
		slog.Warn("push_delivery: VAPID keys not configured, push notifications disabled")
		return &PushDelivery{enabled: false}
	}

	return &PushDelivery{
		privateKey: privKey,
		publicKey:  pubKey,
		subject:    subject,
		enabled:    true,
	}
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
func (p *PushDelivery) Send(ctx context.Context, endpoint, p256dh, auth string, payload []byte) error {
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
	defer resp.Body.Close()

	// 410 Gone: subscription expired or revoked — caller should remove it.
	if resp.StatusCode == 410 {
		return fmt.Errorf("subscription expired (410 Gone)")
	}
	if resp.StatusCode >= 400 {
		return fmt.Errorf("web push returned status %d", resp.StatusCode)
	}

	return nil
}
