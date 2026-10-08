// Package devices implements the Mobile Push / Device Token registration
// surface (Section 77, ADR-074).
//
// Ref: japaconnect-engineering/sections/77-flutter-mobile-app.md
//
//	japaconnect-engineering/adr/ADR-074-screen-contract.md
package devices

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/config"
	"github.com/jedafe/japa/internal/db/gen"
)

// Platform constants.
const (
	PlatformIOS     = "ios"
	PlatformAndroid = "android"
	PlatformWeb     = "web"
)

// Sentinel errors for structured handling (Rule 5).
var (
	ErrMissingFields   = errors.New("devices: device_id and token are required")
	ErrInvalidPlatform = errors.New("devices: invalid platform")
	ErrNoActiveTokens  = errors.New("devices: no active device tokens for user")
	ErrPushDisabled    = errors.New("devices: push notifications are disabled")
	ErrNoDispatcher    = errors.New("devices: no push dispatcher configured")
	ErrDeviceNotFound  = errors.New("devices: device not found")
)

// PushPayload is the resolved push payload handed to a PushDispatcher.
// It is the terminal representation — no further task enqueueing happens
// below this point (BE-007: the service is the terminal dispatcher, not an
// enqueuer of notification:send_push).
type PushPayload struct {
	Title string                 `json:"title"`
	Body  string                 `json:"body"`
	Data  map[string]interface{} `json:"data"`
}

// PushDispatcher is the FCM/APNs provider abstraction. Implementations
// deliver a single push to a single device token and report invalid-token
// feedback so the service can mark the token inactive (§77: "Invalid-token
// feedback from FCM/APNs sets device_tokens.is_active = false").
//
// ErrInvalidToken is a sentinel returned by Dispatch when the provider
// reports the token as invalid/expired (FCM "NotRegistered" / APNs 410).
type PushDispatcher interface {
	// Dispatch delivers push to one device token. Returns ErrInvalidToken
	// when the provider reports the token as invalid/expired.
	Dispatch(ctx context.Context, platform, token string, payload PushPayload) error
}

// ErrInvalidToken is returned by a PushDispatcher when the provider reports
// the device token as invalid or expired.
var ErrInvalidToken = errors.New("devices: invalid or expired device token")

// RegisterDeviceInput is the request body for registering a device token.
type RegisterDeviceInput struct {
	DeviceID   string `json:"device_id"`
	Token      string `json:"token"`
	Platform   string `json:"platform"`
	AppVersion string `json:"app_version"`
}

// DeviceInfo is the public representation of a device token. It deliberately
// omits the push Token so it can be safely serialized to clients (BE-005).
type DeviceInfo struct {
	ID         string `json:"id"`
	DeviceID   string `json:"device_id"`
	Platform   string `json:"platform"`
	AppVersion string `json:"app_version"`
	IsActive   bool   `json:"is_active"`
	LastUsedAt string `json:"last_used_at"`
	CreatedAt  string `json:"created_at"`
}

// settingReader is the subset of *config.Settings the devices service needs.
// Declared as an interface so the service can be unit-tested with a stub
// (dependency inversion over the concrete *config.Settings).
type settingReader interface {
	GetSetting(ctx context.Context, key string) (string, error)
	GetSettingBool(ctx context.Context, key string) (bool, error)
}

// FeatureChecker resolves feature flag state from the feature_flags table —
// the single source of truth for the push.enabled reachability toggle.
// *features.Checker satisfies it structurally.
type FeatureChecker interface {
	IsEnabled(ctx context.Context, key string) bool
}

// IsValidPlatform checks if a platform value is valid.
func IsValidPlatform(platform string) bool {
	switch platform {
	case PlatformIOS, PlatformAndroid, PlatformWeb:
		return true
	}

	return false
}

// Service implements the device token business logic.
type Service struct {
	store      *Store
	settings   settingReader
	checker    FeatureChecker
	dispatcher PushDispatcher
}

// NewService creates a devices service.
func NewService(store *Store, settings *config.Settings) *Service {
	return &Service{store: store, settings: settings}
}

// SetFeatureChecker injects the feature flag checker so push reachability
// reads the feature_flags table instead of system_settings. Falls back to
// the legacy push.enabled setting read when nil.
func (s *Service) SetFeatureChecker(c FeatureChecker) { s.checker = c }

// SetDispatcher injects the FCM/APNs push dispatcher. Without a dispatcher,
// SendPush/SendTestPush return ErrNoDispatcher (BE-006: never silently no-op).
func (s *Service) SetDispatcher(d PushDispatcher) { s.dispatcher = d }

// RegisterDevice registers or updates a device token for the user.
func (s *Service) RegisterDevice(ctx context.Context, userID pgtype.UUID, input RegisterDeviceInput) (DeviceInfo, error) {
	if input.DeviceID == "" || input.Token == "" {
		return DeviceInfo{}, ErrMissingFields
	}

	if !IsValidPlatform(input.Platform) {
		return DeviceInfo{}, fmt.Errorf("%w: %s", ErrInvalidPlatform, input.Platform)
	}

	var appVersion *string
	if input.AppVersion != "" {
		appVersion = &input.AppVersion
	}

	row, err := s.store.UpsertToken(ctx, gen.UpsertDeviceTokenParams{

		UserID:     userID,
		DeviceID:   input.DeviceID,
		Token:      input.Token,
		Platform:   input.Platform,
		AppVersion: appVersion,
	})
	if err != nil {
		return DeviceInfo{}, fmt.Errorf("devices: upsert token: %w", err)
	}

	return toDeviceInfo(row), nil
}

