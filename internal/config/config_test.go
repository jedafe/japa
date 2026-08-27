package config

import (
	"math"
	"os"
	"strconv"
	"strings"
	"testing"
)

// TestLoad_ReadsEnvironmentVariables verifies that Load picks up every JC_
// environment variable and maps it to the correct Config field.
func TestLoad_ReadsEnvironmentVariables(t *testing.T) {
	t.Setenv("JC_DATABASE_URL", "postgresql://test:test@localhost:5432/testdb?sslmode=disable")
	t.Setenv("JC_REDIS_ADDR", "test-redis:6380")
	t.Setenv("JC_REDIS_PASSWORD", "test-pass")
	t.Setenv("JC_REDIS_DB", "2")
	t.Setenv("JC_REDIS_POOL_SIZE", "20")
	t.Setenv("JC_REDIS_DIAL_TIMEOUT_MS", "5000")
	t.Setenv("JC_REDIS_READ_TIMEOUT_MS", "3000")
	t.Setenv("JC_REDIS_TLS_ENABLED", "true")
	t.Setenv("JC_MEILISEARCH_URL", "http://test-meili:7700")
	t.Setenv("JC_MEILISEARCH_KEY", "test-key")
	t.Setenv("JC_GEOIP_DB_PATH", "/data/test.mmdb")
	t.Setenv("JC_MAXMIND_ACCOUNT_ID", "test-account")
	t.Setenv("JC_MAXMIND_LICENSE_KEY", "test-license")
	t.Setenv("JC_GLITCHTIP_DSN", "https://test-dsn@example.com")
	t.Setenv("JC_IMGPROXY_KEY", "test-imgproxy-key")
	t.Setenv("JC_IMGPROXY_SALT", "test-imgproxy-salt")
	t.Setenv("JC_IMGPROXY_BASE_URL", "http://test-imgproxy:8080")
	t.Setenv("JC_HONCHO_URL", "http://test-honcho:8000")
	t.Setenv("JC_RELEASE_SHA", "abc123")
	t.Setenv("JC_DEPLOYMENT_VERSION", "1.0.123")
	t.Setenv("JC_ENV", "production")
	t.Setenv("JC_PORT", "9090")
	t.Setenv("JC_JWT_SECRET", "test-jwt-secret-key-32-bytes-long!")
	t.Setenv("JC_JWT_KEY_ID", "key-prod")
	t.Setenv("JC_COOKIE_DOMAIN", ".example.com")
	t.Setenv("JC_ENCRYPTION_KEY", "test-encryption-key-for-production")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load returned unexpected error: %v", err)
	}

	tests := []struct {
		name string
		got  string
		want string
	}{
		{"DatabaseURL", cfg.DatabaseURL, "postgresql://test:test@localhost:5432/testdb?sslmode=disable"},
		{"RedisAddr", cfg.RedisAddr, "test-redis:6380"},
		{"MeilisearchURL", cfg.MeilisearchURL, "http://test-meili:7700"},
		{"MeilisearchKey", cfg.MeilisearchKey, "test-key"},
		{"GeoIPDBPath", cfg.GeoIPDBPath, "/data/test.mmdb"},
		{"MaxmindAccountID", cfg.MaxmindAccountID, "test-account"},
		{"MaxmindLicenseKey", cfg.MaxmindLicenseKey, "test-license"},
		{"GlitchtipDSN", cfg.GlitchtipDSN, "https://test-dsn@example.com"},
		{"ImgproxyKey", cfg.ImgproxyKey, "test-imgproxy-key"},
		{"ImgproxySalt", cfg.ImgproxySalt, "test-imgproxy-salt"},
		{"ImgproxyBaseURL", cfg.ImgproxyBaseURL, "http://test-imgproxy:8080"},
		{"HonchoURL", cfg.HonchoURL, "http://test-honcho:8000"},
		{"ReleaseSHA", cfg.ReleaseSHA, "abc123"},
		{"DeploymentVersion", cfg.DeploymentVersion, "1.0.123"},
		{"Env", cfg.Env, "production"},
		{"Port", cfg.Port, "9090"},
		{"JWTSecret", cfg.JWTSecret, "test-jwt-secret-key-32-bytes-long!"},
		{"JWTKeyID", cfg.JWTKeyID, "key-prod"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if tt.got != tt.want {
				t.Errorf("%s = %q, want %q", tt.name, tt.got, tt.want)
			}
		})
	}

	// Verify non-string Redis config fields.
	if cfg.RedisPassword != "test-pass" {
		t.Errorf("RedisPassword = %q, want %q", cfg.RedisPassword, "test-pass")
	}

	if cfg.RedisDB != 2 {
		t.Errorf("RedisDB = %d, want %d", cfg.RedisDB, 2)
	}

	if cfg.RedisPoolSize != 20 {
		t.Errorf("RedisPoolSize = %d, want %d", cfg.RedisPoolSize, 20)
	}

	if cfg.RedisDialTimeoutMs != 5000 {
		t.Errorf("RedisDialTimeoutMs = %d, want %d", cfg.RedisDialTimeoutMs, 5000)
	}

	if cfg.RedisReadTimeoutMs != 3000 {
		t.Errorf("RedisReadTimeoutMs = %d, want %d", cfg.RedisReadTimeoutMs, 3000)
	}

	if !cfg.RedisTLSEnabled {
		t.Errorf("RedisTLSEnabled = false, want true")
	}
}

