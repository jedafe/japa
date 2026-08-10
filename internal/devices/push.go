// Package devices — push dispatch providers.
//
// This file implements the FCM (Firebase Cloud Messaging HTTP v1 API) and
// APNs (Apple Push Notification service) PushDispatcher implementations, plus
// a CompositeDispatcher that routes by platform.
//
// Credentials are read from system_settings per-tenant (hot-reloadable per
// Rule 3):
//   - push.apns_key_id         — APNs key ID (10-char hex)
//   - push.apns_team_id        — Apple Team ID (10-char hex)
//   - push.apns_private_key    — APNs private key PEM (p8, ES256)
//   - push.fcm_service_account — FCM service account JSON (HTTP v1 API)
//
// The legacy FCM server key API (push.fcm_server_key) was shut down by Google
// in June 2024. The HTTP v1 API requires a service account JSON instead.
// push.fcm_service_account is a new setting added alongside the spec's
// push.fcm_server_key (which is retained for backward compatibility but is
// no longer consumed).
//
// Ref: japaconnect-engineering/sections/77-flutter-mobile-app.md
package devices

import (
	"context"
	"crypto/ecdsa"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// FCM service-account JSON subset (only the fields we need).
type fcmServiceAccount struct {
	ProjectID  string `json:"project_id"`
	PrivateKey string `json:"private_key"`
	ClientEmail string `json:"client_email"`
}

// fcmTokenCache holds a cached OAuth2 access token for FCM.
type fcmTokenCache struct {
	mu        sync.Mutex
	token     string
	expiresAt time.Time
}

// CompositeDispatcher routes push dispatch to FCM (android) or APNs (ios)
// based on the platform. Web push is NOT handled here — it uses the VAPID
// Web Push API in internal/notifications/push.go.
type CompositeDispatcher struct {
	settings settingReader
	apns     *apnsState
	fcm      *fcmState
	client   *http.Client
}

type apnsState struct {
	mu        sync.Mutex
	jwt       string
	jwtExp    time.Time
}

type fcmState struct {
	cache     fcmTokenCache
}

// NewCompositeDispatcher creates a dispatcher that reads FCM/APNs credentials
// from system_settings per-tenant on each dispatch. The settings cache
// (5-min TTL in config.Settings) prevents per-request DB lookups.
func NewCompositeDispatcher(settings settingReader) *CompositeDispatcher {
	return &CompositeDispatcher{
		settings: settings,
		apns:     &apnsState{},
		fcm:      &fcmState{},
		client:   &http.Client{Timeout: 15 * time.Second},
	}
}

// Dispatch routes to the appropriate provider based on platform.
func (d *CompositeDispatcher) Dispatch(ctx context.Context, tenantID pgtype.UUID, platform, token string, payload PushPayload) error {
	switch platform {
	case PlatformIOS:
		return d.dispatchAPNs(ctx, tenantID, token, payload)
	case PlatformAndroid:
		return d.dispatchFCM(ctx, tenantID, token, payload)
	case PlatformWeb:
		// Web push is handled by internal/notifications/push.go (VAPID).
		// Device tokens for web should not reach here; return a clear error.
		return fmt.Errorf("devices: web push is handled by the VAPID push delivery, not the device token pipeline")
	default:
		return fmt.Errorf("devices: unsupported platform %q", platform)
	}
}

// --- APNs ---

const (
	apnsURL     = "https://api.push.apple.com"
	apnsURLSandbox = "https://api.sandbox.push.apple.com"
	apnsJWTLead = 30 * time.Second // refresh JWT 30s before expiry
)

func (d *CompositeDispatcher) dispatchAPNs(ctx context.Context, tenantID pgtype.UUID, deviceToken string, payload PushPayload) error {
	keyID, teamID, privKeyPEM, err := d.loadAPNsCredentials(ctx, tenantID)
	if err != nil {
		return fmt.Errorf("devices: apns credentials: %w", err)
	}

	jwtToken, err := d.getAPNsJWT(keyID, teamID, privKeyPEM)
	if err != nil {
		return fmt.Errorf("devices: apns jwt: %w", err)
	}

	body, err := json.Marshal(map[string]interface{}{
		"aps": map[string]interface{}{
			"alert": map[string]string{
				"title": payload.Title,
				"body":  payload.Body,
			},
			"sound": "default",
		},
		"data": payload.Data,
	})
	if err != nil {
		return fmt.Errorf("devices: apns marshal: %w", err)
	}

	url := apnsURL + "/3/device/" + deviceToken
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, strings.NewReader(string(body)))
	if err != nil {
		return fmt.Errorf("devices: apns request: %w", err)
	}
	req.Header.Set("authorization", "bearer "+jwtToken)
	req.Header.Set("apns-topic", "com.japaconnect.app") // bundle ID
	req.Header.Set("content-type", "application/json")

	resp, err := d.client.Do(req)
	if err != nil {
		return fmt.Errorf("devices: apns send: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusGone {
		return ErrInvalidToken
	}
	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("devices: apns returned %d: %s", resp.StatusCode, string(respBody))
	}
	return nil
}

