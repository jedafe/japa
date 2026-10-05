// Package config loads all JapaConnect configuration from environment variables.
//
// Every environment variable uses the JC_ prefix so that JapaConnect settings
// are easy to distinguish from system or third-party variables in any
// deployment environment. The Load function reads from os.Getenv and applies
// sensible defaults for development so the app boots without a full .env file.
package config

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// Config holds every configuration value the application needs at startup.
// Fields are intentionally flat — no nested structs — so it is easy to read
// from environment variables and easy to inspect in logs or debug output.
type Config struct {
	DatabaseURL              string // JC_DATABASE_URL — pgx connection string for PostgreSQL (writer/primary)
	DatabaseReaderURL        string // JC_DATABASE_READER_URL — pgx connection string for read replica (empty = use DatabaseURL for reads too)
	DBMaxConns               int32  // JC_DB_MAX_CONNS — max connections per pgxpool (0 = default 25)
	DBMinConns               int32  // JC_DB_MIN_CONNS — min connections per pgxpool (0 = default 5)
	DBMaxConnLifetimeMs      int    // JC_DB_MAX_CONN_LIFETIME_MS — max connection lifetime (0 = default 30m)
	DBMaxConnIdleTimeMs      int    // JC_DB_MAX_CONN_IDLE_TIME_MS — max idle connection lifetime (0 = default 5m)
	RedisAddr                string // JC_VALKEY_ADDR (canonical, per JEOS stack.yaml) → JC_REDIS_ADDR (legacy alias) — Valkey/Redis address for cache and Asynq queues
	RedisPassword            string // JC_REDIS_PASSWORD — optional auth password for Valkey/Redis
	RedisDB                  int    // JC_REDIS_DB — Valkey/Redis database number (0 default)
	RedisPoolSize            int    // JC_REDIS_POOL_SIZE — connection pool size (0 = valkey-go default)
	RedisDialTimeoutMs       int    // JC_REDIS_DIAL_TIMEOUT_MS — dial timeout in milliseconds (0 = default)
	RedisReadTimeoutMs       int    // JC_REDIS_READ_TIMEOUT_MS — read/write timeout in milliseconds (0 = default)
	RedisTLSEnabled          bool   // JC_REDIS_TLS_ENABLED — enable TLS connection to Valkey/Redis
	MeilisearchURL           string // JC_MEILISEARCH_URL — Meilisearch HTTP endpoint
	MeilisearchKey           string // JC_MEILISEARCH_KEY — Meilisearch master key (empty for dev)
	GeoIPDBPath              string // JC_GEOIP_DB_PATH — path to the MaxMind GeoLite2 database file
	MaxmindAccountID         string // JC_MAXMIND_ACCOUNT_ID — MaxMind account ID (Basic Auth username for DB downloads)
	MaxmindLicenseKey        string // JC_MAXMIND_LICENSE_KEY — license key for downloading GeoIP databases
	GlitchtipDSN             string // JC_GLITCHTIP_DSN — GlitchTip DSN for error reporting
	ImgproxyKey              string // JC_IMGPROXY_KEY — signing key for imgproxy URL generation
	ImgproxySalt             string // JC_IMGPROXY_SALT — salt for imgproxy URL signing
	ImgproxyBaseURL          string // JC_IMGPROXY_BASE_URL — public-facing base URL for the imgproxy service
	ImgproxyInternalURL      string // JC_IMGPROXY_INTERNAL_URL — internal Docker-network URL for imgproxy (e.g. http://imgproxy:8099); empty = use ImgproxyBaseURL
	S3Endpoint               string // JC_S3_ENDPOINT — S3-compatible object storage endpoint (SeaweedFS or compatible)
	S3AccessKey              string // JC_S3_ACCESS_KEY — S3 access key
	S3SecretKey              string // JC_S3_SECRET_KEY — S3 secret key
	S3Bucket                 string // JC_S3_BUCKET — S3 bucket name for media storage
	S3Region                 string // JC_S3_REGION — S3 region (placeholder; SeaweedFS ignores this)
	MediaRoot                string // JC_MEDIA_ROOT — local media storage root (default "data/media"; absolute paths recommended)
	VAPIDPublicKey           string // JC_VAPID_PUBLIC_KEY — VAPID public key for WebPush notifications
	VAPIDPrivateKey          string // JC_VAPID_PRIVATE_KEY — VAPID private key for WebPush notifications
	VAPIDSubject             string // JC_VAPID_SUBJECT — VAPID subject (mailto: or https: URL) for WebPush
	HonchoURL                string // JC_HONCHO_URL — Honcho agent memory service URL
	ClamAVAddr               string // JC_CLAMAV_ADDR — ClamAV daemon address (host:port), default localhost:3310
	ReleaseSHA               string // JC_RELEASE_SHA — git commit SHA, used for release tracking
	DeploymentVersion        string // JC_DEPLOYMENT_VERSION — optional override for the deployment version reported by the control panel
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
	MCPPublicURL             string // JC_MCP_PUBLIC_URL — public base URL of the API host serving MCP/OAuth (e.g. https://api.example.com); §120 issuer
	EncryptionKey            string // JC_ENCRYPTION_KEY — AES-256 key for encrypting secrets at rest (TOTP, API keys, api_config). JEOS canonical name.
	TOTPEncryptionKey        string // JC_TOTP_ENCRYPTION_KEY — legacy alias for JC_ENCRYPTION_KEY (kept for backward compatibility)
	StripeSecretKey          string // JC_STRIPE_SECRET_KEY — Stripe API secret key (sk_live_... / sk_test_...)
	StripeWebhookSecret      string // JC_STRIPE_WEBHOOK_SECRET — Stripe webhook signing secret (whsec_...)
	NOWPaymentsIPNSecret     string // JC_NOWPAYMENTS_IPN_SECRET — NOWPayments IPN HMAC secret
	NOWPaymentsAPIKey        string // JC_NOWPAYMENTS_API_KEY — NOWPayments API key for invoice and payout calls
	PaystackSecretKey        string // JC_PAYSTACK_SECRET_KEY — Paystack webhook HMAC secret
	XiaomiAPIKey             string // JC_XIAOMI_API_KEY — Xiaomi MiMo API key (tp- for Token Plan, sk- for Pay-as-you-go)
	CORSOrigins              string // JC_CORS_ORIGINS — comma-separated allowed CORS origins for production
	CookieDomain             string // JC_COOKIE_DOMAIN — cookie domain for cross-subdomain session sharing (e.g. ".example.com")
	InternalToken            string // JC_INTERNAL_TOKEN — shared secret for internal service-to-service endpoints (render helpers)
	UnsplashAccessKey        string // JC_UNSPLASH_ACCESS_KEY — Unsplash API access key
	PexelsAPIKey             string // JC_PEXELS_API_KEY — Pexels API key
	PixabayAPIKey            string // JC_PIXABAY_API_KEY — Pixabay API key
	StockCacheTTLSec         int    // JC_STOCK_CACHE_TTL_SECONDS — search result cache TTL (default 300)
	StockCacheMaxEntries     int    // JC_STOCK_CACHE_MAX_ENTRIES — max cached search pages (default 1000)
	StockHTTPTimeoutSec      int    // JC_STOCK_HTTP_TIMEOUT_SECONDS — provider HTTP client timeout (default 15)
}

