import type { ToolSurface } from '$lib/types';

/**
 * Tool registry — structural metadata for each identity tool on the Tools
 * surface (§78, ADR-086). This is the tool *scaffold* metadata (slug, route,
 * icon name, short description) — NOT domain data. All dynamic content
 * (checklist items, assessment questions, scholarship listings, remittance
 * rates, etc.) comes from the backend API per Rule 16.
 *
 * The icon field is a Lucide icon name (kebab-case) resolved by the Icon
 * component. The accent color is used in single-tool shell mode for branding.
 */

export interface ToolMeta {
	slug: ToolSurface;
	/** Route path on the tools surface (excludes the surface origin). */
	route: string;
	/** Lucide icon name. */
	icon: string;
	/** Short display name. */
	name: string;
	/** i18n key for the display name. */
	nameKey: string;
	/** One-line description for the hub card. */
	tagline: string;
	/** i18n key for the tagline. */
	taglineKey: string;
	/** Longer description for SEO / hub card body. */
	description: string;
	/** i18n key for the description. */
	descriptionKey: string;
	/** Accent color for the single-tool shell (hex). */
	accent: string;
	/** Accent hover color. */
	accentHover: string;
	/** system_settings key prefix for this tool's subdomain. */
	settingsKey: string;
	/** Feature flag key that toggles the tool on/off (e.g. 'checklist.enabled').
	 *  Read from the /feature-flags endpoint (data.features), NOT system_settings. */
	enabledSetting: string;
	/** SEO keywords for the tool page. */
	keywords: string[];
}

/**
 * The six identity tools, in display order for the hub grid.
 * Order is structural (defined by §78), not configurable.
 */
