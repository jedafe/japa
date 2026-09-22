/**
 * Main-app tool registry — structural metadata for every tool surfaced on the
 * `/tools` hub (§67, §69–74, §§90–97, §§99–101). Each tool's availability is
 * gated by its own `{tool}.enabled` system_setting toggle, read from the root
 * layout's `data.siteSettings`. This is scaffold metadata only — all dynamic
 * tool content comes from the backend API (Rule 16).
 *
 * Tools are grouped into six lifecycle categories that follow the japa journey:
 * Planning → Immigration → Education → Career → Finance → Settlement.
 * A tool may appear in multiple categories when it spans journey stages
 * (e.g. Language Test Hub is both Immigration and Education).
 *
 * Ref: §78 (Tools Web Surface), conventions/system-settings-catalog.md.
 */
import { toolsUrl } from '$lib/appOrigins';

export type ToolSection = 'planning' | 'immigration' | 'education' | 'career' | 'finance' | 'settlement';

export interface MainToolMeta {
	/** Route path on the main app. */
	href: string;
	/** i18n key for the display name. */
	labelKey: string;
	/** i18n key for the short description. */
	descKey: string;
	/** Feature flag key that toggles the tool on/off (e.g. 'checklist.enabled').
	 *  Read from the /feature-flags endpoint (data.features), NOT system_settings. */
	enabledSetting: string;
	/** Hub categories the tool belongs to (primary first, then secondary).
	 *  A tool appears under every category listed here. */
	sections: ToolSection[];
}

/** Display order of the six categories on the hub. */
export const TOOL_SECTIONS: ToolSection[] = [
	'planning',
	'immigration',
	'education',
	'career',
	'finance',
	'settlement'
];

/**
 * The full tool set. Each tool lists every category it belongs to — the first
 * entry is the primary category (used for ordering within a section), secondary
 * entries place the tool under additional journey stages. v1 identity tools
 * (§67, §69–74) and v2 geo-aware functional tools (§§90–97, §§99–101) are
 * interleaved by category, not by version.
 */