func (d *CompositeDispatcher) loadAPNsCredentials(ctx context.Context, tenantID pgtype.UUID) (keyID, teamID, privKeyPEM string, err error) {
	keyID, err = d.settings.GetSetting(ctx, tenantID, "push.apns_key_id")
	if err != nil || keyID == "" {
		return "", "", "", fmt.Errorf("push.apns_key_id not configured")
	}
	teamID, err = d.settings.GetSetting(ctx, tenantID, "push.apns_team_id")
	if err != nil || teamID == "" {
		return "", "", "", fmt.Errorf("push.apns_team_id not configured")
	}
	privKeyPEM, err = d.settings.GetSetting(ctx, tenantID, "push.apns_private_key")
	if err != nil || privKeyPEM == "" {
		return "", "", "", fmt.Errorf("push.apns_private_key not configured")
	}
	return keyID, teamID, privKeyPEM, nil
}

func (d *CompositeDispatcher) getAPNsJWT(keyID, teamID, privKeyPEM string) (string, error) {
	d.apns.mu.Lock()
	defer d.apns.mu.Unlock()

	// Return cached JWT if still valid (with lead time).
	if d.apns.jwt != "" && time.Now().Before(d.apns.jwtExp.Add(-apnsJWTLead)) {
		return d.apns.jwt, nil
	}

	// Parse the p8 private key (ECDSA P-256).
	block, _ := pem.Decode([]byte(privKeyPEM))
	if block == nil {
		return "", fmt.Errorf("apns: failed to decode PEM block")
	}
	key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		// Try PKCS1 as fallback.
		if ecdsaKey, e := x509.ParseECPrivateKey(block.Bytes); e == nil {
			key = ecdsaKey
		} else {
			return "", fmt.Errorf("apns: parse private key: %w", err)
		}
	}
	ecdsaKey, ok := key.(*ecdsa.PrivateKey)
	if !ok {
		return "", fmt.Errorf("apns: private key is not ECDSA (got %T)", key)
	}

	now := time.Now()
	tok := jwt.NewWithClaims(jwt.SigningMethodES256, jwt.MapClaims{
		"iss": teamID,
		"iat": now.Unix(),
		"exp": now.Add(1 * time.Hour).Unix(),
	})
	tok.Header["kid"] = keyID

	signed, err := tok.SignedString(ecdsaKey)
	if err != nil {
		return "", fmt.Errorf("apns: sign jwt: %w", err)
	}

	d.apns.jwt = signed
	d.apns.jwtExp = now.Add(1 * time.Hour)
	return signed, nil
}

// --- FCM (HTTP v1 API) ---

const (
	fcmEndpoint = "https://fcm.googleapis.com/v1/projects/%s/messages:send"
	fcmTokenURL = "https://oauth2.googleapis.com/token"
	fcmJWTLead  = 60 * time.Second // refresh OAuth2 token 60s before expiry
)

