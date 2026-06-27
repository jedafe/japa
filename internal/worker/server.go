package worker

import (
	"fmt"

	"github.com/hibiken/asynq"
)

// NewServer creates a new Asynq server configured with all three task tiers.
// The returned server is ready to have handlers registered via NewMux and then
// be started with srv.Run().
//
// The default retry policy uses exponential backoff (asynq.DefaultRetryDelayFunc)
// with up to 25 retries per task. Individual tasks can override MaxRetry via
// the asynq.MaxRetry option when enqueued.
func NewServer(cfg *Config) *asynq.Server {
	return asynq.NewServer(
		asynq.RedisClientOpt{
			Addr:     cfg.RedisAddr,
			Password: cfg.RedisPassword,
			DB:       cfg.RedisDB,
		},
		asynq.Config{
			Queues: map[string]int{
				QueueCritical: cfg.CriticalConcurrency,
				QueueDefault:  cfg.DefaultConcurrency,
				QueueLow:      cfg.LowConcurrency,
			},
			// DefaultRetryDelayFunc implements exponential backoff.
			RetryDelayFunc: asynq.DefaultRetryDelayFunc,
		},
	)
}

// NewMux creates a new Asynq ServeMux and registers all task handlers.
func NewMux() *asynq.ServeMux {
	mux := asynq.NewServeMux()
	RegisterHandlers(mux)
	return mux
}

// Addr returns the Redis address from the config as a formatted string.
// Useful for logging at startup.
func Addr(cfg *Config) string {
	return fmt.Sprintf("%s (db=%d)", cfg.RedisAddr, cfg.RedisDB)
}
