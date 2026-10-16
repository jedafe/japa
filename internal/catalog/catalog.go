package catalog

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	errpkg "github.com/jedafe/japa/internal/errors"
)

// CatalogOption is a single value/label pair returned by the public catalog
// endpoint. The Tools surface uses these to populate select/checkbox options
// instead of hardcoded arrays (Rule 16). Default marks a value that should
// be pre-selected when a form is initially opened or reset.
//
//nolint:revive // exported: stutter naming is intentional
type CatalogOption struct {
	Value   string `json:"value"`
	Label   string `json:"label"`
	Default bool   `json:"default,omitempty"`
}

// publicCatalogs is the source of truth for domain-specific option lists used
// by the public Tools surface. Each catalog is keyed by a dot-separated name of
// the form `<tool>.<option>`. Values that are static domain enums live here.
var publicCatalogs = map[string][]CatalogOption{
	"tax_calculator.employment_status": {
		{Value: "", Label: "Any (optional)"},
		{Value: "employed", Label: "Employed"},
		{Value: "self_employed", Label: "Self-employed"},
		{Value: "contractor", Label: "Contractor"},
		{Value: "unemployed", Label: "Unemployed"},
	},
	"remittance.alert_direction": {
		{Value: "above", Label: "rate goes above"},
		{Value: "below", Label: "rate drops below"},
	},
	"cv_formatter.format": {
		{Value: "docx", Label: "DOCX"},
		{Value: "pdf", Label: "PDF"},
	},
	"appointment_alert.notification_channels": {
		{Value: "in_app", Label: "In-app", Default: true},
		{Value: "email", Label: "Email", Default: true},
		{Value: "push", Label: "Push"},
		{Value: "sms", Label: "SMS"},
	},
	"scholarships.renewable": {
		{Value: "", Label: "Any"},
		{Value: "true", Label: "Yes"},
		{Value: "false", Label: "No"},
	},
	// §83 — canonical milestone sequence for the visa timeline tracker
	// (visa_timeline_milestones.milestone_type enum, migration 000308).
	"visa_timeline.milestone_type": {
		{Value: "applied", Label: "Application submitted"},
		{Value: "biometrics", Label: "Biometrics"},
		{Value: "medical", Label: "Medical exam"},
		{Value: "docs_requested", Label: "Documents requested"},
		{Value: "decision", Label: "Decision"},
		{Value: "visa_issued", Label: "Visa issued"},
	},
	// §83 — timeline outcome enum (visa_timelines.outcome).
	"visa_timeline.outcome": {
		{Value: "pending", Label: "Pending"},
		{Value: "approved", Label: "Approved"},
		{Value: "refused", Label: "Refused"},
		{Value: "withdrawn", Label: "Withdrawn"},
	},
	// §83 — how the application was lodged (visa_timelines.application_method).
	"visa_timeline.application_method": {
		{Value: "online", Label: "Online"},
		{Value: "in_person", Label: "In person"},
		{Value: "agent", Label: "Via agent"},
	},
	// §67 — cabin_class values forwarded to the configured flight provider's
	// search API (travel_providers api_config contract, ADR-085).
	"travel.cabin_class": {
		{Value: "economy", Label: "Economy"},
		{Value: "premium_economy", Label: "Premium Economy"},
		{Value: "business", Label: "Business"},
		{Value: "first", Label: "First"},
	},
	// §67 — travel_providers.provider_type CHECK constraint values; used by
	// the public Providers tab type filter (mirrors the admin catalog).
	"travel.provider_type": {
		{Value: "flight", Label: "Flights"},
		{Value: "hotel", Label: "Hotels"},
		{Value: "both", Label: "Flights & Hotels"},
	},
}

// Handlers exposes the public option catalog endpoints.
type Handlers struct{}

// NewHandlers creates a public catalog handler.
func NewHandlers() *Handlers {
	return &Handlers{}
}

// GetCatalog handles GET /api/v1/catalogs/{name}.
// It returns a single catalog of value/label options for public UI dropdowns.
// The catalog is a canonical backend source of truth (Rule 16).
func (h *Handlers) GetCatalog(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	if name == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_catalog", "Catalog name is required")
		return
	}

	opts, ok := publicCatalogs[name]
	if !ok {
		errpkg.WriteError(w, http.StatusNotFound, "catalog_not_found", "Unknown catalog: "+name)
		return
	}

	// Defensive copy so a caller cannot mutate the shared catalog.
	out := make([]CatalogOption, len(opts))
	copy(out, opts)

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": out})
}
