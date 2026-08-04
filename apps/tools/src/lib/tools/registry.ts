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
	/** system_settings key that toggles the tool on/off (e.g. 'checklist.enabled'). */
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
		enabledSetting: 'ai.japabot_enabled',
		keywords: ['japabot', 'AI', 'chat', 'RAG', 'assistant', 'japa guide']
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
 * Check whether a tool is enabled via its `system_settings` toggle
 * (e.g. `checklist.enabled`). Defaults to `true` when the setting is
 * absent — tools are opt-out, not opt-in.
 *
 * Ref: conventions/system-settings-catalog.md ({tool}.enabled keys),
 *      Rule 3 (config from system_settings, never hardcoded).
 */
export function isToolEnabled(tool: ToolMeta, settings: Record<string, unknown>): boolean {
	const value = settings[tool.enabledSetting];
	// Booleans come through as-is; the backend stores these as bool settings.
	// Absent or non-boolean values default to enabled (safe default).
	if (typeof value === 'boolean') return value;
	if (value === 'true') return true;
	if (value === 'false') return false;
	return true;
}

/** Return only the enabled tools, given the current site settings. */
export function getEnabledTools(settings: Record<string, unknown>): ToolMeta[] {
	return TOOLS.filter((t) => isToolEnabled(t, settings));
}
