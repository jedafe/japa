package worker

import (
	"os"
	"strconv"
)

// getEnv reads an environment variable and returns the fallback if the
// variable is empty or unset.
func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}

	return fallback
}

// getEnvWithFallback returns the value of the first non-empty env var in
// primary, fallback. If both are empty, returns def. Used to honor a
// canonical env var name while keeping a legacy alias working
// (e.g. JC_VALKEY_ADDR → JC_REDIS_ADDR).
func getEnvWithFallback(primary, fallback, def string) string {
	if v := os.Getenv(primary); v != "" {
		return v
	}
	if v := os.Getenv(fallback); v != "" {
		return v
	}
	return def
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
