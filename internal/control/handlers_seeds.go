package control

import (
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"strconv"
	"strings"

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
	worker.TypeSpaceAutoPopulate:             true,
	worker.TypeGeoPopulateScope:              true,
	worker.TypeWikiSeedAttractions:           true,
	worker.TypeWikiPopulateFacts:             true,
	worker.TypeWikiSeedStubPages:             true,
	worker.TypeUniversityFetchLogo:           true,
	worker.TypeMaintenanceUpdateGeoIPDB:      true,
	worker.TypeMaintenanceRegenerateLLMsTxt:  true,
	worker.TypeRSSRegenerate:                 true,
	worker.TypeSearchReindexEngagementScores: true,
}

type SeedParam struct {
	Key         string `json:"key"`
	Label       string `json:"label"`
	Placeholder string `json:"placeholder,omitempty"`
	Type        string `json:"type"` // text | number | checkbox | select
	Required    bool   `json:"required,omitempty"`
	// Options constrains a param to a fixed set of values (renders as a
	// select in the UI and is validated server-side on enqueue).
	Options []string `json:"options,omitempty"`
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
		Params:      seedTaskSchema[worker.TypeGeoPopulateScope],
	},
	{
		Kind:        "populate_space",
		TaskType:    worker.TypeSpaceAutoPopulate,
		Static:      map[string]any{"version": "1"},
		Title:       "Populate single space",
		Description: "Fetch Wikipedia summary + Wikidata P18/P154 (photo, logo, coat of arms) for one space and store as header/logo media.",
		Enqueueable: true,
		Params:      seedTaskSchema[worker.TypeSpaceAutoPopulate],
	},
	{
		Kind:        "wiki_attractions",
		TaskType:    worker.TypeWikiSeedAttractions,
		Title:       "Seed attractions (Wikidata)",
		Description: "Bulk-seed attraction wiki spaces under a geo parent from Wikidata, then auto-populate each (images included).",
		Enqueueable: true,
		Params:      seedTaskSchema[worker.TypeWikiSeedAttractions],
	},
	{
		Kind:        "wiki_stub_pages",
		TaskType:    worker.TypeWikiSeedStubPages,
		Title:       "Seed wiki stub pages",
		Description: "Create stub wiki pages under a space (destination wiki skeleton pages for later community/AI fill).",
		Enqueueable: true,
		Params:      seedTaskSchema[worker.TypeWikiSeedStubPages],
	},
	{
		Kind:        "university_logo",
		TaskType:    worker.TypeUniversityFetchLogo,
		Title:       "Fetch university logo",
		Description: "Download a university logo from a remote URL and store it on the space's media.",
		Enqueueable: true,
		Params:      seedTaskSchema[worker.TypeUniversityFetchLogo],
	},
	{
		Kind:        "wiki_facts",
		TaskType:    worker.TypeWikiPopulateFacts,
		Title:       "Populate wiki facts (Wikidata)",
		Description: "Fetch a space's Wikidata entity and write general-info facts into its wiki page fields — geo: population, languages, currency, postal codes, calling code, capital, founded, head of government, time zone; universities: founded year, students, website, initials, colleges/faculties, actual course offerings (from the course catalog), plus curated-dataset world ranking, type and contact email for seeded universities.",
		Enqueueable: true,
		Params:      seedTaskSchema[worker.TypeWikiPopulateFacts],
	},
	{
		Kind:        "university_geo_link",
		Title:       "Relink universities to geo parents",
		Description: "Backfill each university space's geo parent edges from its geo_code country. CLI — supports --dry-run and --purge.",
		Command:     "go run ./cmd/backfill-university-geo --dry-run",
	},
	{
		Kind:        "requeue_university_media",
		Title:       "Requeue university media enrichment",
		Description: "Re-enqueue space:auto_populate for university (or other) spaces missing a logo/header. Filter by slug or space types.",
		Command:     "go run ./cmd/requeue-universities -types=university",
	},
	{
		Kind:        "geoip_db",
		TaskType:    worker.TypeMaintenanceUpdateGeoIPDB,
		Title:       "Update GeoIP database",
		Description: "Download and refresh the MaxMind GeoIP database (reads license key from settings; geo personalization depends on it).",
		Enqueueable: true,
	},
	{
		Kind:        "llms_txt",
		TaskType:    worker.TypeMaintenanceRegenerateLLMsTxt,
		Title:       "Regenerate llms.txt",
		Description: "Rebuild the llms.txt AI-discovery file from current content and spaces.",
		Enqueueable: true,
	},
	{
		Kind:        "rss_regen",
		TaskType:    worker.TypeRSSRegenerate,
		Title:       "Regenerate RSS feed",
		Description: "Rebuild the public RSS feed XML from published content.",
		Enqueueable: true,
	},
	{
		Kind:        "engagement_reindex",
		TaskType:    worker.TypeSearchReindexEngagementScores,
		Title:       "Reindex engagement scores",
		Description: "Batched re-index of content whose engagement_score changed into Meilisearch.",
		Enqueueable: true,
	},
}

