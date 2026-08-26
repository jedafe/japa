import { describe, it, expect } from 'vitest';
import { validateFrontmatter, validateBody, isHighStakes, isHighStakesCategory } from './validation';

describe('isHighStakesCategory', () => {
	it('recognizes configured high-stakes categories', () => {
		expect(isHighStakesCategory('visa')).toBe(true);
		expect(isHighStakesCategory('Banking')).toBe(true);
		expect(isHighStakesCategory('relocation')).toBe(false);
	});
});

describe('isHighStakes', () => {
	it('flags high-stakes guide categories', () => {
		expect(isHighStakes({ guide_category: 'health_insurance' })).toBe(true);
	});

	it('flags high-stakes tags', () => {
		expect(isHighStakes({ guide_category: 'relocation', tags: ['immigration-advice'] })).toBe(true);
		expect(isHighStakes({ guide_category: 'relocation', tags: 'financial-advice' })).toBe(true);
	});

	it('returns false for ordinary content', () => {
		expect(isHighStakes({ guide_category: 'relocation', tags: ['jobs'] })).toBe(false);
	});
});

describe('validateFrontmatter', () => {
	it('rejects published high-stakes content', () => {
		const errors = validateFrontmatter(
			{
				title: 'Visa Guide',
				slug: 'visa-guide',
				content_type: 'guide',
				guide_category: 'visa',
				status: 'published',
				visibility: 'public',
				space_id: 'space-1',
				persona_id: 'persona-1'
			},
			'published'
		);
		expect(errors.guide_category).toContain('High-stakes');
	});

	it('allows high-stakes content in review', () => {
		const errors = validateFrontmatter(
			{
				title: 'Visa Guide',
				slug: 'visa-guide',
				content_type: 'guide',
				guide_category: 'visa',
				status: 'in_review',
				visibility: 'public',
				space_id: 'space-1',
				persona_id: 'persona-1'
			},
			'in_review'
		);
		expect(errors.guide_category).toBeUndefined();
	});

	it('accepts members visibility', () => {
		const errors = validateFrontmatter(
			{
				title: 'Members Post',
				slug: 'members-post',
				content_type: 'guide',
				status: 'in_review',
				visibility: 'members'
			},
			'in_review'
		);
		expect(errors.visibility).toBeUndefined();
	});

	it('does not require persona_id in member mode for non-draft content', () => {
		const errors = validateFrontmatter(
			{
				title: 'Member Post',
				slug: 'member-post',
				content_type: 'guide',
				status: 'published',
				visibility: 'public',
				space_id: 'space-1'
			},
			'published',
			'member'
		);
		expect(errors.persona_id).toBeUndefined();
	});

	it('requires a future publish_at for scheduled status', () => {
		const future = new Date(Date.now() + 60 * 60 * 1000).toISOString();
		const ok = validateFrontmatter(
			{
				title: 'Scheduled',
				slug: 'scheduled',
				content_type: 'guide',
				status: 'scheduled',
				visibility: 'public',
				space_id: 'space-1',
				persona_id: 'persona-1',
				publish_at: future
			},
			'scheduled'
		);
		expect(ok.publish_at).toBeUndefined();

		const missing = validateFrontmatter(
			{
				title: 'Scheduled',
				slug: 'scheduled',
				content_type: 'guide',
				status: 'scheduled',
				visibility: 'public',
				space_id: 'space-1',
				persona_id: 'persona-1'
			},
			'scheduled'
		);
		expect(missing.publish_at).toContain('publish date');

		const past = validateFrontmatter(
			{
				title: 'Scheduled',
				slug: 'scheduled',
				content_type: 'guide',
				status: 'scheduled',
				visibility: 'public',
				space_id: 'space-1',
				persona_id: 'persona-1',
				publish_at: '2020-01-01T00:00:00Z'
			},
			'scheduled'
		);
		expect(past.publish_at).toContain('future');
	});
});

describe('validateBody', () => {
	it('rejects short bodies for published content', () => {
		expect(validateBody('Hi', 'published')).toContain('too short');
	});

	it('allows empty bodies for drafts', () => {
		expect(validateBody('', 'draft')).toBeUndefined();
		expect(validateBody('   ', 'draft')).toBeUndefined();
	});
});
