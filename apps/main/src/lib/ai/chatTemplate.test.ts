import { describe, expect, it } from 'vitest';
import {
	DEFAULT_CHAT_TEMPLATE_VALUES,
	buildTemplateBlock,
	buildTemplateInsertText,
	buildTemplateSpec,
	normalizeTemplateValues,
	type ChatTemplateValues
} from './chatTemplate';

const base = (): ChatTemplateValues => structuredClone(DEFAULT_CHAT_TEMPLATE_VALUES);

describe('normalizeTemplateValues', () => {
	it('fills defaults for missing keys', () => {
		const v = normalizeTemplateValues({ content_type: 'blog_post' });
		expect(v.content_type).toBe('blog_post');
		expect(v.framework).toBe('');
		expect(v.include_intro).toBe(true);
		expect(v.min_words).toBe(0);
	});

	it('rejects out-of-domain enum values', () => {
		const v = normalizeTemplateValues({ framework: 'bab', humanize_level: 'ultra' });
		expect(v.framework).toBe('');
		expect(v.humanize_level).toBe('medium');
	});

	it('clamps non-positive numbers to 0', () => {
		const v = normalizeTemplateValues({ min_words: -5, heading_count: 4.9 });
		expect(v.min_words).toBe(0);
		expect(v.heading_count).toBe(4);
	});

	it('normalizes journey/settings fields and drops malformed entries', () => {
		const v = normalizeTemplateValues({
			journey_stage: 'researching',
			tags: ['canada', 42, '  '],
			field_values: { roadmap_dimension: 'finance', bad: 7 },
			seo_indexable: false,
			status: 'draft'
		});
		expect(v.journey_stage).toBe('researching');
		expect(v.tags).toEqual(['canada']);
		expect(v.field_values).toEqual({ roadmap_dimension: 'finance' });
		expect(v.seo_indexable).toBe(false);
		expect(v.status).toBe('draft');
		// Absent → studio defaults.
		const d = normalizeTemplateValues({});
		expect(d.status).toBe('in_review');
		expect(d.visibility).toBe('public');
		expect(d.seo_indexable).toBe(true);
	});
});

describe('buildTemplateSpec', () => {
	it('emits the content.generate_draft argument shape', () => {
		const v = base();
		v.min_words = 1500;
		v.heading_count = 4;
		v.min_paragraphs_per_heading = 2;
		v.internal_links = 3;
		v.external_links = 2;
		v.framework = 'aida';
		v.include_faqs = true;
		v.audience = 'Nigerians relocating to Canada';

		const spec = buildTemplateSpec(v) as {
			content_type: string;
			tone: string;
			instructions: string;
			humanize: boolean;
			humanize_level: string;
			options: Record<string, unknown>;
		};

		expect(spec.content_type).toBe('guide');
		expect(spec.tone).toBe('professional');
		expect(spec.instructions).toContain('Nigerians relocating to Canada');
		expect(spec.humanize).toBe(true);
		expect(spec.humanize_level).toBe('medium');
		expect(spec.options).toMatchObject({
			mode: 'generate',
			framework: 'aida',
			min_words: 1500,
			heading_count: 4,
			min_paragraphs_per_heading: 2,
			internal_links: 3,
			external_links: 2,
			include_faqs: true,
			include_intro: true,
			include_steps: true,
			include_key_takeaways: true
		});
		expect(spec.options).not.toHaveProperty('include_inline_image');
	});

	it('omits humanize keys when the level is off', () => {
		const v = base();
		v.humanize_level = 'off';
		const spec = buildTemplateSpec(v);
		expect(spec).not.toHaveProperty('humanize');
		expect(spec).not.toHaveProperty('humanize_level');
	});

	it('emits journey + settings fields under context', () => {
		const v = base();
		v.journey_stage = 'preparing';
		v.visa_route = 'express_entry';
		v.source_country = 'Nigeria';
		v.target_country = 'Canada';
		v.space_id = 'space-1';
		v.series_id = 'series-1';
		v.tags = ['pr', 'settlement'];
		v.status = 'published';
		v.visibility = 'members';
		v.publish_at = '2026-03-01T09:00';
		v.field_values = { roadmap_dimension: 'finance' };
		v.seo_indexable = false;

		const spec = buildTemplateSpec(v) as { context: Record<string, unknown> };
		expect(spec.context).toMatchObject({
			journey_stage: 'preparing',
			visa_route: 'express_entry',
			source_country: 'Nigeria',
			target_country: 'Canada',
			space_id: 'space-1',
			series_id: 'series-1',
			tags: ['pr', 'settlement'],
			status: 'published',
			visibility: 'members',
			seo_indexable: false,
			publish_at: '2026-03-01T09:00',
			field_values: { roadmap_dimension: 'finance' }
		});
	});

	it('omits context when only publishing defaults are set', () => {
		const spec = buildTemplateSpec(base());
		expect(spec).not.toHaveProperty('context');
	});
});

describe('buildTemplateBlock', () => {
	it('embeds the spec as JSON inside the message suffix', () => {
		const block = buildTemplateBlock('My guide', base());
		expect(block).toContain('Chat template: "My guide"');
		expect(block).toContain('content.generate_draft');
		expect(block).toContain('"content_type": "guide"');
	});
});

describe('buildTemplateInsertText', () => {
	it('renders readable constraint lines ending in a Topic prompt', () => {
		const v = base();
		v.min_words = 1500;
		v.include_faqs = true;
		v.external_links = 2;
		const text = buildTemplateInsertText(v);
		expect(text).toContain('at least 1500 words');
		expect(text).toContain('FAQ section');
		expect(text).toContain('2 authoritative external links');
		expect(text.trimEnd().endsWith('Topic:')).toBe(true);
	});

	it('renders journey context and tags lines when set', () => {
		const v = base();
		v.journey_stage = 'preparing';
		v.source_country = 'Nigeria';
		v.target_country = 'Canada';
		v.tags = ['pr', 'settlement'];
		const text = buildTemplateInsertText(v);
		expect(text).toContain('"preparing" journey stage');
		expect(text).toContain('Nigeria → Canada');
		expect(text).toContain('pr, settlement');
	});
});
