// Package config loads all JapaConnect configuration from environment variables.
//
// Every environment variable uses the JC_ prefix so that JapaConnect settings
// are easy to distinguish from system or third-party variables in any
// deployment environment. The Load function reads from os.Getenv and applies
// sensible defaults for development so the app boots without a full .env file.
package config

import (
	"fmt"
	"log/slog"
	"os"
	"strings"
)

// Config holds every configuration value the application needs at startup.
// Fields are intentionally flat — no nested structs — so it is easy to read
// from environment variables and easy to inspect in logs or debug output.
type Config struct {
	DatabaseURL              string // JC_DATABASE_URL — pgx connection string for PostgreSQL
	RedisAddr                string // JC_REDIS_ADDR — Valkey/Redis address for cache and Asynq queues
	MeilisearchURL           string // JC_MEILISEARCH_URL — Meilisearch HTTP endpoint
	MeilisearchKey           string // JC_MEILISEARCH_KEY — Meilisearch master key (empty for dev)
	GeoIPDBPath              string // JC_GEOIP_DB_PATH — path to the MaxMind GeoLite2 database file
	MaxmindLicenseKey        string // JC_MAXMIND_LICENSE_KEY — license key for downloading GeoIP databases
	GlitchtipDSN             string // JC_GLITCHTIP_DSN — GlitchTip DSN for error reporting
	ImgproxyKey              string // JC_IMGPROXY_KEY — signing key for imgproxy URL generation
	ImgproxySalt             string // JC_IMGPROXY_SALT — salt for imgproxy URL signing
	ImgproxyBaseURL          string // JC_IMGPROXY_BASE_URL — base URL for the imgproxy service
	HonchoURL                string // JC_HONCHO_URL — Honcho agent memory service URL
	ClamAVAddr               string // JC_CLAMAV_ADDR — ClamAV daemon address (host:port), default localhost:3310
	ReleaseSHA               string // JC_RELEASE_SHA — git commit SHA, used for release tracking
	Env                      string // JC_ENV — "development" or "production"
	Port                     string // JC_PORT — HTTP listen port, defaults to 8080
	JWTSecret                string // JC_JWT_SECRET — signing secret for JWT access tokens (empty = random in dev)
	JWTKeyID                 string // JC_JWT_KEY_ID — key ID for JWT kid header, defaults to "key-dev"
	SiteName                 string // JC_SITE_NAME — site name for LLMS.txt and branding
	SiteURL                  string // JC_SITE_URL — public-facing base URL (e.g. https://japaconnect.com)
	SiteDescription          string // JC_SITE_DESCRIPTION — site description for LLMS.txt
	OAuthGoogleID            string // JC_OAUTH_GOOGLE_CLIENT_ID — Google OAuth client ID
	OAuthGoogleSecret        string // JC_OAUTH_GOOGLE_CLIENT_SECRET — Google OAuth client secret
	OAuthGitHubID            string // JC_OAUTH_GITHUB_CLIENT_ID — GitHub OAuth client ID
	OAuthGitHubSecret        string // JC_OAUTH_GITHUB_CLIENT_SECRET — GitHub OAuth client secret
	OAuthAppleClientID       string // JC_OAUTH_APPLE_CLIENT_ID — Apple Sign-In client ID (Service ID)
	OAuthAppleTeamID         string // JC_OAUTH_APPLE_TEAM_ID — Apple Developer Team ID
	OAuthAppleKeyID          string // JC_OAUTH_APPLE_KEY_ID — Apple private key ID for client secret JWT
	OAuthApplePrivateKeyPath string // JC_OAUTH_APPLE_PRIVATE_KEY_PATH — path to .p8 private key file
	OAuthFacebookAppID       string // JC_OAUTH_FACEBOOK_APP_ID — Facebook App ID
	OAuthFacebookAppSecret   string // JC_OAUTH_FACEBOOK_APP_SECRET — Facebook App Secret
	OAuthRedirectBase        string // JC_OAUTH_REDIRECT_BASE — base URL for OAuth callbacks (e.g. https://api.japaconnect.com)
	TOTPEncryptionKey        string // JC_TOTP_ENCRYPTION_KEY — AES-256 key for encrypting TOTP secrets at rest (32-byte hex or base64)
	StripeWebhookSecret      string // JC_STRIPE_WEBHOOK_SECRET — Stripe webhook signing secret (whsec_...)
	NOWPaymentsIPNSecret     string // JC_NOWPAYMENTS_IPN_SECRET — NOWPayments IPN HMAC secret
	PaystackSecretKey          string // JC_PAYSTACK_SECRET_KEY — Paystack webhook HMAC secret
	XiaomiAPIKey             string // JC_XIAOMI_API_KEY — Xiaomi MiMo API key (tp- for Token Plan, sk- for Pay-as-you-go)
	CORSOrigins              string // JC_CORS_ORIGINS — comma-separated allowed CORS origins for production
	CookieDomain             string // JC_COOKIE_DOMAIN — cookie domain for cross-subdomain session sharing (e.g. ".japaconnect.com")
	InternalToken            string // JC_INTERNAL_TOKEN — shared secret for internal service-to-service endpoints (render helpers)
}

