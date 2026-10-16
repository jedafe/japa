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
	Kind        string         `json:"kind"`
	Title       string         `json:"title"`
	Description string         `json:"description"`
	Enqueueable bool           `json:"enqueueable"`
	Command     string         `json:"command,omitempty"`
	// TaskType is the Asynq task enqueued for this op; must be in
	// seedTaskAllowlist (system_settings can only re-surface seed tasks,
	// never arbitrary queue writes). Static carries fixed payload fields
	// (e.g. version markers) merged before params.
	TaskType string         `json:"task_type,omitempty"`
	Static   map[string]any `json:"static,omitempty"`
	Params   []SeedParam    `json:"params,omitempty"`
}

// seedTaskAllowlist bounds what a settings-defined op may enqueue. Ops whose
// task_type is absent here are rejected — the catalog can be edited via
// system.settings but never escapes the seed-task surface.
var seedTaskAllowlist = map[string]bool{
	worker.TypeSpaceAutoPopulate:     true,
	worker.TypeGeoPopulateScope:      true,
	worker.TypeWikiSeedAttractions:   true,
	worker.TypeWikiSeedStubPages:     true,
	worker.TypeUniversityFetchLogo:   true,
	"maintenance:update_geoip_db":    true,
	worker.TypeMaintenanceRegenerateLLMsTxt: true,
	worker.TypeRSSRegenerate:         true,
	worker.TypeSearchReindexEngagementScores: true,
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
		TaskType:    worker.TypeGeoPopulateScope,
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
		TaskType:    worker.TypeSpaceAutoPopulate,
		Static:      map[string]any{"version": "1"},
		Title:       "Populate single space",
		Description: "Fetch Wikipedia summary + Wikidata P18/P154 (photo, logo, coat of arms) for one space and store as header/logo media.",
		Enqueueable: true,
		Params: []SeedParam{
			{Key: "space_id", Label: "Space UUID", Type: "text", Required: true},
		},
	},
	{
		Kind:        "wiki_attractions",
		TaskType:    worker.TypeWikiSeedAttractions,
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
// The list is hot-editable via the control.seed_ops system_setting (JSON
// array in SeedOp shape); the built-in seedOps is the fallback.
func (h *Handlers) ListSeedOps(w http.ResponseWriter, r *http.Request) {
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": h.resolveSeedOps(r)})
}

func (h *Handlers) resolveSeedOps(r *http.Request) []SeedOp {
	if h.Settings != nil {
		if raw, err := h.Settings.GetSetting(r.Context(), "control.seed_ops"); err == nil && raw != "" {
			var custom []SeedOp
			if json.Unmarshal([]byte(raw), &custom) == nil && len(custom) > 0 {
				return custom
			}
		}
	}
	return seedOps
}

func (h *Handlers) findSeedOp(r *http.Request, kind string) *SeedOp {
	for _, op := range h.resolveSeedOps(r) {
		if op.Kind == kind {
			return &op
		}
	}
	return nil
}

// EnqueueSeed handles POST /control/seeds/enqueue — resolves the op from
// the (possibly settings-driven) catalog, validates required params, merges
// static + params into the task payload, and enqueues it. Task type is
// allowlist-gated so the settings catalog can only surface seed tasks.
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

	op := h.findSeedOp(r, req.Kind)
	if op == nil || !op.Enqueueable || op.TaskType == "" || !seedTaskAllowlist[op.TaskType] {
		errpkg.WriteError(w, http.StatusBadRequest, "unknown_seed_op", "Unknown or non-enqueueable seed op")
		return
	}

	payload := map[string]any{}
	for k, v := range op.Static {
		payload[k] = v
	}
	for _, p := range op.Params {
		v, ok := req.Params[p.Key]
		if p.Required && (v == nil || v == "") {
			errpkg.WriteError(w, http.StatusBadRequest, "missing_param", p.Key+" is required")
			return
		}
		if !ok {
			continue
		}
		if p.Type == "number" {
			switch t := v.(type) {
			case string:
				n, _ := strconv.Atoi(t)
				payload[p.Key] = n
			case float64:
				payload[p.Key] = int(t)
			default:
				payload[p.Key] = v
			}
		} else {
			payload[p.Key] = v
		}
	}

	body, _ := json.Marshal(payload)
	if _, err := h.Client.Enqueue(asynq.NewTask(op.TaskType, body), asynq.Queue(worker.QueueForTask(op.TaskType))); err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "enqueue_failed", "Failed to enqueue seed job")
		return
	}

	errpkg.WriteJSON(w, http.StatusAccepted, map[string]any{
		"data": map[string]string{"status": "enqueued", "task": op.TaskType},
	})
}
