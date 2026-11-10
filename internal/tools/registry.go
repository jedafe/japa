package tools

// Canonical registry of the indexable /tools/{slug} pages served by the
// main SvelteKit app (apps/main/src/routes/(app)/tools/*). These are
// editorial SSR pages, not DB rows — every backend surface that needs the
// catalog reads it from here:
//   - sitemap emission (internal/seo: loc + flag gating)
//   - thread anchors (internal/comments: mint/resolve validation, embed
//     card title/summary)
//
// Mirror: apps/main/src/lib/components/shell/routeFeatureFlags.ts lists the
// same /tools/* flag keys — keep the two in sync when a tool page is added
// or re-flagged. A page absent here cannot mint a thread and never lands
// in the sitemap.
//
// Lives in a leaf package (no internal imports) so comments and seo can
// share it without an import cycle — same pattern as
// rssmodel.ContentTypeFeatureFlag.

// Page is one indexable /tools/{slug} page.
type Page struct {
	Slug    string // /tools/{Slug} path segment — thread anchor entity_key
	Flag    string // feature_flags key gating the page ('' = always on)
	Title   string // display title (anchor entity_title / thread title base)
	Summary string // embed-card / anchor entity_summary
}

// Pages is the canonical tool-page catalog.
var Pages = []Page{
	{"arrival-checklist", "arrival_checklist.enabled", "Arrival Checklist", "A personalized checklist for your first weeks after landing — documents to carry, registrations to complete, bank, phone, transport, and the tasks people commonly forget. Built per destination so nothing slips through."},
	{"attestation-guide", "attestation_guide.enabled", "Attestation Guide", "Step-by-step guidance for getting your documents attested — which offices to visit, what order to follow, and the fees and timelines at each stage. Covers Nigerian and destination-country requirements."},
	{"community-locator", "community_locator.enabled", "Community Locator", "Find diaspora communities near you — associations, religious centres, cultural groups, and informal networks organised by city and country of origin."},
	{"cost-of-living", "col_calculator.enabled", "Cost of Living Calculator", "Compare real living costs across destination cities — rent, food, transport, childcare, and utilities — side by side with your origin city so you can budget before you move."},
	{"credential-checker", "credential_checker.enabled", "Credential Checker", "Check how your degrees and professional credentials transfer abroad — recognition status, evaluation bodies, and the extra steps each destination requires."},
	{"crs-calculator", "crs_calculator.enabled", "CRS Calculator", "Estimate your Comprehensive Ranking System score for Canadian Express Entry — age, education, language, and work experience broken down per factor so you can see where to gain points."},
	{"currency-converter", "fx.enabled", "Currency Converter", "Convert between your home and destination currencies with live rates — useful for budgeting, remittances, and comparing salaries across countries."},
	{"cv-formatter", "cv_formatter.enabled", "CV Formatter", "Format your CV for your destination market — the structure, length, and conventions recruiters in that country actually expect."},
	{"eca-guide", "eca_guide.enabled", "ECA Guide", "Educational Credential Assessment guidance — which ECA bodies evaluate your qualifications, how long each takes, and which one your destination accepts."},
	{"embassy-alerter", "appointment_alert.enabled", "Embassy Alerter", "Alerts when embassy and visa application centre appointment slots open — interview calendars fill fast and checking manually is a losing game."},
	{"housing-guide", "housing_guide.enabled", "Housing Guide", "Housing guides for destination cities — where newcomers typically live, what deposits and paperwork landlords expect, and realistic rent ranges by area."},
	{"language-test", "language_test_hub.enabled", "Language Test Hub", "Prepare for IELTS, TOEFL, PTE and other language tests — target scores per visa route, prep resources, and booking links."},
	{"pof-calculator", "pof_calculator.enabled", "Proof of Funds Calculator", "Compute proof-of-funds requirements for your destination — minimum balances, how many months of statements, and acceptable account types."},
	{"processing-tracker", "visa_timeline.enabled", "Processing Time Tracker", "Track current visa and permit processing times — real submission-to-decision timelines by route, so you can plan flights and resignations sensibly."},
	{"readiness", "assessment.enabled", "Readiness Assessment", "Assess how ready you are to relocate — documents, finances, credentials, and timeline scored so you know exactly what's left before you can move."},
	{"remittance", "remittance.enabled", "Remittance Rate Tracker", "Track live remittance rates and fees across corridors — compare providers before you send money home."},
	{"sop-generator", "sop_generator.enabled", "SOP Generator", "Draft a statement of purpose for admissions or visa applications — structured around your background, goals, and the programme you're targeting."},
	{"tax-calculator", "tax_calculator.enabled", "Tax Calculator", "Estimate income tax and take-home pay in your destination country — so salary offers can be compared on net, not gross."},
	{"travel", "travel.enabled", "Travel Search", "Search and compare flights and hotels for your route — prices across airlines and dates so you can book the legwork smartly."},
	{"visa-checklist", "checklist.enabled", "Visa Checklist", "Generate your visa document checklist by destination — every required document, form, and supporting evidence for your route, in submission order."},
	{"visa-routes", "visa_routes.enabled", "Visa Routes", "Compare visa routes for your destination — eligibility, cost, timeline, and pathway-to-residency for each option."},
}

var bySlug = func() map[string]Page {
	m := make(map[string]Page, len(Pages))
	for _, p := range Pages {
		m[p.Slug] = p
	}
	return m
}()

var flagToSlug = func() map[string]string {
	m := make(map[string]string, len(Pages))
	for _, p := range Pages {
		if p.Flag != "" {
			m[p.Flag] = p.Slug
		}
	}
	return m
}()

// Lookup returns the catalog entry for a /tools/{slug} page, or false.
func Lookup(slug string) (Page, bool) {
	p, ok := bySlug[slug]
	return p, ok
}

// FlagFor returns the feature_flags key gating a tool page ('' when the
// page is always on or unknown).
func FlagFor(slug string) string {
	return bySlug[slug].Flag
}

// SlugForFlag returns the tool slug a feature flag gates, or false when the
// flag gates no tool page.
func SlugForFlag(flagKey string) (string, bool) {
	slug, ok := flagToSlug[flagKey]
	return slug, ok
}

// Slugs returns every registered tool slug (sitemap enumeration).
func Slugs() []string {
	out := make([]string, len(Pages))
	for i, p := range Pages {
		out[i] = p.Slug
	}
	return out
}
