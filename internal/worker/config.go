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

import (
	"os"
	"strings"
)

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
	RedisAddr     string // JC_VALKEY_ADDR (canonical) → JC_REDIS_ADDR (legacy alias) — host:port of the Valkey/Redis instance
	RedisPassword string // JC_REDIS_PASSWORD — optional auth password
	RedisDB       int    // JC_REDIS_DB — database number (0 default)

	// Per-tier concurrency limits.
	CriticalConcurrency int // JC_WORKER_CRITICAL_CONCURRENCY
	DefaultConcurrency  int // JC_WORKER_DEFAULT_CONCURRENCY
	LowConcurrency      int // JC_WORKER_LOW_CONCURRENCY

	// SchedulerCronSpecs allows overriding cron specs per task type via
	// environment variables. Key is the task type string, value is the
	// cron spec. Populated from JC_SCHEDULER_CRON_<TASK_TYPE> env vars
	// where <TASK_TYPE> has non-alphanumeric characters replaced with _.
	SchedulerCronSpecs map[string]string
}

// DefaultConfig returns a Config populated from environment variables with
// development-safe defaults. Callers that need production values must set
// the corresponding JC_ variables before calling this function.
func DefaultConfig() *Config {
	return &Config{
		RedisAddr:           getEnvWithFallback("JC_VALKEY_ADDR", "JC_REDIS_ADDR", "localhost:6379"),
		RedisPassword:       getEnv("JC_REDIS_PASSWORD", ""),
		RedisDB:             getEnvInt("JC_REDIS_DB", 0),
		CriticalConcurrency: getEnvInt("JC_WORKER_CRITICAL_CONCURRENCY", 10),
		DefaultConcurrency:  getEnvInt("JC_WORKER_DEFAULT_CONCURRENCY", 5),
		LowConcurrency:      getEnvInt("JC_WORKER_LOW_CONCURRENCY", 2),
		SchedulerCronSpecs:  loadSchedulerCronSpecs(),
	}
}

// loadSchedulerCronSpecs scans environment variables for JC_SCHEDULER_CRON_*
// and returns a map of task type → cron spec. The env var suffix is converted
// back to a task type by replacing underscores with the original separators.
// Example: JC_SCHEDULER_CRON_CONTENT_PUBLISH_SWEEP=0 */5 * * * *
// maps to task type "content:publish_sweep".
func loadSchedulerCronSpecs() map[string]string {
	specs := make(map[string]string)

	for _, env := range os.Environ() {
		if !strings.HasPrefix(env, "JC_SCHEDULER_CRON_") {
			continue
		}

		pair := strings.SplitN(env, "=", 2)
		if len(pair) != 2 {
			continue
		}

		key := strings.TrimPrefix(pair[0], "JC_SCHEDULER_CRON_")
		value := pair[1]

		if key == "" || value == "" {
			continue
		}
		// Convert env suffix back to task type: replace _ with :
		// Handle double-underscore as literal underscore in task type
		taskType := strings.ReplaceAll(key, "__", "\x00")
		taskType = strings.ReplaceAll(taskType, "_", ":")
		taskType = strings.ReplaceAll(taskType, "\x00", "_")
		specs[taskType] = value
	}

	return specs
}