export const MAIN_TOOLS: MainToolMeta[] = [
	// --- Planning — assess readiness and plan the move ---
	{
		href: '/tools/readiness',
		labelKey: 'tools.readiness',
		descKey: 'tools.readiness_desc',
		enabledSetting: 'assessment.enabled',
		sections: ['planning', 'finance']
	},
	{
		href: '/ai/chat',
		labelKey: 'tools.japabot',
		descKey: 'tools.japabot_desc',
		enabledSetting: 'feature.japabot',
		sections: ['planning', 'immigration']
	},
	{
		href: '/tools/travel',
		labelKey: 'tools.travel',
		descKey: 'tools.travel_desc',
		enabledSetting: 'travel.enabled',
		sections: ['planning', 'settlement']
	},
	// --- Immigration — visas, embassies, attestation, language tests ---
	{
		href: '/tools/visa-checklist',
		labelKey: 'tools.visa_checklist',
		descKey: 'tools.visa_checklist_desc',
		enabledSetting: 'checklist.enabled',
		sections: ['immigration', 'planning']
	},
	{
		href: '/tools/visa-routes',
		labelKey: 'tools.visa_routes',
		descKey: 'tools.visa_routes_desc',
		enabledSetting: 'visa_routes.enabled',
		sections: ['immigration', 'planning']
	},
	{
		href: '/tools/embassy-alerter',
		labelKey: 'tools.embassy_alerter',
		descKey: 'tools.embassy_alerter_desc',
		enabledSetting: 'appointment_alert.enabled',
		sections: ['immigration', 'planning']
	},
	{
		href: '/tools/processing-tracker',
		labelKey: 'tools.processing_tracker',
		descKey: 'tools.processing_tracker_desc',
		enabledSetting: 'visa_timeline.enabled',
		sections: ['immigration', 'planning']
	},
	{
		href: '/tools/attestation-guide',
		labelKey: 'tools.attestation_guide',
		descKey: 'tools.attestation_guide_desc',
		enabledSetting: 'attestation_guide.enabled',
		sections: ['immigration', 'planning']
	},
	{
		href: '/tools/language-test',
		labelKey: 'tools.language_test',
		descKey: 'tools.language_test_desc',
		enabledSetting: 'language_test_hub.enabled',
		sections: ['immigration', 'education']
	},
	// --- Education — scholarships, study funding, language tests ---
	{
		href: '/scholarships',
		labelKey: 'tools.scholarships',
		descKey: 'tools.scholarships_desc',
		enabledSetting: 'scholarship_finder.enabled',
		sections: ['education', 'finance']
	},
	// --- Career — credential recognition and CV formatting ---
	{
		href: '/tools/credential-checker',
		labelKey: 'tools.credential_checker',
		descKey: 'tools.credential_checker_desc',
		enabledSetting: 'credential_checker.enabled',
		sections: ['career', 'immigration']
	},
	{
		href: '/tools/cv-formatter',
		labelKey: 'tools.cv_formatter',
		descKey: 'tools.cv_formatter_desc',
		enabledSetting: 'cv_formatter.enabled',
		sections: ['career', 'education']
	},
	// --- Finance — cost of living, remittance, tax ---
	{
		href: '/tools/cost-of-living',
		labelKey: 'tools.cost_of_living',
		descKey: 'tools.cost_of_living_desc',
		enabledSetting: 'col_calculator.enabled',
		sections: ['finance', 'planning']
	},
	{
		href: '/tools/remittance',
		labelKey: 'tools.remittance',
		descKey: 'tools.remittance_desc',
		enabledSetting: 'remittance.enabled',
		sections: ['finance', 'settlement']
	},
	{
		href: '/tools/tax-calculator',
		labelKey: 'tools.tax_calculator',
		descKey: 'tools.tax_calculator_desc',
		enabledSetting: 'tax_calculator.enabled',
		sections: ['finance', 'settlement']
	},
	// --- §§107–110 functional tools — hosted on the Tools surface
	// (tools.japaconnect.com), linked out via toolsUrl(). Categories are
	// journey-stage based, same as the in-app tools above. ---
	{
		href: toolsUrl('/crs-calculator'),
		labelKey: 'tools.crs_calculator',
		descKey: 'tools.crs_calculator_desc',
		enabledSetting: 'crs_calculator.enabled',
		sections: ['immigration', 'planning']
	},
	{
		href: toolsUrl('/pof-calculator'),
		labelKey: 'tools.pof_calculator',
		descKey: 'tools.pof_calculator_desc',
		enabledSetting: 'pof_calculator.enabled',
		sections: ['finance', 'immigration']
	},
	{
		href: toolsUrl('/eca-guide'),
		labelKey: 'tools.eca_guide',
		descKey: 'tools.eca_guide_desc',
		enabledSetting: 'eca_guide.enabled',
		sections: ['education', 'career', 'immigration']
	},
	{
		href: toolsUrl('/sop-generator'),
		labelKey: 'tools.sop_generator',
		descKey: 'tools.sop_generator_desc',
		enabledSetting: 'sop_generator.enabled',
		sections: ['education', 'career']
	},
	// --- Settlement — arrival, housing, community ---
	{
		href: '/tools/arrival-checklist',
		labelKey: 'tools.arrival_checklist',
		descKey: 'tools.arrival_checklist_desc',
		enabledSetting: 'arrival_checklist.enabled',
		sections: ['settlement', 'planning']
	},
	{
		href: '/tools/housing-guide',
		labelKey: 'tools.housing_guide',
		descKey: 'tools.housing_guide_desc',
		enabledSetting: 'housing_guide.enabled',
		sections: ['settlement', 'planning']
	},
	{
		href: '/tools/community-locator',
		labelKey: 'tools.community_locator',
		descKey: 'tools.community_locator_desc',
		enabledSetting: 'community_locator.enabled',
		sections: ['settlement', 'planning']
	}
];

/**
 * Check whether a tool is enabled via its feature flag. Reads from the
 * /feature-flags map (data.features), NOT system_settings. Defaults to
 * `false` when the flag is absent — unknown features default to disabled
 * for safety, matching the backend features.DefaultFor behaviour.
 */
export function isMainToolEnabled(
	tool: MainToolMeta,
	features: Record<string, boolean>
): boolean {
	return features[tool.enabledSetting] === true;
}

/** Return only the enabled tools that belong to a given section (primary or secondary). */
export function getEnabledToolsBySection(
	features: Record<string, boolean>,
	section: ToolSection
): MainToolMeta[] {
	return MAIN_TOOLS.filter(
		(t) => t.sections.includes(section) && isMainToolEnabled(t, features)
	);
}
