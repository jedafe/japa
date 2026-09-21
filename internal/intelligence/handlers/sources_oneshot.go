package handlers

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/hibiken/asynq"

	errpkg "github.com/jedafe/japa/internal/errors"
)

// oneshotRequest is the POST /api/v1/sources/oneshot body — a user-submitted
// URL to fetch once (share-target/bookmarklet; intel-channels Phase 2,
// spec §3). categories optionally hint entity types for the pipeline.
type oneshotRequest struct {
	URL        string   `json:"url"`
	Categories []string `json:"categories"`
}

// HandleOneshotSource records the URL as a oneshot channel on the shared
// user-submissions source and enqueues a single intel:fetch_channel — the
// channel is never scheduled (ListDueChannels excludes oneshot). The URL
// is validated like a probe target (https assumed, private hosts rejected)
// and shares the per-user probe quota — it is the same "fetch a URL a user
// pointed at" action class.
//
// POST /api/v1/sources/oneshot — authenticated.
// Response: 202 {channel_id, source_id, status: "queued", created: bool}.
func (h *Handlers) HandleOneshotSource(w http.ResponseWriter, r *http.Request) {
	if h.intelSources == nil || h.asynqClient == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "intel_unavailable", "Intelligence system is not configured")
		return
	}

	userID, ok := requireUser(w, r)
	if !ok {
		return
	}

	var req oneshotRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	// Per-user hourly quota — same jc:intel:probe_quota budget as the
	// preview probe (a oneshot is a probe-adjacent outbound fetch).
	if h.rdb != nil {
		limit := defaultProbeRateLimit
		if h.settings != nil {
			limit = h.settings.GetIntOr(r.Context(), "intel.probe.rate_limit_per_hr", defaultProbeRateLimit)
		}

		key := "jc:intel:probe_quota:" + userID.String()
		count, incrErr := h.rdb.Incr(r.Context(), key).Result()
		if incrErr != nil {
			slog.Warn("intel: oneshot quota increment failed; allowing request", "user_id", userID.String(), "error", incrErr)
		} else {
			if count == 1 {
				h.rdb.Expire(r.Context(), key, probeQuotaWindow)
			}
			if count > int64(limit) {
				errpkg.WriteError(w, http.StatusTooManyRequests, "quota_exceeded", "Hourly limit reached; try again later")
				return
			}
		}
	}

	ch, created, err := h.intelSources.SubmitOneshot(r.Context(), req.URL, req.Categories)
	if err != nil {
		writeChannelServiceErr(w, err, "intel: oneshot submit failed", "submit_failed", "Could not submit the URL")
		return
	}

	// Fire the single fetch immediately (fire-and-forget per Rule 8).
	payload, err := json.Marshal(map[string]string{
		"version":    "v1",
		"channel_id": ch.ID.String(),
		"source_id":  ch.SourceID.String(),
	})
	if err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "enqueue_failed", "Failed to build task payload")
		return
	}
	if _, err := h.asynqClient.EnqueueContext(r.Context(), asynq.NewTask(TaskIntelFetchChannel, payload)); err != nil {
		slog.Error("intel: oneshot fetch enqueue failed", "channel_id", ch.ID.String(), "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "enqueue_failed", "Failed to enqueue fetch task")
		return
	}

	errpkg.WriteJSON(w, http.StatusAccepted, map[string]interface{}{
		"data": map[string]interface{}{
			"channel_id": ch.ID.String(),
			"source_id":  ch.SourceID.String(),
			"status":     "queued",
			"created":    created,
		},
	})
}
