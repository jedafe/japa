import { describe, it, expect } from 'vitest';
import { validateFrontmatter, validateBody, isHighStakes, isHighStakesCategory } from './validation';

describe('isHighStakesCategory', () => {
	it('recognizes configured high-stakes categories', () => {
		expect(isHighStakesCategory('visa')).toBe(true);
		expect(isHighStakesCategory('Banking')).toBe(true);
		expect(isHighStakesCategory('import_export')).toBe(true);
		expect(isHighStakesCategory('relocation')).toBe(false);
	});
});

describe('isHighStakes', () => {
	it('flags high-stakes guide categories', () => {
		expect(isHighStakes({ guide_category: 'health_insurance' })).toBe(true);
	});

	it('returns false for non-high-stakes categories', () => {
		expect(isHighStakes({ guide_category: 'relocation' })).toBe(false);
		expect(isHighStakes({ guide_category: 'jobs' })).toBe(false);
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
				publish_at: new Date(Date.now() - 60 * 60 * 1000).toISOString()
			},
			'scheduled'
		);
		expect(past.publish_at).toContain('future');
	});

	it('requires space and persona for non-draft operator content', () => {
		const errors = validateFrontmatter(
			{
				title: 'Post',
				slug: 'post',
				content_type: 'guide',
				status: 'published',
				visibility: 'public'
			},
			'published',
			'operator'
		);
		expect(errors.space_id).toBeTruthy();
		expect(errors.persona_id).toBeTruthy();
	});
});

describe('validateBody', () => {
	it('skips draft bodies', () => {
		expect(validateBody('', 'draft')).toBeUndefined();
	});

	it('requires a body for published content', () => {
		expect(validateBody('', 'published')).toContain('required');
	});

	it('enforces minimum length for published content', () => {
		expect(validateBody('short', 'published')).toContain('too short');
	});
});