// Load reads all JC_ environment variables and returns a populated Config.
// Missing variables fall back to development-safe defaults so the server can
// boot locally without a complete .env file. In production every value should
// be set explicitly.
//
// The function never returns an error for missing values — it only returns an
// error if an internal invariant is violated, which currently cannot happen.
// The error return is kept so callers can handle future validation needs
// without changing the function signature.
func Load() (Config, error) {
	cfg := Config{
		DatabaseURL:              os.Getenv("JC_DATABASE_URL"),
		RedisAddr:                os.Getenv("JC_REDIS_ADDR"),
		MeilisearchURL:           os.Getenv("JC_MEILISEARCH_URL"),
		MeilisearchKey:           os.Getenv("JC_MEILISEARCH_KEY"),
		GeoIPDBPath:              os.Getenv("JC_GEOIP_DB_PATH"),
		MaxmindLicenseKey:        os.Getenv("JC_MAXMIND_LICENSE_KEY"),
		GlitchtipDSN:             os.Getenv("JC_GLITCHTIP_DSN"),
		ImgproxyKey:              os.Getenv("JC_IMGPROXY_KEY"),
		ImgproxySalt:             os.Getenv("JC_IMGPROXY_SALT"),
		ImgproxyBaseURL:          os.Getenv("JC_IMGPROXY_BASE_URL"),
		HonchoURL:                os.Getenv("JC_HONCHO_URL"),
		ClamAVAddr:               os.Getenv("JC_CLAMAV_ADDR"),
		ReleaseSHA:               os.Getenv("JC_RELEASE_SHA"),
		Env:                      os.Getenv("JC_ENV"),
		Port:                     os.Getenv("JC_PORT"),
		JWTSecret:                os.Getenv("JC_JWT_SECRET"),
		JWTKeyID:                 os.Getenv("JC_JWT_KEY_ID"),
		SiteName:                 os.Getenv("JC_SITE_NAME"),
		SiteURL:                  os.Getenv("JC_SITE_URL"),
		SiteDescription:          os.Getenv("JC_SITE_DESCRIPTION"),
		OAuthGoogleID:            os.Getenv("JC_OAUTH_GOOGLE_CLIENT_ID"),
		OAuthGoogleSecret:        os.Getenv("JC_OAUTH_GOOGLE_CLIENT_SECRET"),
		OAuthGitHubID:            os.Getenv("JC_OAUTH_GITHUB_CLIENT_ID"),
		OAuthGitHubSecret:        os.Getenv("JC_OAUTH_GITHUB_CLIENT_SECRET"),
		OAuthAppleClientID:       os.Getenv("JC_OAUTH_APPLE_CLIENT_ID"),
		OAuthAppleTeamID:         os.Getenv("JC_OAUTH_APPLE_TEAM_ID"),
		OAuthAppleKeyID:          os.Getenv("JC_OAUTH_APPLE_KEY_ID"),
		OAuthApplePrivateKeyPath: os.Getenv("JC_OAUTH_APPLE_PRIVATE_KEY_PATH"),
		OAuthFacebookAppID:       os.Getenv("JC_OAUTH_FACEBOOK_APP_ID"),
		OAuthFacebookAppSecret:   os.Getenv("JC_OAUTH_FACEBOOK_APP_SECRET"),
		OAuthRedirectBase:        os.Getenv("JC_OAUTH_REDIRECT_BASE"),
		TOTPEncryptionKey:        os.Getenv("JC_TOTP_ENCRYPTION_KEY"),
		StripeWebhookSecret:      os.Getenv("JC_STRIPE_WEBHOOK_SECRET"),
		NOWPaymentsIPNSecret:     os.Getenv("JC_NOWPAYMENTS_IPN_SECRET"),
		PaystackSecretKey:          os.Getenv("JC_PAYSTACK_SECRET_KEY"),
		XiaomiAPIKey:             os.Getenv("JC_XIAOMI_API_KEY"),
		CORSOrigins:              os.Getenv("JC_CORS_ORIGINS"),
		CookieDomain:             os.Getenv("JC_COOKIE_DOMAIN"),
		InternalToken:            os.Getenv("JC_INTERNAL_TOKEN"),
	}

	applyDefaults(&cfg)

	return cfg, nil
}

