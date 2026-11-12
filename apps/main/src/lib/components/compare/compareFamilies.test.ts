import { describe, expect, it } from 'vitest';
import {
	COMPARE_FAMILIES,
	familyForSlug,
	familyKeyFromSlug,
	familySlug,
	groupTypesByFamily
} from './compareFamilies.js';
import type { ComparisonType } from '#lib/types/jce.js';

const t = (id: string, slug: string, family?: string): ComparisonType =>
	({ id, slug, name: slug, config: family ? { family } : null }) as unknown as ComparisonType;

describe('compareFamilies', () => {
	it('maps family keys to URL slugs', () => {
		expect(familySlug('visa_routes')).toBe('visa-routes');
		expect(familySlug('location')).toBe('location');
		// cost-of-living is hyphenated in config.family already — no transform.
		expect(familySlug('cost-of-living')).toBe('cost-of-living');
	});

	it('maps slugs back to keys, honoring hyphenated keys', () => {
		expect(familyKeyFromSlug('visa-routes')).toBe('visa_routes');
		expect(familyKeyFromSlug('cost-of-living')).toBe('cost-of-living');
	});

	it('matcher set covers every family slug', () => {
		for (const f of COMPARE_FAMILIES) {
			expect(familyForSlug(f.slug)?.key).toBe(f.key);
		}
		expect(familyForSlug('not-a-family')).toBeUndefined();
	});

	it('groups types under their family, unknown families last into ungrouped', () => {
		const { groups, ungrouped } = groupTypesByFamily([
			t('1', 'countries', 'location'),
			t('2', 'states', 'location'),
			t('3', 'universities', 'education'),
			t('4', 'mentors') // no family
		]);
		expect(groups.map((g) => g.family.key)).toEqual(['location', 'education']);
		expect(groups[0].types.map((x) => x.slug)).toEqual(['countries', 'states']);
		expect(ungrouped.map((x) => x.slug)).toEqual(['mentors']);
	});

	it('keeps COMPARE_FAMILIES order regardless of input order', () => {
		const { groups } = groupTypesByFamily([
			t('1', 'universities', 'education'),
			t('2', 'countries', 'location')
		]);
		expect(groups.map((g) => g.family.key)).toEqual(['location', 'education']);
	});
});
