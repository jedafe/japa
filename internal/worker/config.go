// Package worker provides the Asynq background job queue for JapaConnect.
//
// The worker package defines three task tiers (critical, default, low) with
// independent concurrency limits, registers all V1 task handlers, and exposes
// constructors for the Asynq server and client used by cmd/api and other
// packages that enqueue background work.
//
// Configuration is read from JC_ environment variables and follows the same
// graceful-fallback pattern as the rest of the codebase: missing values get
// development-safe defaults so the application boots locally without a full
// .env file.
package worker

// Queue tier constants. The names map directly to Asynq queue keys and are
// used both when enqueuing tasks and when configuring the server's per-queue
// concurrency.
const (
	QueueCritical = "critical"
	QueueDefault  = "default"
	QueueLow      = "low"
)

// Config holds the configuration needed to create Asynq server and client
// instances. All fields map to JC_ environment variables.
type Config struct {
	RedisAddr     string // JC_REDIS_ADDR — host:port of the Valkey/Redis instance
	RedisPassword string // JC_REDIS_PASSWORD — optional auth password
	RedisDB       int    // JC_REDIS_DB — database number (0 default)

	// Per-tier concurrency limits.
	CriticalConcurrency int // JC_WORKER_CRITICAL_CONCURRENCY
	DefaultConcurrency  int // JC_WORKER_DEFAULT_CONCURRENCY
	LowConcurrency      int // JC_WORKER_LOW_CONCURRENCY
}

// DefaultConfig returns a Config populated from environment variables with
// development-safe defaults. Callers that need production values must set
// the corresponding JC_ variables before calling this function.
func DefaultConfig() *Config {
	return &Config{
		RedisAddr:           getEnv("JC_REDIS_ADDR", "localhost:6379"),
		RedisPassword:       getEnv("JC_REDIS_PASSWORD", ""),
		RedisDB:             getEnvInt("JC_REDIS_DB", 0),
		CriticalConcurrency: getEnvInt("JC_WORKER_CRITICAL_CONCURRENCY", 10),
		DefaultConcurrency:  getEnvInt("JC_WORKER_DEFAULT_CONCURRENCY", 5),
		LowConcurrency:      getEnvInt("JC_WORKER_LOW_CONCURRENCY", 2),
	}
}