func (d *CompositeDispatcher) dispatchFCM(ctx context.Context, tenantID pgtype.UUID, deviceToken string, payload PushPayload) error {
	sa, err := d.loadFCMServiceAccount(ctx, tenantID)
	if err != nil {
		return fmt.Errorf("devices: fcm credentials: %w", err)
	}

	accessToken, err := d.getFCMAccessToken(sa)
	if err != nil {
		return fmt.Errorf("devices: fcm oauth2: %w", err)
	}

	// Build the FCM v1 message.
	message := map[string]interface{}{
		"message": map[string]interface{}{
			"token": deviceToken,
			"notification": map[string]string{
				"title": payload.Title,
				"body":  payload.Body,
			},
			"data": stringifyData(payload.Data),
		},
	}
	body, err := json.Marshal(message)
	if err != nil {
		return fmt.Errorf("devices: fcm marshal: %w", err)
	}

	url := fmt.Sprintf(fcmEndpoint, sa.ProjectID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, strings.NewReader(string(body)))
	if err != nil {
		return fmt.Errorf("devices: fcm request: %w", err)
	}
	req.Header.Set("authorization", "Bearer "+accessToken)
	req.Header.Set("content-type", "application/json")

	resp, err := d.client.Do(req)
	if err != nil {
		return fmt.Errorf("devices: fcm send: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		// FCM returns 404 with "UNREGISTERED" or 400 with "INVALID_ARGUMENT"
		// for invalid/expired tokens.
		if resp.StatusCode == http.StatusNotFound || strings.Contains(string(respBody), "UNREGISTERED") {
			return ErrInvalidToken
		}
		return fmt.Errorf("devices: fcm returned %d: %s", resp.StatusCode, string(respBody))
	}
	return nil
}

func (d *CompositeDispatcher) loadFCMServiceAccount(ctx context.Context, tenantID pgtype.UUID) (*fcmServiceAccount, error) {
	raw, err := d.settings.GetSetting(ctx, tenantID, "push.fcm_service_account")
	if err != nil || raw == "" {
		return nil, fmt.Errorf("push.fcm_service_account not configured")
	}
	// The setting value is stored as a JSON string (double-encoded in
	// system_settings). Strip the outer JSON string quotes if present.
	if strings.HasPrefix(raw, "\"") && strings.HasSuffix(raw, "\"") {
		var unquoted string
		if err := json.Unmarshal([]byte(raw), &unquoted); err == nil {
			raw = unquoted
		}
	}
	var sa fcmServiceAccount
	if err := json.Unmarshal([]byte(raw), &sa); err != nil {
		return nil, fmt.Errorf("parse service account json: %w", err)
	}
	if sa.ProjectID == "" || sa.PrivateKey == "" || sa.ClientEmail == "" {
		return nil, fmt.Errorf("service account json missing required fields (project_id, private_key, client_email)")
	}
	return &sa, nil
}

func (d *CompositeDispatcher) getFCMAccessToken(sa *fcmServiceAccount) (string, error) {
	d.fcm.cache.mu.Lock()
	defer d.fcm.cache.mu.Unlock()

	// Return cached token if still valid (with lead time).
	if d.fcm.cache.token != "" && time.Now().Before(d.fcm.cache.expiresAt.Add(-fcmJWTLead)) {
		return d.fcm.cache.token, nil
	}

	// Parse the RSA private key from the service account.
	block, _ := pem.Decode([]byte(sa.PrivateKey))
	if block == nil {
		return "", fmt.Errorf("fcm: failed to decode PEM block")
	}
	key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		if rsaKey, e := x509.ParsePKCS1PrivateKey(block.Bytes); e == nil {
			key = rsaKey
		} else {
			return "", fmt.Errorf("fcm: parse private key: %w", err)
		}
	}
	rsaKey, ok := key.(*rsa.PrivateKey)
	if !ok {
		return "", fmt.Errorf("fcm: private key is not RSA (got %T)", key)
	}

	now := time.Now()
	jwtToken := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{
		"iss":   sa.ClientEmail,
		"scope": "https://www.googleapis.com/auth/firebase.messaging",
		"aud":   fcmTokenURL,
		"exp":   now.Add(1 * time.Hour).Unix(),
		"iat":   now.Unix(),
	})
	signed, err := jwtToken.SignedString(rsaKey)
	if err != nil {
		return "", fmt.Errorf("fcm: sign jwt: %w", err)
	}

	// Exchange the JWT for an OAuth2 access token.
	form := fmt.Sprintf("grant_type=urn%%3Aietf%%3Aparams%%3Aoauth%%3Agrant-type%%3Ajwt-bearer&assertion=%s", signed)
	req, err := http.NewRequest(http.MethodPost, fcmTokenURL, strings.NewReader(form))
	if err != nil {
		return "", fmt.Errorf("fcm: token request: %w", err)
	}
	req.Header.Set("content-type", "application/x-www-form-urlencoded")

	resp, err := d.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("fcm: token exchange: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return "", fmt.Errorf("fcm: token exchange returned %d: %s", resp.StatusCode, string(body))
	}

	var tokenResp struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return "", fmt.Errorf("fcm: decode token response: %w", err)
	}
	if tokenResp.AccessToken == "" {
		return "", fmt.Errorf("fcm: empty access token in response")
	}

	expiry := time.Duration(tokenResp.ExpiresIn) * time.Second
	if expiry <= 0 {
		expiry = 45 * time.Minute // safe default
	}

	d.fcm.cache.token = tokenResp.AccessToken
	d.fcm.cache.expiresAt = now.Add(expiry)
	slog.Info("devices: fcm access token refreshed", "expires_in", tokenResp.ExpiresIn)
	return d.fcm.cache.token, nil
}

// stringifyData converts map[string]interface{} to map[string]string for FCM's
// data field (FCM v1 requires string values).
func stringifyData(data map[string]interface{}) map[string]string {
	out := make(map[string]string, len(data))
	for k, v := range data {
		switch val := v.(type) {
		case string:
			out[k] = val
		default:
			b, _ := json.Marshal(val)
			out[k] = string(b)
		}
	}
	return out
}

// --- NoopDispatcher (for testing / disabled state) ---

// NoopDispatcher is a PushDispatcher that always returns ErrNoDispatcher.
// Useful as an explicit "disabled" sentinel.
type NoopDispatcher struct{}

func (NoopDispatcher) Dispatch(_ context.Context, _ pgtype.UUID, _, _ string, _ PushPayload) error {
	return ErrNoDispatcher
}

// Ensure the base64 import is used (for potential future token encoding).
var _ = base64.StdEncoding
