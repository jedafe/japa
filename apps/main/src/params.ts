import type { ParamMatcher } from '@sveltejs/kit/params';

// Keep in sync with COMPARE_FAMILIES slugs in
// src/lib/components/compare/compareFamilies.ts (kit matchers can't import app modules).
const compareCategorySlugs = new Set([
	'location',
	'education',
	'visa-routes',
	'banking',
	'cost-of-living',
	'embassies',
	'travel',
	'jobs'
]);

function slugMatcher(values: ReadonlySet<string>, message: string): ParamMatcher<string> {
	return {
		'~standard': {
			version: 1,
			vendor: 'japaconnect',
			validate(value: unknown) {
				return typeof value === 'string' && values.has(value)
					? { value }
					: { issues: [{ message }] };
			}
		}
	};
}

export const params = {
	compareCategory: slugMatcher(compareCategorySlugs, 'Unknown comparison category')
};