// applyDefaults fills in development-safe values for any field that is empty.
// Production deployments should set every variable explicitly — these defaults
// exist only so local development works without a full environment.
func applyDefaults(cfg *Config) {
	if cfg.DatabaseURL == "" {
		if cfg.Env == "production" {
			slog.Error("JC_DATABASE_URL is required in production")
			os.Exit(1)
		}
		cfg.DatabaseURL = "postgresql://japa:japa@localhost:5432/japa?sslmode=disable"
	}

	if cfg.RedisAddr == "" {
		cfg.RedisAddr = "localhost:6379"
	}

	if cfg.MeilisearchURL == "" {
		cfg.MeilisearchURL = "http://localhost:7700"
	}

	if cfg.ImgproxyBaseURL == "" {
		cfg.ImgproxyBaseURL = "http://localhost:8099"
	}

	if cfg.HonchoURL == "" {
		cfg.HonchoURL = "http://localhost:8001"
	}

	if cfg.ClamAVAddr == "" {
		cfg.ClamAVAddr = "localhost:3310"
	}

	if cfg.ReleaseSHA == "" {
		cfg.ReleaseSHA = "dev"
	}

	if cfg.Env == "" {
		cfg.Env = "development"
	}

	if cfg.Port == "" {
		cfg.Port = "8080"
	}

	if cfg.JWTKeyID == "" {
		cfg.JWTKeyID = "key-dev"
	}

	if cfg.SiteName == "" {
		cfg.SiteName = "JapaConnect"
	}

	if cfg.SiteURL == "" {
		cfg.SiteURL = "http://localhost:8080"
	}

	if cfg.SiteDescription == "" {
		cfg.SiteDescription = "A community platform for the Nigerian diaspora and Nigerians planning to relocate abroad"
	}

	if cfg.OAuthRedirectBase == "" {
		cfg.OAuthRedirectBase = "http://localhost:8080"
	}

	// CookieDomain defaults to empty in development (cookie scoped to exact host).
	// In production, set JC_COOKIE_DOMAIN explicitly (e.g. ".japaconnect.com").
	if cfg.IsProduction() && cfg.CookieDomain == "" {
		slog.Error("JC_COOKIE_DOMAIN must be set in production for cross-subdomain session sharing")
		os.Exit(1)
	}
}

// IsProduction returns true when the environment is set to production.
// Use this to gate behavior that should only run in production, such as
// enforcing HTTPS or enabling error reporting.
func (c Config) IsProduction() bool {
	return strings.EqualFold(c.Env, "production")
}

// IsDevelopment returns true when the environment is not production.
// This is the inverse of IsProduction and is provided for readability at
// call sites that check for development-specific behavior.
func (c Config) IsDevelopment() bool {
	return !c.IsProduction()
}

// String returns a human-readable summary of the config for logging at startup.
// Sensitive values (keys, DSNs, passwords) are masked so they never appear in
// logs in cleartext.
func (c Config) String() string {
	return fmt.Sprintf(
		"Config{Env=%s, Port=%s, DatabaseURL=%s, RedisAddr=%s, MeilisearchURL=%s, ReleaseSHA=%s}",
		c.Env,
		c.Port,
		maskURL(c.DatabaseURL),
		c.RedisAddr,
		c.MeilisearchURL,
		c.ReleaseSHA,
	)
}

// maskURL hides the credentials embedded in a connection string so they do
// not leak into logs. If the URL has no credentials, it is returned as-is.
func maskURL(rawURL string) string {
	// Find the "://" separator that marks the end of the scheme.
	schemeEnd := strings.Index(rawURL, "://")
	if schemeEnd == -1 {
		return rawURL
	}
	// Find the "@" that separates credentials from the host.
	at := strings.Index(rawURL[schemeEnd+3:], "@")
	if at == -1 {
		return rawURL // no credentials present
	}

	return rawURL[:schemeEnd+3] + "***@" + rawURL[schemeEnd+3+at+1:]
}
