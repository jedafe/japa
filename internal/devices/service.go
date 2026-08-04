// Package devices implements the Mobile Push / Device Token registration
// surface (Section 77, ADR-074).
//
// Ref: japaconnect-engineering/sections/77-flutter-mobile-app.md
//      japaconnect-engineering/adr/ADR-074-screen-contract.md
package devices

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"

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

// Task name constants.
const (
	TaskSendPush = "notification:send_push"
)

// TaskEnqueuer enqueues Asynq tasks fire-and-forget (Rule 8).
type TaskEnqueuer interface {
	Enqueue(taskType string, payload []byte, opts ...interface{}) error
}

// RegisterDeviceInput is the request body for registering a device token.
type RegisterDeviceInput struct {
	DeviceID   string `json:"device_id"`
	Token      string `json:"token"`
	Platform   string `json:"platform"`
	AppVersion string `json:"app_version"`
}

// DeviceInfo is the public representation of a device token.
type DeviceInfo struct {
	ID         string `json:"id"`
	DeviceID   string `json:"device_id"`
	Platform   string `json:"platform"`
	AppVersion string `json:"app_version"`
	IsActive   bool   `json:"is_active"`
	LastUsedAt string `json:"last_used_at"`
	CreatedAt  string `json:"created_at"`
}

// PushPayload is the payload for the notification:send_push task.
type PushPayload struct {
	TenantID string                 `json:"tenant_id"`
	UserID   string                 `json:"user_id"`
	Title    string                 `json:"title"`
	Body     string                 `json:"body"`
	Data     map[string]interface{} `json:"data"`
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
	store    *Store
	settings *config.Settings
	enqueuer TaskEnqueuer
}

// NewService creates a devices service.
func NewService(store *Store, settings *config.Settings) *Service {
	return &Service{store: store, settings: settings}
}

// SetEnqueuer injects the Asynq task enqueuer.
func (s *Service) SetEnqueuer(e TaskEnqueuer) { s.enqueuer = e }

// RegisterDevice registers or updates a device token for the user.
func (s *Service) RegisterDevice(ctx context.Context, tenantID, userID pgtype.UUID, input RegisterDeviceInput) (gen.DeviceToken, error) {
	if input.DeviceID == "" || input.Token == "" {
		return gen.DeviceToken{}, errors.New("devices: device_id and token are required")
	}
	if !IsValidPlatform(input.Platform) {
		return gen.DeviceToken{}, fmt.Errorf("devices: invalid platform: %s", input.Platform)
	}

	var appVersion *string
	if input.AppVersion != "" {
		appVersion = &input.AppVersion
	}

	return s.store.UpsertToken(ctx, gen.UpsertDeviceTokenParams{
		TenantID:   tenantID,
		UserID:     userID,
		DeviceID:   input.DeviceID,
		Token:      input.Token,
		Platform:   input.Platform,
		AppVersion: appVersion,
	})
}

// DeregisterDevice removes a device token (set is_active = false).
func (s *Service) DeregisterDevice(ctx context.Context, tenantID, userID pgtype.UUID, deviceID string) error {
	if deviceID == "" {
		return errors.New("devices: device_id is required")
	}
	return s.store.DeregisterToken(ctx, gen.DeregisterDeviceTokenParams{
		TenantID: tenantID,
		UserID:   userID,
		DeviceID: deviceID,
	})
}

// ListDevices returns the user's registered devices.
func (s *Service) ListDevices(ctx context.Context, tenantID, userID pgtype.UUID) ([]gen.DeviceToken, error) {
	return s.store.ListUserTokens(ctx, gen.ListUserDeviceTokensParams{
		TenantID:   tenantID,
		UserID:     userID,
		ActiveOnly: true,
	})
}

// SendPush enqueues a push notification to all of a user's active devices.
// This is called by the notification:send_push worker or other services
// that need to trigger push notifications.
func (s *Service) SendPush(ctx context.Context, tenantID, userID pgtype.UUID, payload PushPayload) error {
	enabled, err := s.settings.GetSettingBool(ctx, tenantID, "push.enabled")
	if err != nil || !enabled {
		return errors.New("devices: push notifications are disabled")
	}

	tokens, err := s.store.GetActiveTokensForUser(ctx, gen.GetActiveDeviceTokensForUserParams{
		TenantID: tenantID,
		UserID:   userID,
	})
	if err != nil {
		return fmt.Errorf("devices: get active tokens: %w", err)
	}
	if len(tokens) == 0 {
		return errors.New("devices: no active device tokens for user")
	}

	// Enqueue a push task per device token (fire-and-forget per Rule 8).
	if s.enqueuer != nil {
		for _, token := range tokens {
			taskPayload, _ := json.Marshal(map[string]interface{}{
				"tenant_id": tenantID.String(),
				"user_id":   userID.String(),
				"token":     token.Token,
				"platform":  token.Platform,
				"title":     payload.Title,
				"body":      payload.Body,
				"data":      payload.Data,
			})
			if err := s.enqueuer.Enqueue(TaskSendPush, taskPayload); err != nil {
				slog.Warn("devices: enqueue push failed", "error", err, "device_id", token.DeviceID)
			}
		}
	}

	return nil
}

// MarkTokenInactive is called when FCM/APNs reports a token as invalid.
func (s *Service) MarkTokenInactive(ctx context.Context, token string) error {
	return s.store.MarkTokenInactive(ctx, token)
}

// SendTestPush sends a test push to a specific device (admin route).
func (s *Service) SendTestPush(ctx context.Context, tenantID pgtype.UUID, deviceID string) error {
	token, err := s.store.GetTokenByDeviceID(ctx, gen.GetDeviceTokenByDeviceIDParams{
		TenantID: tenantID,
		DeviceID: deviceID,
	})
	if err != nil {
		return fmt.Errorf("devices: device not found: %w", err)
	}

	if s.enqueuer != nil {
		payload, _ := json.Marshal(map[string]interface{}{
			"tenant_id": tenantID.String(),
			"user_id":   token.UserID.String(),
			"token":     token.Token,
			"platform":  token.Platform,
			"title":     "Test Push",
			"body":      "This is a test push notification from JapaConnect.",
			"data":      map[string]interface{}{"type": "test"},
		})
		if err := s.enqueuer.Enqueue(TaskSendPush, payload); err != nil {
			return fmt.Errorf("devices: enqueue test push: %w", err)
		}
	}

	return nil
}
