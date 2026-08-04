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
	/** One-line description for the hub card. */
	tagline: string;
	/** Longer description for SEO / hub card body. */
	description: string;
	/** Accent color for the single-tool shell (hex). */
	accent: string;
	/** Accent hover color. */
	accentHover: string;
	/** system_settings key prefix for this tool's subdomain. */
	settingsKey: string;
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
		tagline: 'Know exactly what documents you need',
		description:
			'Select your origin, destination, and visa type to get a personalized, checkable document list with progress tracking.',
		accent: '#3b49df',
		accentHover: '#2f3bc0',
		settingsKey: 'checklist',
		keywords: ['visa', 'documents', 'checklist', 'immigration', 'japa']
	},
	{
		slug: 'readiness',
		route: '/readiness',
		icon: 'clipboard-check',
		name: 'Japa Readiness Assessment',
		tagline: 'How ready are you to japa?',
		description:
			'A scored questionnaire across financial, logistical, and personal dimensions. Get a readiness score, threshold label, and a personalized roadmap.',
		accent: '#0ea5e9',
		accentHover: '#0284c7',
		settingsKey: 'assessment',
		keywords: ['readiness', 'assessment', 'japa', 'score', 'roadmap']
	},
	{
		slug: 'cost-of-living',
		route: '/cost-of-living',
		icon: 'calculator',
		name: 'Cost of Living Calculator',
		tagline: 'Compare cities before you move',
		description:
			'Compare cost of living between your destination cities and the Lagos baseline. Rent, groceries, transport, utilities, and more.',
		accent: '#22c55e',
		accentHover: '#16a34a',
		settingsKey: 'col_calculator',
		keywords: ['cost of living', 'calculator', 'city comparison', 'rent', 'expenses']
	},
	{
		slug: 'scholarships',
		route: '/scholarships',
		icon: 'graduation-cap',
		name: 'Scholarship Finder',
		tagline: 'Find funding for your studies',
		description:
			'Filter scholarships by country, level of study, award amount, and deadline. Save scholarships to your tracker and get deadline reminders.',
		accent: '#f59e0b',
		accentHover: '#d97706',
		settingsKey: 'scholarship_finder',
		keywords: ['scholarships', 'funding', 'education', 'grants', 'study abroad']
	},
	{
		slug: 'remittance',
		route: '/remittance',
		icon: 'arrow-left-right',
		name: 'Remittance Rate Tracker',
		tagline: 'Best rates for sending money home',
		description:
			'Compare live remittance rates across providers, view historical trends, calculate transfer costs, and set rate alerts.',
		accent: '#ec4899',
		accentHover: '#db2777',
		settingsKey: 'remittance',
		keywords: ['remittance', 'exchange rate', 'transfer', 'send money', 'NGN']
	},
	{
		slug: 'japabot',
		route: '/japabot',
		icon: 'bot',
		name: 'JapaBot',
		tagline: 'Your AI japa guide',
		description:
			'Chat with JapaBot — answers grounded in JapaConnect\'s published guides, forum threads, and community knowledge via RAG.',
		accent: '#8b5cf6',
		accentHover: '#7c3aed',
		settingsKey: 'japabot',
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
