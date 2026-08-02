// Package config loads all JapaConnect configuration from environment variables.
//
// Every environment variable uses the JC_ prefix so that JapaConnect settings
// are easy to distinguish from system or third-party variables in any
// deployment environment. The Load function reads from os.Getenv and applies
// sensible defaults for development so the app boots without a full .env file.
package config

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// Config holds every configuration value the application needs at startup.
// Fields are intentionally flat — no nested structs — so it is easy to read
// from environment variables and easy to inspect in logs or debug output.
type Config struct {
	DatabaseURL              string // JC_DATABASE_URL — pgx connection string for PostgreSQL
	RedisAddr                string // JC_REDIS_ADDR — Valkey/Redis address for cache and Asynq queues
	RedisPassword            string // JC_REDIS_PASSWORD — optional auth password for Valkey/Redis
	RedisDB                  int    // JC_REDIS_DB — Valkey/Redis database number (0 default)
	RedisPoolSize            int    // JC_REDIS_POOL_SIZE — connection pool size (0 = valkey-go default)
	RedisDialTimeoutMs       int    // JC_REDIS_DIAL_TIMEOUT_MS — dial timeout in milliseconds (0 = default)
	RedisReadTimeoutMs       int    // JC_REDIS_READ_TIMEOUT_MS — read/write timeout in milliseconds (0 = default)
	RedisTLSEnabled          bool   // JC_REDIS_TLS_ENABLED — enable TLS connection to Valkey/Redis
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
	SiteURL                  string // JC_SITE_URL — public-facing base URL (e.g. https://example.com)
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
	OAuthRedirectBase        string // JC_OAUTH_REDIRECT_BASE — base URL for OAuth callbacks (e.g. https://api.example.com)
	TOTPEncryptionKey        string // JC_TOTP_ENCRYPTION_KEY — AES-256 key for encrypting TOTP secrets at rest (32-byte hex or base64)
	StripeSecretKey          string // JC_STRIPE_SECRET_KEY — Stripe API secret key (sk_live_... / sk_test_...)
	StripeWebhookSecret      string // JC_STRIPE_WEBHOOK_SECRET — Stripe webhook signing secret (whsec_...)
	NOWPaymentsIPNSecret     string // JC_NOWPAYMENTS_IPN_SECRET — NOWPayments IPN HMAC secret (also used as API key)
	PaystackSecretKey          string // JC_PAYSTACK_SECRET_KEY — Paystack webhook HMAC secret
	XiaomiAPIKey             string // JC_XIAOMI_API_KEY — Xiaomi MiMo API key (tp- for Token Plan, sk- for Pay-as-you-go)
	CORSOrigins              string // JC_CORS_ORIGINS — comma-separated allowed CORS origins for production
	CookieDomain             string // JC_COOKIE_DOMAIN — cookie domain for cross-subdomain session sharing (e.g. ".example.com")
	InternalToken            string // JC_INTERNAL_TOKEN — shared secret for internal service-to-service endpoints (render helpers)
	UnsplashAccessKey        string // JC_UNSPLASH_ACCESS_KEY — Unsplash API access key
	PexelsAPIKey             string // JC_PEXELS_API_KEY — Pexels API key
	PixabayAPIKey            string // JC_PIXABAY_API_KEY — Pixabay API key
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
	// In non-production environments, load a .env file from the working
	// directory if present. This lets `japa-api` boot with the project's
	// local secrets (JC_JWT_SECRET, JC_TOTP_ENCRYPTION_KEY, …) even when the
	// process was not started through a wrapper that sourced .env. Variables
	// already present in the real environment are never overridden, so
	// explicit env vars and CI/production deployments always win. Production
	// is skipped entirely: a production deployment must set its variables
	// explicitly and must not rely on a checked-in .env file.
	if os.Getenv("JC_ENV") != "production" {
		loadDotEnv(".env")
	}

	cfg := Config{
		DatabaseURL:              os.Getenv("JC_DATABASE_URL"),
		RedisAddr:                os.Getenv("JC_REDIS_ADDR"),
		RedisPassword:            os.Getenv("JC_REDIS_PASSWORD"),
		RedisDB:                  getEnvInt("JC_REDIS_DB", 0),
		RedisPoolSize:            getEnvInt("JC_REDIS_POOL_SIZE", 0),
		RedisDialTimeoutMs:       getEnvInt("JC_REDIS_DIAL_TIMEOUT_MS", 0),
		RedisReadTimeoutMs:       getEnvInt("JC_REDIS_READ_TIMEOUT_MS", 0),
		RedisTLSEnabled:          getEnvBool("JC_REDIS_TLS_ENABLED", false),
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
		SiteName:                 getEnvWithFallback("JC_SITE_NAME", "JC_SITE_TITLE"),
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
		StripeSecretKey:          os.Getenv("JC_STRIPE_SECRET_KEY"),
		StripeWebhookSecret:      os.Getenv("JC_STRIPE_WEBHOOK_SECRET"),
		NOWPaymentsIPNSecret:     os.Getenv("JC_NOWPAYMENTS_IPN_SECRET"),
		PaystackSecretKey:          os.Getenv("JC_PAYSTACK_SECRET_KEY"),
		XiaomiAPIKey:             os.Getenv("JC_XIAOMI_API_KEY"),
		CORSOrigins:              os.Getenv("JC_CORS_ORIGINS"),
		CookieDomain:             os.Getenv("JC_COOKIE_DOMAIN"),
		InternalToken:            os.Getenv("JC_INTERNAL_TOKEN"),
		UnsplashAccessKey:        os.Getenv("JC_UNSPLASH_ACCESS_KEY"),
		PexelsAPIKey:             os.Getenv("JC_PEXELS_API_KEY"),
		PixabayAPIKey:            os.Getenv("JC_PIXABAY_API_KEY"),
	}

	if err := applyDefaults(&cfg); err != nil {
		return Config{}, err
	}

	return cfg, nil
}