// DeregisterDevice removes a device token (set is_active = false).
func (s *Service) DeregisterDevice(ctx context.Context, userID pgtype.UUID, deviceID string) error {
	if deviceID == "" {
		return ErrMissingFields
	}

	if err := s.store.DeregisterToken(ctx, gen.DeregisterDeviceTokenParams{

		UserID:   userID,
		DeviceID: deviceID,
	}); err != nil {
		return fmt.Errorf("devices: deregister token: %w", err)
	}

	return nil
}

// ListDevices returns the user's registered devices as DeviceInfo (no token).
func (s *Service) ListDevices(ctx context.Context, userID pgtype.UUID) ([]DeviceInfo, error) {
	rows, err := s.store.ListUserTokens(ctx, gen.ListUserDeviceTokensParams{

		UserID:     userID,
		ActiveOnly: true,
	})
	if err != nil {
		return nil, fmt.Errorf("devices: list user tokens: %w", err)
	}

	out := make([]DeviceInfo, 0, len(rows))
	for _, r := range rows {
		out = append(out, toDeviceInfo(r))
	}

	return out, nil
}

// SendPush dispatches a push notification to all of a user's active devices.
// This is the terminal dispatch step called by the notification:send_push
// worker via PushAdapter (BE-007: it does NOT re-enqueue notification:send_push).
func (s *Service) SendPush(ctx context.Context, userID pgtype.UUID, payload PushPayload) error {
	// push.enabled is a feature flag — read it from feature_flags when a
	// checker is wired; the legacy system_settings read is the fallback.
	pushEnabled := false
	if s.checker != nil {
		pushEnabled = s.checker.IsEnabled(ctx, "push.enabled")
	} else if s.settings != nil {
		enabled, err := s.settings.GetSettingBool(ctx, "push.enabled")
		if err != nil {
			// BE-008: surface config errors distinctly from the disabled state.
			return fmt.Errorf("devices: read push.enabled setting: %w", err)
		}

		pushEnabled = enabled
	}

	if !pushEnabled {
		return ErrPushDisabled
	}

	if s.dispatcher == nil {
		return ErrNoDispatcher
	}

	tokens, err := s.store.GetActiveTokensForUser(ctx, userID)
	if err != nil {
		return fmt.Errorf("devices: get active tokens: %w", err)
	}

	if len(tokens) == 0 {
		return ErrNoActiveTokens
	}

	// Dispatch per token. Invalid-token feedback marks the token inactive
	// (§77). Dispatch errors for one token do not abort the others.
	var failed int

	for _, t := range tokens {
		if dErr := s.dispatcher.Dispatch(ctx, t.Platform, t.Token, payload); dErr != nil {
			if errors.Is(dErr, ErrInvalidToken) {
				if markErr := s.store.MarkTokenInactive(ctx, t.Token); markErr != nil {
					slog.Warn("devices: mark invalid token inactive failed", "error", markErr)
				}

				continue
			}

			slog.Warn("devices: dispatch push failed", "error", dErr, "device_id", t.DeviceID)

			failed++
		}
	}

	if failed == len(tokens) {
		return fmt.Errorf("devices: all %d dispatches failed", failed)
	}

	return nil
}

// MarkTokenInactive is called when FCM/APNs reports a token as invalid.
func (s *Service) MarkTokenInactive(ctx context.Context, token string) error {
	return s.store.MarkTokenInactive(ctx, token)
}

// SendTestPush sends a test push to a specific device (admin route, §77).
// It resolves the device token and dispatches directly via PushDispatcher
// (BE-007: no notification:send_push enqueue, which would carry the wrong
// payload shape for the worker).
func (s *Service) SendTestPush(ctx context.Context, deviceID string) error {
	if s.dispatcher == nil {
		return ErrNoDispatcher
	}

	token, err := s.store.GetTokenByDeviceID(ctx, deviceID)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrDeviceNotFound, err)
	}

	payload := PushPayload{
		Title: "Test Push",
		Body:  "This is a test push notification from JapaConnect.",
		Data:  map[string]interface{}{"type": "test"},
	}
	if err := s.dispatcher.Dispatch(ctx, token.Platform, token.Token, payload); err != nil {
		if errors.Is(err, ErrInvalidToken) {
			if markErr := s.store.MarkTokenInactive(ctx, token.Token); markErr != nil {
				slog.Warn("devices: mark invalid test-push token inactive failed", "error", markErr)
			}
		}

		return fmt.Errorf("devices: dispatch test push: %w", err)
	}

	return nil
}

// toDeviceInfo maps a sqlc DeviceToken row to the public DeviceInfo, omitting
// the push Token (BE-005).
func toDeviceInfo(r gen.DeviceToken) DeviceInfo {
	info := DeviceInfo{
		ID:        r.ID.String(),
		DeviceID:  r.DeviceID,
		Platform:  r.Platform,
		IsActive:  r.IsActive,
		CreatedAt: formatTs(r.CreatedAt),
	}
	if r.AppVersion != nil {
		info.AppVersion = *r.AppVersion
	}

	if r.LastUsedAt.Valid {
		info.LastUsedAt = formatTs(r.LastUsedAt)
	}

	return info
}

func formatTs(t pgtype.Timestamptz) string {
	if !t.Valid || t.Time.IsZero() {
		return ""
	}

	return t.Time.UTC().Format(time.RFC3339)
}