export const TOOLS: ToolMeta[] = [
	{
		slug: 'visa-checklist',
		route: '/visa-checklist',
		icon: 'file-check-2',
		name: 'Visa Document Checklist',
		nameKey: 'tool.visa_checklist.name',
		tagline: 'Know exactly what documents you need',
		taglineKey: 'tool.visa_checklist.tagline',
		description:
			'Select your origin, destination, and visa type to get a personalized, checkable document list with progress tracking.',
		descriptionKey: 'tool.visa_checklist.description',
		accent: '#3b49df',
		accentHover: '#2f3bc0',
		settingsKey: 'checklist',
		enabledSetting: 'checklist.enabled',
		keywords: ['visa', 'documents', 'checklist', 'immigration', 'japa']
	},
	{
		slug: 'readiness',
		route: '/readiness',
		icon: 'clipboard-check',
		name: 'Japa Readiness Assessment',
		nameKey: 'tool.readiness.name',
		tagline: 'How ready are you to japa?',
		taglineKey: 'tool.readiness.tagline',
		description:
			'A scored questionnaire across financial, logistical, and personal dimensions. Get a readiness score, threshold label, and a personalized roadmap.',
		descriptionKey: 'tool.readiness.description',
		accent: '#0ea5e9',
		accentHover: '#0284c7',
		settingsKey: 'assessment',
		enabledSetting: 'assessment.enabled',
		keywords: ['readiness', 'assessment', 'japa', 'score', 'roadmap']
	},
	{
		slug: 'cost-of-living',
		route: '/cost-of-living',
		icon: 'calculator',
		name: 'Cost of Living Calculator',
		nameKey: 'tool.cost_of_living.name',
		tagline: 'Compare cities before you move',
		taglineKey: 'tool.cost_of_living.tagline',
		description:
			'Compare cost of living between your destination cities and the Lagos baseline. Rent, groceries, transport, utilities, and more.',
		descriptionKey: 'tool.cost_of_living.description',
		accent: '#22c55e',
		accentHover: '#16a34a',
		settingsKey: 'col_calculator',
		enabledSetting: 'col_calculator.enabled',
		keywords: ['cost of living', 'calculator', 'city comparison', 'rent', 'expenses']
	},
	{
		slug: 'scholarships',
		route: '/scholarships',
		icon: 'graduation-cap',
		name: 'Scholarship Finder',
		nameKey: 'tool.scholarships.name',
		tagline: 'Find funding for your studies',
		taglineKey: 'tool.scholarships.tagline',
		description:
			'Filter scholarships by country, level of study, award amount, and deadline. Save scholarships to your tracker and get deadline reminders.',
		descriptionKey: 'tool.scholarships.description',
		accent: '#f59e0b',
		accentHover: '#d97706',
		settingsKey: 'scholarship_finder',
		enabledSetting: 'scholarship_finder.enabled',
		keywords: ['scholarships', 'funding', 'education', 'grants', 'study abroad']
	},
	{
		slug: 'remittance',
		route: '/remittance',
		icon: 'arrow-left-right',
		name: 'Remittance Rate Tracker',
		nameKey: 'tool.remittance.name',
		tagline: 'Best rates for sending money home',
		taglineKey: 'tool.remittance.tagline',
		description:
			'Compare live remittance rates across providers, view historical trends, calculate transfer costs, and set rate alerts.',
		descriptionKey: 'tool.remittance.description',
		accent: '#ec4899',
		accentHover: '#db2777',
		settingsKey: 'remittance',
		enabledSetting: 'remittance.enabled',
		keywords: ['remittance', 'exchange rate', 'transfer', 'send money', 'NGN']
	},
	{
		slug: 'japabot',
		route: '/japabot',
		icon: 'bot',
		name: 'JapaBot',
		nameKey: 'tool.japabot.name',
		tagline: 'Your AI japa guide',
		taglineKey: 'tool.japabot.tagline',
		description:
			'Chat with JapaBot — answers grounded in JapaConnect\'s published guides, forum threads, and community knowledge via RAG.',
		descriptionKey: 'tool.japabot.description',
		accent: '#8b5cf6',
		accentHover: '#7c3aed',
		settingsKey: 'japabot',
		enabledSetting: 'feature.japabot',
		keywords: ['japabot', 'AI', 'chat', 'RAG', 'assistant', 'japa guide']
	},
	// --- V2 functional tools (§§90–97, §§99–101) — geo-aware tool pattern ---
	{
		slug: 'visa-routes',
		route: '/visa-routes',
		icon: 'route',
		name: 'Visa Route Comparison',
		nameKey: 'tool.visa_routes.name',
		tagline: 'Compare visa routes side by side',
		taglineKey: 'tool.visa_routes.tagline',
		description:
			'Compare visa routes for your destination — eligibility, fees, path to PR, and renewal terms — side by side.',
		descriptionKey: 'tool.visa_routes.description',
		accent: '#2563eb',
		accentHover: '#1d4ed8',
		settingsKey: 'visa_routes',
		enabledSetting: 'visa_routes.enabled',
		keywords: ['visa', 'routes', 'comparison', 'PR', 'renewal', 'immigration']
	},
	{
		slug: 'embassy-alerter',
		route: '/embassy-alerter',
		icon: 'bell-ring',
		name: 'Embassy Appointment Alerter',
		nameKey: 'tool.embassy_alerter.name',
		tagline: 'Get notified when slots open',
		taglineKey: 'tool.embassy_alerter.tagline',
		description:
			'Subscribe to embassy appointment availability alerts. Get notified when a slot opens in your date range.',
		descriptionKey: 'tool.embassy_alerter.description',
		accent: '#dc2626',
		accentHover: '#b91c1c',
		settingsKey: 'appointment_alert',
		enabledSetting: 'appointment_alert.enabled',
		keywords: ['embassy', 'appointment', 'alert', 'visa slot', 'notification']
	},
	{
		slug: 'arrival-checklist',
		route: '/arrival-checklist',
		icon: 'plane-landing',
		name: 'Arrival Checklist',
		nameKey: 'tool.arrival_checklist.name',
		tagline: 'Everything to do after you land',
		taglineKey: 'tool.arrival_checklist.tagline',
		description:
			'A phased checklist of tasks to complete after arriving in your destination country — registration, banking, and more.',
		descriptionKey: 'tool.arrival_checklist.description',
		accent: '#0891b2',
		accentHover: '#0e7490',
		settingsKey: 'arrival_checklist',
		enabledSetting: 'arrival_checklist.enabled',
		keywords: ['arrival', 'checklist', 'post-arrival', 'registration', 'settling in']
	},
	{
		slug: 'processing-times',
		route: '/processing-times',
		icon: 'timer',
		name: 'Visa Processing Time Tracker',
		nameKey: 'tool.processing_tracker.name',
		tagline: 'How long will your visa take?',
		taglineKey: 'tool.processing_tracker.tagline',
		description:
			'View visa processing time statistics and trends — average, median, and percentile wait times for your corridor.',
		descriptionKey: 'tool.processing_tracker.description',
		accent: '#7c3aed',
		accentHover: '#6d28d9',
		settingsKey: 'visa_timeline',
		enabledSetting: 'visa_timeline.enabled',
		keywords: ['processing time', 'visa', 'wait time', 'timeline', 'statistics']
	},
	{
		slug: 'credential-checker',
		route: '/credential-checker',
		icon: 'badge-check',
		name: 'Professional Credential Checker',
		nameKey: 'tool.credential_checker.name',
		tagline: 'Will your qualification be recognised?',
		taglineKey: 'tool.credential_checker.tagline',
		description:
			'Look up credential recognition pathways for your profession — regulatory body, timeline, cost, and steps.',
		descriptionKey: 'tool.credential_checker.description',
		accent: '#059669',
		accentHover: '#047857',
		settingsKey: 'credential_checker',
		enabledSetting: 'credential_checker.enabled',
		keywords: ['credentials', 'qualifications', 'professional', 'regulatory body', 'recognition']
	},
	{
		slug: 'cv-formatter',
		route: '/cv-formatter',
		icon: 'file-text',
		name: 'CV Formatter',
		nameKey: 'tool.cv_formatter.name',
		tagline: 'Format your CV for your destination',
		taglineKey: 'tool.cv_formatter.tagline',
		description:
			'Get destination-specific CV standards and AI-assisted formatting — photo policy, sections, tone, and common pitfalls.',
		descriptionKey: 'tool.cv_formatter.description',
		accent: '#9333ea',
		accentHover: '#7e22ce',
		settingsKey: 'cv_formatter',
		enabledSetting: 'cv_formatter.enabled',
		keywords: ['CV', 'resume', 'format', 'standards', 'job application']
	},
	{
		slug: 'housing-guide',
		route: '/housing-guide',
		icon: 'home',
		name: 'Housing Guide',
		nameKey: 'tool.housing_guide.name',
		tagline: 'Find safe housing abroad',
		taglineKey: 'tool.housing_guide.tagline',
		description:
			'Destination-specific housing guides — platforms, landlord requirements, rent ranges, scam categories, and tips.',
		descriptionKey: 'tool.housing_guide.description',
		accent: '#ca8a04',
		accentHover: '#a16207',
		settingsKey: 'housing_guide',
		enabledSetting: 'housing_guide.enabled',
		keywords: ['housing', 'rent', 'accommodation', 'landlord', 'scam', 'moving']
	},
	{
		slug: 'attestation-guide',
		route: '/attestation-guide',
		icon: 'stamp',
		name: 'Attestation Guide',
		nameKey: 'tool.attestation_guide.name',
		tagline: 'Document attestation made clear',
		taglineKey: 'tool.attestation_guide.tagline',
		description:
			'Step-by-step document attestation pathways — apostille requirements, steps, fees, and community-verified updates.',
		descriptionKey: 'tool.attestation_guide.description',
		accent: '#b45309',
		accentHover: '#92400e',
		settingsKey: 'attestation_guide',
		enabledSetting: 'attestation_guide.enabled',
		keywords: ['attestation', 'apostille', 'document', 'legalisation', 'authentication']
	},
	{
		slug: 'language-test',
		route: '/language-test',
		icon: 'languages',
		name: 'Language Test Hub',
		nameKey: 'tool.language_test.name',
		tagline: 'Meet the language requirement',
		taglineKey: 'tool.language_test.tagline',
		description:
			'Find required language tests for your visa, minimum scores, prep resources, and community score stories.',
		descriptionKey: 'tool.language_test.description',
		accent: '#0d9488',
		accentHover: '#0f766e',
		settingsKey: 'language_test_hub',
		enabledSetting: 'language_test_hub.enabled',
		keywords: ['language test', 'IELTS', 'TOEFL', 'requirements', 'score', 'prep']
	},
	{
		slug: 'tax-calculator',
		route: '/tax-calculator',
		icon: 'receipt',
		name: 'Tax Calculator',
		nameKey: 'tool.tax_calculator.name',
		tagline: 'Understand your tax obligations',
		taglineKey: 'tool.tax_calculator.tagline',
		description:
			'Destination-specific tax guides — residency triggers, worldwide income rules, double taxation treaties, and checklists.',
		descriptionKey: 'tool.tax_calculator.description',
		accent: '#4f46e5',
		accentHover: '#4338ca',
		settingsKey: 'tax_calculator',
		enabledSetting: 'tax_calculator.enabled',
		keywords: ['tax', 'calculator', 'double taxation', 'treaty', 'residency', 'income']
	},
	{
		slug: 'community-locator',
		route: '/community-locator',
		icon: 'map-pin',
		name: 'Diaspora Community Locator',
		nameKey: 'tool.community_locator.name',
		tagline: 'Find your community abroad',
		taglineKey: 'tool.community_locator.tagline',
		description:
			'Find diaspora organisations near your destination — cultural groups, religious communities, and social clubs.',
		descriptionKey: 'tool.community_locator.description',
		accent: '#e11d48',
		accentHover: '#be123c',
		settingsKey: 'community_locator',
		enabledSetting: 'community_locator.enabled',
		keywords: ['community', 'diaspora', 'organisations', 'cultural', 'social', 'locator']
	},
	// --- SEO growth tools (§§107–110, ADR-118..121) ---
	{
		slug: 'crs-calculator',
		route: '/crs-calculator',
		icon: 'gauge',
		name: 'CRS Calculator',
		nameKey: 'tool.crs_calculator.name',
		tagline: 'Your Express Entry score in seconds',
		taglineKey: 'tool.crs_calculator.tagline',
		description:
			'Calculate your Comprehensive Ranking System score against the live IRCC ruleset, see the full factor breakdown, and track recent draw cut-offs.',
		descriptionKey: 'tool.crs_calculator.description',
		accent: '#ef4444',
		accentHover: '#dc2626',
		settingsKey: 'crs_calculator',
		enabledSetting: 'crs_calculator.enabled',
		keywords: ['CRS calculator', 'Express Entry', 'Canada immigration', 'CRS score', 'ITA']
	},
	{
		slug: 'pof-calculator',
		route: '/pof-calculator',
		icon: 'wallet',
		name: 'Proof of Funds Calculator',
		nameKey: 'tool.pof_calculator.name',
		tagline: 'Exactly how much you need to show',
		taglineKey: 'tool.pof_calculator.tagline',
		description:
			'Work out the proof-of-funds requirement for your destination and family size — with live NGN and USD equivalents.',
		descriptionKey: 'tool.pof_calculator.description',
		accent: '#0ea5e9',
		accentHover: '#0284c7',
		settingsKey: 'pof_calculator',
		enabledSetting: 'pof_calculator.enabled',
		keywords: ['proof of funds', 'POF', 'settlement funds', 'visa funds', 'Canada POF']
	},
	{
		slug: 'eca-guide',
		route: '/eca-guide',
		icon: 'file-badge',
		name: 'ECA Evaluation Guide',
		nameKey: 'tool.eca_guide.name',
		tagline: 'WES, IQAS and more — step by step',
		taglineKey: 'tool.eca_guide.tagline',
		description:
			'Compare credential evaluation providers, follow the step-by-step guide for your destination, and read real fee and timeline reports from members.',
		descriptionKey: 'tool.eca_guide.description',
		accent: '#8b5cf6',
		accentHover: '#7c3aed',
		settingsKey: 'eca_guide',
		enabledSetting: 'eca_guide.enabled',
		keywords: ['ECA', 'WES evaluation', 'credential assessment', 'IQAS', 'educational credential']
	},
	{
		slug: 'sop-generator',
		route: '/sop-generator',
		icon: 'pen-line',
		name: 'AI SOP Generator',
		nameKey: 'tool.sop_generator.name',
		tagline: 'Statement of purpose drafts that land',
		taglineKey: 'tool.sop_generator.tagline',
		description:
			'Generate a tailored statement of purpose draft from proven templates, browse real samples, and refine until it is ready.',
		descriptionKey: 'tool.sop_generator.description',
		accent: '#f97316',
		accentHover: '#ea580c',
		settingsKey: 'sop_generator',
		enabledSetting: 'sop_generator.enabled',
		keywords: ['SOP generator', 'statement of purpose', 'SOP sample', 'study permit SOP']
	}
];

/** Lookup a tool by its surface slug. */
export function getTool(slug: ToolSurface): ToolMeta | undefined {
	return TOOLS.find((t) => t.slug === slug);
}

/** Get all tools except 'hub' (which is the listing, not a tool). */
export function getAllTools(): ToolMeta[] {
	return TOOLS;
}

/**
 * Check whether a tool is enabled via its feature flag. Reads from the
 * /feature-flags map (data.features), NOT system_settings. Defaults to
 * `false` when the flag is absent — unknown features default to disabled
 * for safety, matching the backend features.DefaultFor behaviour.
 *
 * Ref: sections/42-feature-flags.md
 */
export function isToolEnabled(tool: ToolMeta, features: Record<string, boolean>): boolean {
	return features[tool.enabledSetting] === true;
}

/** Return only the enabled tools, given the current feature flags. */
export function getEnabledTools(features: Record<string, boolean>): ToolMeta[] {
	return TOOLS.filter((t) => isToolEnabled(t, features));
}