// Load reads all JC_ environment variables and returns a populated Config.
// Missing variables fall back to development-safe defaults so the server can
// boot locally without a complete .env file. In production, required values
// (database URL, cookie domain, JWT secret, and encryption key) are validated
// and an error is returned if any are missing or too short.
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
		loadDotEnv()
	}

	cfg := Config{
		DatabaseURL:              os.Getenv("JC_DATABASE_URL"),
		DatabaseReaderURL:        os.Getenv("JC_DATABASE_READER_URL"),
		DBMaxConns:               getEnvInt32("JC_DB_MAX_CONNS", 0),
		DBMinConns:               getEnvInt32("JC_DB_MIN_CONNS", 0),
		DBMaxConnLifetimeMs:      getEnvInt("JC_DB_MAX_CONN_LIFETIME_MS", 0),
		DBMaxConnIdleTimeMs:      getEnvInt("JC_DB_MAX_CONN_IDLE_TIME_MS", 0),
		RedisAddr:                getEnvWithFallback("JC_VALKEY_ADDR", "JC_REDIS_ADDR"),
		RedisPassword:            os.Getenv("JC_REDIS_PASSWORD"),
		RedisDB:                  getEnvInt("JC_REDIS_DB", 0),
		RedisPoolSize:            getEnvInt("JC_REDIS_POOL_SIZE", 0),
		RedisDialTimeoutMs:       getEnvInt("JC_REDIS_DIAL_TIMEOUT_MS", 0),
		RedisReadTimeoutMs:       getEnvInt("JC_REDIS_READ_TIMEOUT_MS", 0),
		RedisTLSEnabled:          getEnvBool("JC_REDIS_TLS_ENABLED", false),
		MeilisearchURL:           os.Getenv("JC_MEILISEARCH_URL"),
		MeilisearchKey:           os.Getenv("JC_MEILISEARCH_KEY"),
		GeoIPDBPath:              os.Getenv("JC_GEOIP_DB_PATH"),
		MaxmindAccountID:         os.Getenv("JC_MAXMIND_ACCOUNT_ID"),
		MaxmindLicenseKey:        os.Getenv("JC_MAXMIND_LICENSE_KEY"),
		GlitchtipDSN:             os.Getenv("JC_GLITCHTIP_DSN"),
		ImgproxyKey:              os.Getenv("JC_IMGPROXY_KEY"),
		ImgproxySalt:             os.Getenv("JC_IMGPROXY_SALT"),
		ImgproxyBaseURL:          os.Getenv("JC_IMGPROXY_BASE_URL"),
		ImgproxyInternalURL:      os.Getenv("JC_IMGPROXY_INTERNAL_URL"),
		S3Endpoint:               os.Getenv("JC_S3_ENDPOINT"),
		S3AccessKey:              os.Getenv("JC_S3_ACCESS_KEY"),
		S3SecretKey:              os.Getenv("JC_S3_SECRET_KEY"),
		S3Bucket:                 os.Getenv("JC_S3_BUCKET"),
		S3Region:                 os.Getenv("JC_S3_REGION"),
		MediaRoot:                os.Getenv("JC_MEDIA_ROOT"),
		VAPIDPublicKey:           os.Getenv("JC_VAPID_PUBLIC_KEY"),
		VAPIDPrivateKey:          os.Getenv("JC_VAPID_PRIVATE_KEY"),
		VAPIDSubject:             os.Getenv("JC_VAPID_SUBJECT"),
		HonchoURL:                os.Getenv("JC_HONCHO_URL"),
		ClamAVAddr:               os.Getenv("JC_CLAMAV_ADDR"),
		ReleaseSHA:               os.Getenv("JC_RELEASE_SHA"),
		DeploymentVersion:        os.Getenv("JC_DEPLOYMENT_VERSION"),
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
		MCPPublicURL:             os.Getenv("JC_MCP_PUBLIC_URL"),
		EncryptionKey:            os.Getenv("JC_ENCRYPTION_KEY"),
		TOTPEncryptionKey:        os.Getenv("JC_TOTP_ENCRYPTION_KEY"),
		StripeSecretKey:          os.Getenv("JC_STRIPE_SECRET_KEY"),
		StripeWebhookSecret:      os.Getenv("JC_STRIPE_WEBHOOK_SECRET"),
		NOWPaymentsIPNSecret:     os.Getenv("JC_NOWPAYMENTS_IPN_SECRET"),
		NOWPaymentsAPIKey:        os.Getenv("JC_NOWPAYMENTS_API_KEY"),
		PaystackSecretKey:        os.Getenv("JC_PAYSTACK_SECRET_KEY"),
		XiaomiAPIKey:             os.Getenv("JC_XIAOMI_API_KEY"),
		CORSOrigins:              os.Getenv("JC_CORS_ORIGINS"),
		CookieDomain:             os.Getenv("JC_COOKIE_DOMAIN"),
		InternalToken:            os.Getenv("JC_INTERNAL_TOKEN"),
		UnsplashAccessKey:        os.Getenv("JC_UNSPLASH_ACCESS_KEY"),
		PexelsAPIKey:             os.Getenv("JC_PEXELS_API_KEY"),
		PixabayAPIKey:            os.Getenv("JC_PIXABAY_API_KEY"),
		StockCacheTTLSec:         getEnvInt("JC_STOCK_CACHE_TTL_SECONDS", 300),
		StockCacheMaxEntries:     getEnvInt("JC_STOCK_CACHE_MAX_ENTRIES", 1000),
		StockHTTPTimeoutSec:      getEnvInt("JC_STOCK_HTTP_TIMEOUT_SECONDS", 15),
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
			return errors.New("JC_DATABASE_URL is required in production")
		}

		cfg.DatabaseURL = "postgresql://japa:japa@localhost:5432/japa?sslmode=disable"
	}

	setStringIfEmpty(&cfg.RedisAddr, "localhost:6379")
	setStringIfEmpty(&cfg.MeilisearchURL, "http://localhost:7700")
	setStringIfEmpty(&cfg.ImgproxyBaseURL, "http://localhost:8099")

	// When no internal imgproxy URL is configured, route internal requests
	// through the public base URL so callers can use ImgproxyInternalURL
	// unconditionally without a nil/empty check.
	if cfg.ImgproxyInternalURL == "" {
		cfg.ImgproxyInternalURL = cfg.ImgproxyBaseURL
	}

	setStringIfEmpty(&cfg.HonchoURL, "http://localhost:8001")
	setStringIfEmpty(&cfg.ClamAVAddr, "localhost:3310")
	setStringIfEmpty(&cfg.ReleaseSHA, "dev")
	setStringIfEmpty(&cfg.Env, "development")
	setStringIfEmpty(&cfg.Port, "8080")
	setStringIfEmpty(&cfg.JWTKeyID, "key-dev")
	setStringIfEmpty(&cfg.SiteName, "JapaConnect")
	setStringIfEmpty(&cfg.SiteURL, "http://localhost:8080")
	setStringIfEmpty(&cfg.SiteDescription, "A community platform for the diaspora and people planning to relocate abroad")
	setStringIfEmpty(&cfg.OAuthRedirectBase, "http://localhost:8080")

	// §120: the OAuth issuer/MCP public URL is the API host. Fall back to the
	// OAuth redirect base (same host in practice), then the site URL.
	setStringIfEmpty(&cfg.MCPPublicURL, cfg.OAuthRedirectBase)
	setStringIfEmpty(&cfg.MCPPublicURL, cfg.SiteURL)

	// CookieDomain defaults to empty in development (cookie scoped to exact host).
	// In production, set JC_COOKIE_DOMAIN explicitly (e.g. ".example.com").
	if cfg.IsProduction() && cfg.CookieDomain == "" {
		return errors.New("JC_COOKIE_DOMAIN must be set in production for cross-subdomain session sharing")
	}

	// Without a configured secret the server signs tokens with a per-process
	// random key: sessions break across replicas and restarts, and the key is
	// never rotated or audited.
	if cfg.IsProduction() && len(cfg.JWTSecret) < minJWTSecretLength {
		return fmt.Errorf("JC_JWT_SECRET must be set to at least %d characters in production", minJWTSecretLength)
	}

	// The encryption key protects TOTP secrets, API keys, and api_config at
	// rest (AES-256-GCM via internal/crypto). Without it, the crypto package
	// falls back to storing secrets in cleartext — acceptable in development
	// but a hard fail-fast requirement in production. Priority:
	// JC_ENCRYPTION_KEY → JC_STOCK_ENCRYPTION_KEY → JC_TOTP_ENCRYPTION_KEY.
	if cfg.IsProduction() && cfg.EncryptionKey == "" && os.Getenv("JC_STOCK_ENCRYPTION_KEY") == "" && cfg.TOTPEncryptionKey == "" {
		return errors.New("JC_ENCRYPTION_KEY must be set in production (secrets at rest cannot be encrypted without it)")
	}

	return nil
}

