package config

import (
	"os"
	"testing"
)

// TestLoad_ReadsEnvironmentVariables verifies that Load picks up every JC_
// environment variable and maps it to the correct Config field.
func TestLoad_ReadsEnvironmentVariables(t *testing.T) {
	t.Setenv("JC_DATABASE_URL", "postgresql://test:test@localhost:5432/testdb?sslmode=disable")
	t.Setenv("JC_REDIS_ADDR", "test-redis:6380")
	t.Setenv("JC_MEILISEARCH_URL", "http://test-meili:7700")
	t.Setenv("JC_MEILISEARCH_KEY", "test-key")
	t.Setenv("JC_GEOIP_DB_PATH", "/data/test.mmdb")
	t.Setenv("JC_MAXMIND_LICENSE_KEY", "test-license")
	t.Setenv("JC_GLITCHTIP_DSN", "https://test-dsn@example.com")
	t.Setenv("JC_IMGPROXY_KEY", "test-imgproxy-key")
	t.Setenv("JC_IMGPROXY_SALT", "test-imgproxy-salt")
	t.Setenv("JC_IMGPROXY_BASE_URL", "http://test-imgproxy:8080")
	t.Setenv("JC_HONCHO_URL", "http://test-honcho:8000")
	t.Setenv("JC_RELEASE_SHA", "abc123")
	t.Setenv("JC_ENV", "production")
	t.Setenv("JC_PORT", "9090")

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
		{"MaxmindLicenseKey", cfg.MaxmindLicenseKey, "test-license"},
		{"GlitchtipDSN", cfg.GlitchtipDSN, "https://test-dsn@example.com"},
		{"ImgproxyKey", cfg.ImgproxyKey, "test-imgproxy-key"},
		{"ImgproxySalt", cfg.ImgproxySalt, "test-imgproxy-salt"},
		{"ImgproxyBaseURL", cfg.ImgproxyBaseURL, "http://test-imgproxy:8080"},
		{"HonchoURL", cfg.HonchoURL, "http://test-honcho:8000"},
		{"ReleaseSHA", cfg.ReleaseSHA, "abc123"},
		{"Env", cfg.Env, "production"},
		{"Port", cfg.Port, "9090"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.want {
				t.Errorf("%s = %q, want %q", tt.name, tt.got, tt.want)
			}
		})
	}
}

// TestLoad_AppliesDefaults verifies that Load fills in development-safe
// defaults when environment variables are not set.
func TestLoad_AppliesDefaults(t *testing.T) {
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
		{"ImgproxyBaseURL default", cfg.ImgproxyBaseURL, "http://localhost:8080"},
		{"HonchoURL default", cfg.HonchoURL, "http://localhost:8000"},
		{"ReleaseSHA default", cfg.ReleaseSHA, "dev"},
		{"Env default", cfg.Env, "development"},
		{"Port default", cfg.Port, "8080"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.want {
				t.Errorf("%s = %q, want %q", tt.name, tt.got, tt.want)
			}
		})
	}
}

// TestIsProduction verifies the IsProduction and IsDevelopment helpers.
func TestIsProduction(t *testing.T) {
	tests := []struct {
		name         string
		env          string
		wantProd     bool
		wantDev      bool
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
				os.Unsetenv("JC_ENV")
			}
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
			got := maskURL(tt.input)
			if got != tt.want {
				t.Errorf("maskURL(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// clearAllJCEnv unsets every JC_ environment variable so tests can verify
// default behavior without interference from the host environment.
func clearAllJCEnv(t *testing.T) {
	t.Helper()
	for _, key := range []string{
		"JC_DATABASE_URL",
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
		"JC_ENV",
		"JC_PORT",
	} {
		os.Unsetenv(key)
	}
}
