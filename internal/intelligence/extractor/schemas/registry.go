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
	EntityTypeScholarship           = "scholarship"
	EntityTypeJob                   = "job"
	EntityTypeVisa                  = "visa"
	EntityTypeRemittance            = "remittance"
	EntityTypeCostOfLiving          = "cost_of_living"
	EntityTypeHousing               = "housing"
	EntityTypeTravelRoutes          = "travel_routes"
	EntityTypeGrants                = "grants"
	EntityTypeAdmission             = "admission"
	EntityTypeBanking               = "banking"
	EntityTypeWorkCareer            = "work_career"
	EntityTypeHealthInsurance       = "health_insurance"
	EntityTypeCultureLifestyle      = "culture_lifestyle"
	EntityTypeInternship            = "internship"
	EntityTypeImportExport          = "import_export"
	EntityTypeWikiPage              = "wiki_page"
	EntityTypeLanguageTest          = "language_test_requirement"
	EntityTypeTaxObligation         = "tax_obligation_guide"
	EntityTypeCommunityOrganisation = "community_organisation"
	// #nosec G101 -- "credential_pathway" is an entity-type label, not a secret.
	EntityTypeCredentialPathway = "credential_pathway"
	EntityTypeNews              = "news"
)

// registry maps entity_type → EntitySchema. Populated by init() so the
// map is immutable after package load. Adding a domain = add a var here
// and an entry in the registry.
var registry = map[string]EntitySchema{
	EntityTypeScholarship:           Scholarship,
	EntityTypeJob:                   Job,
	EntityTypeVisa:                  Visa,
	EntityTypeRemittance:            Remittance,
	EntityTypeCostOfLiving:          CostOfLiving,
	EntityTypeHousing:               Housing,
	EntityTypeTravelRoutes:          TravelRoutes,
	EntityTypeGrants:                Grants,
	EntityTypeAdmission:             Admission,
	EntityTypeBanking:               Banking,
	EntityTypeWorkCareer:            WorkCareer,
	EntityTypeHealthInsurance:       HealthInsurance,
	EntityTypeCultureLifestyle:      CultureLifestyle,
	EntityTypeInternship:            Internship,
	EntityTypeImportExport:          ImportExport,
	EntityTypeWikiPage:              WikiPage,
	EntityTypeLanguageTest:          LanguageTestRequirement,
	EntityTypeTaxObligation:         TaxObligationGuide,
	EntityTypeCommunityOrganisation: CommunityOrganisation,
	EntityTypeCredentialPathway:     CredentialPathway,
	EntityTypeNews:                  News,
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

// categoryEntityTypes maps each intel_sources.category value to the
// entity_type used in intel_records and the schema registry.
var categoryEntityTypes = map[string]string{
	"scholarship":               EntityTypeScholarship,
	"job":                       EntityTypeJob,
	"visa":                      EntityTypeVisa,
	"travel_alert":              EntityTypeVisa,
	"remittance":                EntityTypeRemittance,
	"cost_of_living":            EntityTypeCostOfLiving,
	"housing":                   EntityTypeHousing,
	"accommodation":             EntityTypeHousing,
	"travel_routes":             EntityTypeTravelRoutes,
	"grants":                    EntityTypeGrants,
	"admission":                 EntityTypeAdmission,
	"banking":                   EntityTypeBanking,
	"work_career":               EntityTypeWorkCareer,
	"health_insurance":          EntityTypeHealthInsurance,
	"culture_lifestyle":         EntityTypeCultureLifestyle,
	"internship":                EntityTypeInternship,
	"import_export":             EntityTypeImportExport,
	"wiki":                      EntityTypeWikiPage,
	"wiki_page":                 EntityTypeWikiPage,
	"language_test":             EntityTypeLanguageTest,
	"language_test_requirement": EntityTypeLanguageTest,
	"tax":                       EntityTypeTaxObligation,
	"tax_obligation":            EntityTypeTaxObligation,
	"tax_obligation_guide":      EntityTypeTaxObligation,
	"community":                 EntityTypeCommunityOrganisation,
	"community_organisation":    EntityTypeCommunityOrganisation,
	"diaspora":                  EntityTypeCommunityOrganisation,
	"credential":                EntityTypeCredentialPathway,
	"credential_pathway":        EntityTypeCredentialPathway,
	"professional_credential":   EntityTypeCredentialPathway,
	"news":                      EntityTypeNews,
}

// CategoryToEntityType maps an intel_sources.category value to the
// entity_type used in intel_records and the schema registry. The source
// category is the user-facing classification; the entity_type is the
// extraction key. Categories that don't map to a registered extraction
// domain return "" and the extractor routes them to REVIEW_REQUIRED.
func CategoryToEntityType(category string) string {
	return categoryEntityTypes[category]
}

// RegisteredCategories returns the canonical intel_sources.category values —
// one primary label per extraction domain, plus "fx" (a directory domain
// with no entity mapping; such sources produce review-queue stubs). Aliases
// (accommodation → housing, diaspora → community_organisation, etc.) remain
// valid in the DB via CategoryToEntityType but are not offered as choices.
// Sorted for stable option lists. Exposed through /api/v1/sources/meta as
// admin_categories so the control-panel source form consumes the taxonomy
// from the API rather than a hardcoded list (Rule 16).
func RegisteredCategories() []string {
	return []string{
		"admission",
		"banking",
		"community_organisation",
		"cost_of_living",
		"credential_pathway",
		"culture_lifestyle",
		"fx",
		"grants",
		"health_insurance",
		"housing",
		"import_export",
		"internship",
		"job",
		"language_test",
		"news",
		"remittance",
		"scholarship",
		"tax_obligation",
		"travel_alert",
		"travel_routes",
		"visa",
		"wiki_page",
		"work_career",
	}
}
