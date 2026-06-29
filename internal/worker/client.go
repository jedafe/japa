package worker

import (
	"github.com/hibiken/asynq"
)

// NewClient creates a new Asynq client for enqueuing tasks. The client shares
// the same Valkey connection configuration as the server. Callers must close
// the returned client when done to release connection resources.
func NewClient(cfg *Config) *asynq.Client {
	return asynq.NewClient(asynq.RedisClientOpt{
		Addr:     cfg.RedisAddr,
		Password: cfg.RedisPassword,
		DB:       cfg.RedisDB,
	})
}
