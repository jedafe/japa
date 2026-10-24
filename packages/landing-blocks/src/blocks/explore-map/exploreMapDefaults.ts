// Ref: japaconnect-engineering/sections/16-page-builder.md (explore_map)
import type { ExploreMapProps } from '../../types';

/** Default props for `explore_map` — a fresh block (props: {}) merges over
 * these so it renders the full explorer out of the box. Shared by the block,
 * the content editor, and admin seeding. */
export const EXPLORE_MAP_DEFAULTS: ExploreMapProps = {
	headline: 'Explore the World. Plan Your Move.',
	subtitle: 'Discover countries and cities with the best opportunities for your journey.',
	show_toolbar: true,
	show_legend: true,
	show_stats: true,
	show_region_filter: true,
	show_col_index: true,
	show_zoom: true,
	geo_source: 'api',
	pin_limit: 8,
	default_currency: 'USD',
	stats_position: 'overlay',
	views: [
		{ key: 'destinations', label: 'Destinations', source: 'countries' },
		{ key: 'cities', label: 'Cities', source: 'col_cities' },
		{ key: 'universities', label: 'Universities', source: 'universities' },
	],
	categories: [
		{ key: 'opportunities', icon: 'Briefcase', label: 'Best for Opportunities', sublabel: 'Great for career & business', color: '#4a8df6' },
		{ key: 'study', icon: 'GraduationCap', label: 'Best for Study', sublabel: 'Top study destinations', color: '#4caf7d' },
		{ key: 'quality', icon: 'Heart', label: 'Best for Quality of Life', sublabel: 'High livability & safety', color: '#9b7fe8' },
		{ key: 'affordable', icon: 'DollarSign', label: 'Affordable Living', sublabel: 'Low cost of living', color: '#f0b73a' },
		{ key: 'emerging', icon: 'Star', label: 'Emerging Destinations', sublabel: 'Rising opportunities', color: '#2fb3a3' },
	],
	stats: [
		{ icon: 'Globe', label: 'Countries', source: 'countries' },
		{ icon: 'Building2', label: 'Cities', source: 'col_cities', suffix: '+' },
		{ icon: 'BookOpen', label: 'Guides', source: 'blog', suffix: '+' },
		{ icon: 'Handshake', label: 'Services', source: 'services', suffix: '+' },
	],
	col_bands: [
		{ key: 'very_affordable', label: 'Very Affordable', min: 0, max: 1000, color: '#6abf69' },
		{ key: 'affordable', label: 'Affordable', min: 1001, max: 2000, color: '#f0b73a' },
		{ key: 'moderate', label: 'Moderate', min: 2001, max: 3500, color: '#6ba3e8' },
		{ key: 'expensive', label: 'Expensive', min: 3501, max: 5000, color: '#9b7fe8' },
		{ key: 'very_expensive', label: 'Very Expensive', min: 5001, max: null, color: '#f27ba0' },
	],
	region_bands: {
		'Northern America': 'moderate',
		'Central America': 'affordable',
		Caribbean: 'affordable',
		'South America': 'very_affordable',
		Europe: 'expensive',
		Africa: 'affordable',
		Asia: 'very_affordable',
		Oceania: 'very_expensive',
	},
	pins: [],
};
