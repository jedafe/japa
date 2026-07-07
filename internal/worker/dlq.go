package worker

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/hibiken/asynq"
)

// DLQStats holds dead letter queue statistics.
type DLQStats struct {
	Depth       int       `json:"depth"`
	CheckedAt   time.Time `json:"checked_at"`
	AlertActive bool      `json:"alert_active"`
}

// HandleQueueStats returns Asynq queue statistics for the control panel.
// GET /api/v1/control/queue/stats
func HandleQueueStats(inspector *asynq.Inspector) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		stats := map[string]any{}

		// Get queue info for each tier.
		for _, queueName := range []string{QueueCritical, QueueDefault, QueueLow} {
			qi, err := inspector.GetQueueInfo(queueName)
			if err != nil {
				slog.Warn("dlq: failed to get queue info", "queue", queueName, "error", err)
				continue
			}
			stats[queueName] = map[string]any{
				"size":            qi.Size,
				"active":          qi.Active,
				"pending":         qi.Pending,
				"scheduled":       qi.Scheduled,
				"completed":       qi.Completed,
				"failed":          qi.Failed,
				"paused":          qi.Paused,
				"latency_seconds": qi.Latency.Seconds(),
			}
		}

		// DLQ depth = sum of failed tasks across all queues.
		dlqDepth := 0
		for _, queueName := range []string{QueueCritical, QueueDefault, QueueLow} {
			qi, err := inspector.GetQueueInfo(queueName)
			if err != nil {
				continue
			}
			dlqDepth += qi.Failed
		}

		stats["dlq"] = DLQStats{
			Depth:       dlqDepth,
			CheckedAt:   time.Now().UTC(),
			AlertActive: dlqDepth > 0,
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]any{"data": stats}); err != nil {
			slog.Error("dlq: failed to encode response", "error", err)
		}
	}
}
