// Package schemas defines the entity schemas used by the extractor to
// validate LLM output, and the registry that maps an entity_type to its
// schema so the spine can dispatch extraction generically across domains.
//
// Each domain (scholarship, job, visa, fx, remittance, cost_of_living,
// housing) registers its EntitySchema here. The extractor's buildPrompt
// looks up the schema + prompt template by entity_type, so adding a new
// domain is a thin addition: one schema file + one prompt template + a
// registry entry. No extractor code change.
//
// Ref: docs/superpowers/specs/2026-08-12-japa-intelligence-system-design.md (Section 8)
package schemas

// EntityType constants for the intelligence domains. These are the
// intel_records.entity_type values and the registry keys. The source
// category on intel_sources.category maps to one of these via
// CategoryToEntityType.
const (
	EntityTypeScholarship      = "scholarship"
	EntityTypeJob              = "job"
	EntityTypeVisa             = "visa"
	EntityTypeRemittance       = "remittance"
	EntityTypeCostOfLiving     = "cost_of_living"
	EntityTypeHousing          = "housing"
	EntityTypeTravelRoutes     = "travel_routes"
	EntityTypeGrants           = "grants"
	EntityTypeAdmission        = "admission"
	EntityTypeBanking          = "banking"
	EntityTypeWorkCareer       = "work_career"
	EntityTypeHealthInsurance  = "health_insurance"
	EntityTypeCultureLifestyle = "culture_lifestyle"
)

// registry maps entity_type → EntitySchema. Populated by init() so the
// map is immutable after package load. Adding a domain = add a var here
// and an entry in the registry.
var registry = map[string]EntitySchema{
	EntityTypeScholarship:      Scholarship,
	EntityTypeJob:              Job,
	EntityTypeVisa:             Visa,
	EntityTypeRemittance:       Remittance,
	EntityTypeCostOfLiving:     CostOfLiving,
	EntityTypeHousing:          Housing,
	EntityTypeTravelRoutes:     TravelRoutes,
	EntityTypeGrants:           Grants,
	EntityTypeAdmission:        Admission,
	EntityTypeBanking:          Banking,
	EntityTypeWorkCareer:       WorkCareer,
	EntityTypeHealthInsurance:  HealthInsurance,
	EntityTypeCultureLifestyle: CultureLifestyle,
}

// ForEntityType returns the registered EntitySchema for the given
// entity_type. ok=false when no schema is registered (the caller should
// route the record to REVIEW_REQUIRED rather than failing the chain).
func ForEntityType(entityType string) (EntitySchema, bool) {
	s, ok := registry[entityType]
	return s, ok
}

// RegisteredEntityTypes returns all entity_type keys in the registry.
// Used by the moderation surface to populate the entity_type filter and
// by the radar union to know which domain tables to query.
func RegisteredEntityTypes() []string {
	types := make([]string, 0, len(registry))
	for k := range registry {
		types = append(types, k)
	}
	return types
}

// CategoryToEntityType maps an intel_sources.category value to the
// entity_type used in intel_records and the schema registry. The source
// category is the user-facing classification; the entity_type is the
// extraction key. Categories that don't map to a registered extraction
// domain return "" and the extractor routes them to REVIEW_REQUIRED.
func CategoryToEntityType(category string) string {
	switch category {
	case "scholarship":
		return EntityTypeScholarship
	case "job":
		return EntityTypeJob
	case "visa", "travel_alert":
		return EntityTypeVisa
	case "remittance":
		return EntityTypeRemittance
	case "cost_of_living":
		return EntityTypeCostOfLiving
	case "housing", "accommodation":
		return EntityTypeHousing
	case "travel_routes":
		return EntityTypeTravelRoutes
	case "grants":
		return EntityTypeGrants
	case "admission":
		return EntityTypeAdmission
	case "banking":
		return EntityTypeBanking
	case "work_career":
		return EntityTypeWorkCareer
	case "health_insurance":
		return EntityTypeHealthInsurance
	case "culture_lifestyle":
		return EntityTypeCultureLifestyle
	default:
		return ""
	}
}
