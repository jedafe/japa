package email

import "context"

// EmailRequest is the standard envelope sent to any email provider.
//
//nolint:revive // exported: stutter naming is intentional
type EmailRequest struct {
	To       string
	Subject  string
	Body     string
	IsHTML   bool
	Category string // "transactional" or "marketing"
	// Headers carries optional extra SMTP headers (e.g. List-Unsubscribe,
	// List-Unsubscribe-Post) merged into the outgoing message by providers
	// that support custom headers. Providers that do not support custom
	// headers silently ignore this field.
	Headers map[string]string
}

// EmailResponse is the standard envelope returned by any email provider.
//
//nolint:revive // exported: stutter naming is intentional
type EmailResponse struct {
	ProviderRef string // provider-assigned reference ID for audit trail
	Status      string // "success", "failed"
}

// EmailProvider defines the contract every email backend must implement.
// Business logic never calls a vendor SDK directly; it always goes
// through this interface so providers can be swapped or fallback can
// be applied without touching callers.
//
//nolint:revive // exported: stutter naming is intentional
type EmailProvider interface {
	// Name returns a human-readable identifier for logs and dashboards.
	Name() string
	// Send dispatches an email request to the provider and returns the response.
	Send(ctx context.Context, req EmailRequest) (EmailResponse, error)
}