// seedTaskSchema declares the payload params each allowlisted task accepts —
// keys mirror the worker payload struct's json tags. The Settings seed-ops
// editor consumes this via GET /control/seeds/task-types so its param key
// field is a select instead of free text.
var seedTaskSchema = map[string][]SeedParam{
	worker.TypeSpaceAutoPopulate: {
		{Key: "space_id", Label: "Space UUID", Type: "text", Required: true},
	},
	worker.TypeGeoPopulateScope: {
		{Key: "geo_code", Label: "Country code", Placeholder: "NG", Type: "text", Required: true},
		{Key: "level", Label: "Level", Type: "select", Options: []string{"city", "state", "university", "country", "custom"}},
		{Key: "limit", Label: "Max spaces", Placeholder: "200", Type: "number"},
	},
	worker.TypeWikiSeedAttractions: {
		{Key: "parent_space_id", Label: "Parent geo space UUID", Type: "text", Required: true},
		{Key: "limit", Label: "Max attractions", Placeholder: "50", Type: "number"},
		{Key: "dry_run", Label: "Dry run (plan only, no writes)", Type: "checkbox"},
	},
	worker.TypeWikiSeedStubPages: {
		{Key: "space_id", Label: "Space UUID", Type: "text", Required: true},
	},
	worker.TypeWikiPopulateFacts: {
		{Key: "space_id", Label: "Space UUID (geo or university)", Type: "text", Required: true},
		{Key: "dry_run", Label: "Dry run (report planned writes, no persistence)", Type: "checkbox"},
		{Key: "overwrite", Label: "Overwrite existing field values", Type: "checkbox"},
	},
	worker.TypeUniversityFetchLogo: {
		{Key: "space_id", Label: "University space UUID", Type: "text", Required: true},
		{Key: "logo_url", Label: "Logo URL", Type: "text", Required: true},
		{Key: "uploaded_by", Label: "Uploaded by (user UUID)", Type: "text"},
	},
	worker.TypeMaintenanceUpdateGeoIPDB:      {},
	worker.TypeMaintenanceRegenerateLLMsTxt:  {},
	worker.TypeRSSRegenerate:                 {},
	worker.TypeSearchReindexEngagementScores: {},
}

// seedOpsDefaultJSON is the serialized built-in catalog used as the
// control.seed_ops setting default, so the Settings UI shows the real ops
// (editable/extendable) instead of an empty [] that silently means
// "use built-ins".
var seedOpsDefaultJSON = func() string {
	b, _ := json.Marshal(seedOps)
	return string(b)
}()

// ListSeedTaskTypes handles GET /control/seeds/task-types — the allowlisted
// Asynq task types a seed op may target. Consumed by the Control → Settings
// seed-ops editor so its task_type select is populated from the backend
// rather than a hardcoded list (Rule 16).
func (h *Handlers) ListSeedTaskTypes(w http.ResponseWriter, r *http.Request) {
	type taskEntry struct {
		Type   string      `json:"type"`
		Params []SeedParam `json:"params"`
	}
	types := make([]taskEntry, 0, len(seedTaskAllowlist))
	for taskType := range seedTaskAllowlist {
		params := seedTaskSchema[taskType]
		if params == nil {
			params = []SeedParam{}
		}
		types = append(types, taskEntry{Type: taskType, Params: params})
	}
	slices.SortFunc(types, func(a, b taskEntry) int { return strings.Compare(a.Type, b.Type) })
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": types})
}