// TestLoad_AppliesDefaults verifies that Load fills in development-safe
// defaults when environment variables are not set.
func TestLoad_AppliesDefaults(t *testing.T) {
	t.Parallel()
	// Clear all JC_ environment variables so defaults kick in.
	clearAllJCEnv(t)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load returned unexpected error: %v", err)
	}

	tests := []struct {
		name string
		got  string
		want string
	}{
		{"DatabaseURL default", cfg.DatabaseURL, "postgresql://japa:japa@localhost:5432/japa?sslmode=disable"},
		{"RedisAddr default", cfg.RedisAddr, "localhost:6379"},
		{"MeilisearchURL default", cfg.MeilisearchURL, "http://localhost:7700"},
		{"ImgproxyBaseURL default", cfg.ImgproxyBaseURL, "http://localhost:8099"},
		{"HonchoURL default", cfg.HonchoURL, "http://localhost:8001"},
		{"ReleaseSHA default", cfg.ReleaseSHA, "dev"},
		{"Env default", cfg.Env, "development"},
		{"Port default", cfg.Port, "8080"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if tt.got != tt.want {
				t.Errorf("%s = %q, want %q", tt.name, tt.got, tt.want)
			}
		})
	}
}

// TestLoad_JWTSecretDefaults verifies that JWTSecret defaults to empty string
// (random key in dev) and JWTKeyID defaults to "key-dev".
func TestLoad_JWTSecretDefaults(t *testing.T) {
	t.Parallel()
	// Clear all JC_ environment variables so defaults kick in.
	clearAllJCEnv(t)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load returned unexpected error: %v", err)
	}

	if cfg.JWTSecret != "" {
		t.Errorf("JWTSecret = %q, want empty string", cfg.JWTSecret)
	}

	if cfg.JWTKeyID != "key-dev" {
		t.Errorf("JWTKeyID = %q, want %q", cfg.JWTKeyID, "key-dev")
	}
}

// TestIsProduction verifies the IsProduction and IsDevelopment helpers.
func TestIsProduction(t *testing.T) {
	tests := []struct {
		name     string
		env      string
		wantProd bool
		wantDev  bool
	}{
		{"production lowercase", "production", true, false},
		{"production uppercase", "PRODUCTION", true, false},
		{"development", "development", false, true},
		{"empty string defaults to dev", "", false, true},
		{"random value is not production", "staging", false, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.env != "" {
				t.Setenv("JC_ENV", tt.env)
			} else {
				if err := os.Unsetenv("JC_ENV"); err != nil {
					t.Fatalf("unset JC_ENV: %v", err)
				}
			}
			// Production validation requires JC_COOKIE_DOMAIN, JC_DATABASE_URL,
			// JC_JWT_SECRET, and JC_ENCRYPTION_KEY.
			t.Setenv("JC_COOKIE_DOMAIN", ".example.com")
			t.Setenv("JC_DATABASE_URL", "postgresql://test:test@localhost:5432/testdb?sslmode=disable")
			t.Setenv("JC_JWT_SECRET", strings.Repeat("x", 32))
			t.Setenv("JC_ENCRYPTION_KEY", "test-encryption-key")

			cfg, err := Load()
			if err != nil {
				t.Fatalf("Load returned unexpected error: %v", err)
			}

			if cfg.IsProduction() != tt.wantProd {
				t.Errorf("IsProduction() = %v, want %v", cfg.IsProduction(), tt.wantProd)
			}

			if cfg.IsDevelopment() != tt.wantDev {
				t.Errorf("IsDevelopment() = %v, want %v", cfg.IsDevelopment(), tt.wantDev)
			}
		})
	}
}