func setStringIfEmpty(value *string, fallback string) {
	if *value == "" {
		*value = fallback
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

// getEnvInt32 reads an environment variable as a signed 32-bit integer and
// returns the fallback if the variable is empty, unset, not a valid integer,
// or outside the int32 range. Negative values are treated as invalid and fall
// back to the default.
func getEnvInt32(key string, fallback int32) int32 {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}

	n, err := strconv.ParseInt(v, 10, 32)
	if err != nil || n < 0 {
		return fallback
	}

	return int32(n)
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

// String returns a human-readable summary of the config for logging at startup.
// Sensitive values (keys, DSNs, passwords) are masked so they never appear in
// logs in cleartext.
func (c Config) String() string {
	return fmt.Sprintf(
		"Config{Env=%s, Port=%s, DatabaseURL=%s, DatabaseReaderURL=%s, RedisAddr=%s, MeilisearchURL=%s, ReleaseSHA=%s}",
		c.Env,
		c.Port,
		maskURL(c.DatabaseURL),
		maskURL(c.DatabaseReaderURL),
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

// loadDotEnv reads a .env file from the working directory and populates the
// process environment for any KEY=VALUE pair whose KEY is not already set.
// Existing environment variables always take precedence, so explicit exports,
// CI configuration, and production deployments are never overridden. Missing
// files and unreadable files are silently ignored — a missing .env must never
// prevent the app from booting.
//
// The parser supports:
//   - KEY=VALUE and KEY="VALUE" (with surrounding quotes stripped)
//   - single-line values containing spaces when quoted
//   - # comments and blank lines
//   - export KEY=VALUE (leading export is stripped)
//
// It does not interpret escapes or multi-line values; .env files in this
// project use plain scalar values, and complex secrets are base64/hex blobs.
func loadDotEnv() {
	f, err := os.Open(".env")
	if err != nil {
		return
	}

	defer func() { _ = f.Close() }()

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
