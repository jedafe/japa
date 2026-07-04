package webhooks

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"time"
)

// SchemaVersion is the current webhook envelope schema version. Bumping this
// signals to consumers that the payload structure has changed.
const SchemaVersion = "1.0"

// WebhookEnvelope is the versioned payload format sent to every webhook
// endpoint. The envelope wraps the event-specific data with metadata that
// consumers need to verify, route, and deduplicate deliveries.
type WebhookEnvelope struct {
	EventID       string    `json:"event_id"`
	EventType     string    `json:"event_type"`
	OccurredAt    time.Time `json:"occurred_at"`
	SchemaVersion string    `json:"schema_version"`
	Data          any       `json:"data"`
}

// Marshal serializes the envelope to JSON bytes for delivery.
func (e WebhookEnvelope) Marshal() ([]byte, error) {
	return json.Marshal(e)
}

// SignPayload returns the hex-encoded HMAC-SHA256 signature of payload using
// the provided secret. This is the value placed in the X-Webhook-Signature
// header so receivers can verify authenticity.
func SignPayload(secret string, payload []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)

	return hex.EncodeToString(mac.Sum(nil))
}
