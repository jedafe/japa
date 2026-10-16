package control

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/hibiken/asynq"
	errpkg "github.com/jedafe/japa/internal/errors"
	"github.com/jedafe/japa/internal/worker"
)

// SeedOp describes one seeding/enrichment operation surfaced in
// Control → Seeds. kind selects the backing mechanism; ops with
// enqueueable=true map to an Asynq task, CLI-only ops carry the command
// an operator runs manually (too heavy for in-process execution).
type SeedOp struct {
	Kind        string `json:"kind"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Enqueueable bool   `json:"enqueueable"`
	Command     string `json:"command,omitempty"`
	// Param fields the UI renders as inputs when enqueueable.
	Params []SeedParam `json:"params,omitempty"`
}

type SeedParam struct {
	Key         string `json:"key"`
	Label       string `json:"label"`
	Placeholder string `json:"placeholder,omitempty"`
	Type        string `json:"type"` // text | number | checkbox
	Required    bool   `json:"required,omitempty"`
}

// seedOps is the catalog shown in Control → Seeds. Extend this list as new
// seed pipelines land (university courses/rankings, attraction media, etc).
var seedOps = []SeedOp{
	{
		Kind:        "geo_cities",
		Title:       "Geo cities (GeoNames)",
		Description: "Bulk-seed city spaces for every country from the GeoNames cities5000 dump (~70k rows). CLI-only — runs against the DB directly. Pair with --populate-top to also fetch images.",
		Command:     "go run ./cmd/seed-geo-cities --populate-top=5",
	},
	{
		Kind:        "populate_geo",
		Title:       "Populate geo scope",
		Description: "Enqueue space:auto_populate for spaces in a country missing header/logo media (fetches Wikipedia/Wikidata images + metadata).",
		Enqueueable: true,
		Params: []SeedParam{
			{Key: "geo_code", Label: "Country code", Placeholder: "NG", Type: "text", Required: true},
			{Key: "level", Label: "Level", Placeholder: "city | university | state", Type: "text"},
			{Key: "limit", Label: "Max spaces", Placeholder: "200", Type: "number"},
		},
	},
	{
		Kind:        "populate_space",
		Title:       "Populate single space",
		Description: "Fetch Wikipedia summary + Wikidata P18/P154 (photo, logo, coat of arms) for one space and store as header/logo media.",
		Enqueueable: true,
		Params: []SeedParam{
			{Key: "space_id", Label: "Space UUID", Type: "text", Required: true},
		},
	},
	{
		Kind:        "wiki_attractions",
		Title:       "Seed attractions (Wikidata)",
		Description: "Bulk-seed attraction wiki spaces under a geo parent from Wikidata, then auto-populate each (images included).",
		Enqueueable: true,
		Params: []SeedParam{
			{Key: "parent_space_id", Label: "Parent geo space UUID", Type: "text", Required: true},
			{Key: "limit", Label: "Max attractions", Placeholder: "50", Type: "number"},
			{Key: "dry_run", Label: "Dry run (plan only, no writes)", Type: "checkbox"},
		},
	},
}

// ListSeedOps handles GET /control/seeds — the catalog of seed operations.
func (h *Handlers) ListSeedOps(w http.ResponseWriter, _ *http.Request) {
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": seedOps})
}

// EnqueueSeed handles POST /control/seeds/enqueue — validates the op +
// params and enqueues the backing Asynq task.
func (h *Handlers) EnqueueSeed(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Kind   string         `json:"kind"`
		Params map[string]any `json:"params"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Request body is not valid JSON")
		return
	}
	if h.Client == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "queue_unavailable", "Task queue not available")
		return
	}

	str := func(k string) string {
		if v, ok := req.Params[k].(string); ok {
			return v
		}
		return ""
	}
	num := func(k string) int {
		switch v := req.Params[k].(type) {
		case float64:
			return int(v)
		case string:
			n, _ := strconv.Atoi(v)
			return n
		}
		return 0
	}
	b := func(k string) bool {
		v, _ := req.Params[k].(bool)
		return v
	}

	var taskType string
	var payload []byte

	switch req.Kind {
	case "populate_geo":
		geo := str("geo_code")
		if geo == "" {
			errpkg.WriteError(w, http.StatusBadRequest, "missing_param", "geo_code is required")
			return
		}
		taskType = worker.TypeGeoPopulateScope
		payload, _ = json.Marshal(map[string]any{
			"geo_code": geo,
			"level":    str("level"),
			"limit":    num("limit"),
		})
	case "populate_space":
		sid := str("space_id")
		if sid == "" {
			errpkg.WriteError(w, http.StatusBadRequest, "missing_param", "space_id is required")
			return
		}
		taskType = worker.TypeSpaceAutoPopulate
		payload, _ = json.Marshal(map[string]string{"version": "1", "space_id": sid})
	case "wiki_attractions":
		pid := str("parent_space_id")
		if pid == "" {
			errpkg.WriteError(w, http.StatusBadRequest, "missing_param", "parent_space_id is required")
			return
		}
		taskType = worker.TypeWikiSeedAttractions
		payload, _ = json.Marshal(map[string]any{
			"parent_space_id": pid,
			"limit":           num("limit"),
			"dry_run":         b("dry_run"),
		})
	default:
		errpkg.WriteError(w, http.StatusBadRequest, "unknown_seed_op", "Unknown or non-enqueueable seed op")
		return
	}

	if _, err := h.Client.Enqueue(asynq.NewTask(taskType, payload), asynq.Queue(worker.QueueForTask(taskType))); err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "enqueue_failed", "Failed to enqueue seed job")
		return
	}

	errpkg.WriteJSON(w, http.StatusAccepted, map[string]any{
		"data": map[string]string{"status": "enqueued", "task": taskType},
	})
}