// minJWTSecretLength is the shortest JC_JWT_SECRET accepted in production.
const minJWTSecretLength = 32

// applyDefaults fills in development-safe values for any field that is empty.
// Production deployments should set every variable explicitly — these defaults
// exist only so local development works without a full environment.
func applyDefaults(cfg *Config) error {
	if cfg.DatabaseURL == "" {
		if cfg.Env == "production" {
			return fmt.Errorf("JC_DATABASE_URL is required in production")
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
		// Support both JC_SITE_NAME and JC_SITE_TITLE for backward compatibility
		if title := os.Getenv("JC_SITE_TITLE"); title != "" {
			cfg.SiteName = title
		} else {
			cfg.SiteName = "JapaConnect"
		}
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
	// In production, set JC_COOKIE_DOMAIN explicitly (e.g. ".example.com").
	if cfg.IsProduction() && cfg.CookieDomain == "" {
		return fmt.Errorf("JC_COOKIE_DOMAIN must be set in production for cross-subdomain session sharing")
	}

	// Without a configured secret the server signs tokens with a per-process
	// random key: sessions break across replicas and restarts, and the key is
	// never rotated or audited.
	if cfg.IsProduction() && len(cfg.JWTSecret) < minJWTSecretLength {
		return fmt.Errorf("JC_JWT_SECRET must be set to at least %d characters in production", minJWTSecretLength)
	}

	return nil
}

// IsProduction returns true when the environment is set to production.
// Use this to gate behavior that should only run in production, such as
// enforcing HTTPS or enabling error reporting.
func (c Config) IsProduction() bool {
	return strings.EqualFold(c.Env, "production")
}

// getEnvWithFallback returns the value of the first env var that is set.
// If primary is empty, returns fallback. If both are empty, returns "".
func getEnvWithFallback(primary, fallback string) string {
	if v := os.Getenv(primary); v != "" {
		return v
	}
	return os.Getenv(fallback)
}

// getEnvInt reads an environment variable as an integer and returns the
// fallback if the variable is empty, unset, or not a valid integer.
func getEnvInt(key string, fallback int) int {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return n
}

// getEnvBool reads an environment variable as a boolean and returns the
// fallback if the variable is empty, unset, or not a valid boolean.
// Accepts "true", "1", "yes" (case-insensitive) as true.
func getEnvBool(key string, fallback bool) bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv(key)))
	if v == "" {
		return fallback
	}
	return v == "true" || v == "1" || v == "yes"
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

// loadDotEnv reads a dotenv-style file and populates the process environment
// for any KEY=VALUE pair whose KEY is not already set. Existing environment
// variables always take precedence, so explicit exports, CI configuration,
// and production deployments are never overridden. Missing files and
// unreadable files are silently ignored — a missing .env must never prevent
// the app from booting.
//
// The parser supports:
//   - KEY=VALUE and KEY="VALUE" (with surrounding quotes stripped)
//   - single-line values containing spaces when quoted
//   - # comments and blank lines
//   - export KEY=VALUE (leading export is stripped)
//
// It does not interpret escapes or multi-line values; .env files in this
// project use plain scalar values, and complex secrets are base64/hex blobs.
func loadDotEnv(path string) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Strip an optional leading "export ".
		line = strings.TrimPrefix(line, "export ")

		eq := strings.IndexByte(line, '=')
		if eq <= 0 {
			continue
		}

		key := strings.TrimSpace(line[:eq])
		value := strings.TrimSpace(line[eq+1:])
		if key == "" {
			continue
		}

		// Strip a single layer of surrounding double quotes so values
		// containing spaces or # are preserved.
		if len(value) >= 2 && value[0] == '"' && value[len(value)-1] == '"' {
			value = value[1 : len(value)-1]
		}

		// Never override an explicitly-set environment variable.
		if _, ok := os.LookupEnv(key); ok {
			continue
		}

		_ = os.Setenv(key, value)
	}
}