// TestMaskURL verifies that credentials in connection strings are hidden.
func TestMaskURL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"URL with credentials", "postgresql://user:pass@host:5432/db", "postgresql://***@host:5432/db"},
		{"URL without credentials", "postgresql://host:5432/db", "postgresql://host:5432/db"},
		{"URL without scheme", "host:5432/db", "host:5432/db"},
		{"empty string", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := maskURL(tt.input)
			if got != tt.want {
				t.Errorf("maskURL(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// TestGetEnvInt32Bounds verifies that out-of-range or negative env values for
// 32-bit integer fields fall back to the default instead of wrapping.
func TestGetEnvInt32Bounds(t *testing.T) {
	t.Setenv("JC_DB_MAX_CONNS", strconv.FormatInt(int64(math.MaxInt32)+1, 10))
	t.Setenv("JC_DB_MIN_CONNS", "-5")

	if got := getEnvInt32("JC_DB_MAX_CONNS", 0); got != 0 {
		t.Errorf("getEnvInt32 overflow should fall back to 0, got %d", got)
	}

	if got := getEnvInt32("JC_DB_MIN_CONNS", 0); got != 0 {
		t.Errorf("getEnvInt32 negative should fall back to 0, got %d", got)
	}
}

// clearAllJCEnv unsets every JC_ environment variable so tests can verify
// default behavior without interference from the host environment.
func clearAllJCEnv(t *testing.T) {
	t.Helper()

	for _, key := range []string{
		"JC_DATABASE_URL",
		"JC_DATABASE_READER_URL",
		"JC_REDIS_ADDR",
		"JC_MEILISEARCH_URL",
		"JC_MEILISEARCH_KEY",
		"JC_GEOIP_DB_PATH",
		"JC_MAXMIND_LICENSE_KEY",
		"JC_GLITCHTIP_DSN",
		"JC_IMGPROXY_KEY",
		"JC_IMGPROXY_SALT",
		"JC_IMGPROXY_BASE_URL",
		"JC_HONCHO_URL",
		"JC_RELEASE_SHA",
		"JC_DEPLOYMENT_VERSION",
		"JC_ENV",
		"JC_PORT",
		"JC_JWT_SECRET",
		"JC_JWT_KEY_ID",
		"JC_COOKIE_DOMAIN",
		"JC_XIAOMI_API_KEY",
		"JC_ENCRYPTION_KEY",
		"JC_STOCK_ENCRYPTION_KEY",
		"JC_TOTP_ENCRYPTION_KEY",
	} {
		if err := os.Unsetenv(key); err != nil {
			t.Fatalf("clearAllJCEnv: unset %s: %v", key, err)
		}
	}
}

// TestLoad_ProductionRequiresEncryptionKey verifies that JC_ENCRYPTION_KEY
// (or a legacy alias) is mandatory in production — without it, secrets at
// rest cannot be encrypted and the server must fail fast.
func TestLoad_ProductionRequiresEncryptionKey(t *testing.T) {
	t.Setenv("JC_ENV", "production")
	t.Setenv("JC_DATABASE_URL", "postgresql://test:test@localhost:5432/testdb?sslmode=disable")
	t.Setenv("JC_COOKIE_DOMAIN", ".example.com")
	t.Setenv("JC_JWT_SECRET", strings.Repeat("x", 32))

	if err := os.Unsetenv("JC_ENCRYPTION_KEY"); err != nil {
		t.Fatalf("unset JC_ENCRYPTION_KEY: %v", err)
	}

	if err := os.Unsetenv("JC_STOCK_ENCRYPTION_KEY"); err != nil {
		t.Fatalf("unset JC_STOCK_ENCRYPTION_KEY: %v", err)
	}

	if err := os.Unsetenv("JC_TOTP_ENCRYPTION_KEY"); err != nil {
		t.Fatalf("unset JC_TOTP_ENCRYPTION_KEY: %v", err)
	}

	_, err := Load()
	if err == nil {
		t.Fatal("Load should fail in production without JC_ENCRYPTION_KEY")
	}

	if !strings.Contains(err.Error(), "JC_ENCRYPTION_KEY") {
		t.Errorf("error should mention JC_ENCRYPTION_KEY, got: %v", err)
	}
}

// TestLoad_ProductionAcceptsLegacyEncryptionKey verifies that the legacy
// JC_TOTP_ENCRYPTION_KEY alias still satisfies the production check.
func TestLoad_ProductionAcceptsLegacyEncryptionKey(t *testing.T) {
	t.Setenv("JC_ENV", "production")
	t.Setenv("JC_DATABASE_URL", "postgresql://test:test@localhost:5432/testdb?sslmode=disable")
	t.Setenv("JC_COOKIE_DOMAIN", ".example.com")
	t.Setenv("JC_JWT_SECRET", strings.Repeat("x", 32))

	if err := os.Unsetenv("JC_ENCRYPTION_KEY"); err != nil {
		t.Fatalf("unset JC_ENCRYPTION_KEY: %v", err)
	}

	if err := os.Unsetenv("JC_STOCK_ENCRYPTION_KEY"); err != nil {
		t.Fatalf("unset JC_STOCK_ENCRYPTION_KEY: %v", err)
	}

	t.Setenv("JC_TOTP_ENCRYPTION_KEY", "legacy-totp-key")

	if _, err := Load(); err != nil {
		t.Fatalf("Load should succeed with legacy JC_TOTP_ENCRYPTION_KEY, got: %v", err)
	}
}