// ListSeedOps handles GET /control/seeds — the catalog of seed operations.
// The list is hot-editable via the control.seed_ops system_setting (JSON
// array in SeedOp shape); the built-in seedOps is the fallback.
func (h *Handlers) ListSeedOps(w http.ResponseWriter, r *http.Request) {
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": h.resolveSeedOps(r)})
}

func (h *Handlers) resolveSeedOps(r *http.Request) []SeedOp {
	ops := seedOps
	if h.Settings != nil {
		if raw, err := h.Settings.GetSetting(r.Context(), "control.seed_ops"); err == nil && raw != "" {
			var custom []SeedOp
			if json.Unmarshal([]byte(raw), &custom) == nil && len(custom) > 0 {
				ops = custom
			}
		}
	}
	return clampSeedOps(ops)
}

// clampSeedOps forces enqueueable=false on ops whose task_type is missing or
// outside the allowlist, so a settings-defined op cannot render an enqueue
// button that is guaranteed to 400 — it degrades to a display/command card.
func clampSeedOps(ops []SeedOp) []SeedOp {
	out := make([]SeedOp, len(ops))
	for i, op := range ops {
		if op.Enqueueable && (op.TaskType == "" || !seedTaskAllowlist[op.TaskType]) {
			op.Enqueueable = false
		}
		out[i] = op
	}
	return out
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
		// An absent key or an empty-string value on an optional param is
		// simply omitted from the payload — the worker applies its own
		// defaults. A required param must carry a non-empty value.
		if !ok || v == nil || v == "" {
			if p.Required {
				errpkg.WriteError(w, http.StatusBadRequest, "missing_param", p.Key+" is required")
				return
			}
			continue
		}
		switch p.Type {
		case "number":
			n, err := seedParamAsInt(v)
			if err != nil {
				errpkg.WriteError(w, http.StatusBadRequest, "invalid_param", p.Key+" must be a number")
				return
			}
			payload[p.Key] = n
		case "checkbox":
			b, isBool := v.(bool)
			if !isBool {
				errpkg.WriteError(w, http.StatusBadRequest, "invalid_param", p.Key+" must be true or false")
				return
			}
			payload[p.Key] = b
		default:
			// text, select, and any future string-typed params must arrive
			// as strings — a wrong JSON type here would otherwise reach the
			// worker and fail payload decoding there instead.
			s, isString := v.(string)
			if !isString {
				errpkg.WriteError(w, http.StatusBadRequest, "invalid_param", p.Key+" must be a string")
				return
			}
			if len(p.Options) > 0 && !slices.Contains(p.Options, s) {
				errpkg.WriteError(w, http.StatusBadRequest, "invalid_param", p.Key+" must be one of: "+strings.Join(p.Options, ", "))
				return
			}
			payload[p.Key] = s
		}
	}

	body, _ := json.Marshal(payload)
	// TaskOpts carries the queue tier plus the task's registered defaults
	// (Unique dedup, timeout, max retry) — a bare Queue() enqueue would drop
	// them (e.g. space:auto_populate's repeat-collapse for the same space).
	if _, err := h.Client.Enqueue(asynq.NewTask(op.TaskType, body), worker.TaskOpts(op.TaskType)...); err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "enqueue_failed", "Failed to enqueue seed job")
		return
	}

	errpkg.WriteJSON(w, http.StatusAccepted, map[string]any{
		"data": map[string]string{"status": "enqueued", "task": op.TaskType},
	})
}

// seedParamAsInt coerces a JSON param value to int. Numeric strings (what
// the SPA sends from <input type="number">) and JSON numbers are accepted;
// anything else is an invalid_param rather than silently becoming zero.
func seedParamAsInt(v any) (int, error) {
	switch t := v.(type) {
	case string:
		n, err := strconv.Atoi(strings.TrimSpace(t))
		if err != nil {
			return 0, err
		}
		return n, nil
	case float64:
		return int(t), nil
	default:
		return 0, errors.New("not a number")
	}
}
