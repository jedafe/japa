import { describe, expect, it } from 'vitest';
import {
	DEFAULT_CHAT_TEMPLATE_VALUES,
	buildTemplateBlock,
	buildTemplateInsertText,
	buildTemplateSpec,
	normalizeTemplateFolder,
	normalizeTemplateValues,
	normalizeTopicsTail,
	splitComposerForInsert,
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
	it('renders readable constraint lines ending in a Topics section', () => {
		const v = base();
		v.min_words = 1500;
		v.include_faqs = true;
		v.external_links = 2;
		const text = buildTemplateInsertText(v);
		expect(text).toContain('at least 1500 words');
		expect(text).toContain('FAQ section');
		expect(text).toContain('2 authoritative external links');
		expect(text.trimEnd().endsWith('Topics (one per line — one draft is generated per topic):')).toBe(
			true
		);
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

	it('emits non-default publishing fields and id labels', () => {
		const v = base();
		v.status = 'published';
		v.visibility = 'members';
		v.seo_indexable = false;
		v.publish_at = '2026-02-01T10:00';
		v.space_id = 's1';
		v.series_id = 'r1';
		v.voice_id = 'v1';
		v.field_values = { deadline: 'March 1' };
		const text = buildTemplateInsertText(v, {
			space: 'Nigeria Hub',
			series: 'Student Series',
			voice: 'Professional Ada'
		});
		expect(text).toContain('Publishing: status published, members visibility, not indexable');
		expect(text).toContain('Publish at: 2026-02-01T10:00');
		expect(text).toContain('Space: Nigeria Hub');
		expect(text).toContain('Series: Student Series');
		expect(text).toContain('Voice: Professional Ada');
		expect(text).toContain('Field values: deadline = March 1');
	});

	it('falls back to raw ids and omits default publishing fields', () => {
		const v = base();
		v.space_id = 's1';
		const text = buildTemplateInsertText(v);
		expect(text).toContain('Space: s1');
		expect(text).not.toContain('Publishing:');
	});
});

describe('normalizeTemplateFolder', () => {
	it('trims segments and collapses empty parts', () => {
		expect(normalizeTemplateFolder(' visa / guides /')).toBe('visa/guides');
		expect(normalizeTemplateFolder('//a//b//')).toBe('a/b');
		expect(normalizeTemplateFolder('')).toBe('');
		expect(normalizeTemplateFolder(undefined)).toBe('');
		expect(normalizeTemplateFolder(42)).toBe('');
	});
});

describe('splitComposerForInsert', () => {
	const spec = () => buildTemplateInsertText(base());

	it('extracts the topics tail from a prior insert', () => {
		const input = spec() + '\n- UK visa\n- Canada PR';
		expect(splitComposerForInsert(input)).toBe('- UK visa\n- Canada PR');
	});

	it('extracts the draft from a legacy Topic: insert', () => {
		const input = 'Write a guide draft with these requirements:\n- Tone: professional\n\nTopic: UK visa';
		expect(splitComposerForInsert(input)).toBe('UK visa');
	});

	it('treats free text as the tail', () => {
		expect(splitComposerForInsert('  UK visa checklist  ')).toBe('UK visa checklist');
	});

	it('never drops text — a marker-stripped spec becomes the tail', () => {
		const input = 'Write one guide draft per topic, with these requirements:\n- Tone: x';
		expect(splitComposerForInsert(input)).toBe(input);
	});
});

describe('normalizeTopicsTail', () => {
	it('bulletizes a single plain line, keeps an existing bullet', () => {
		expect(normalizeTopicsTail('UK visa')).toBe('- UK visa');
		expect(normalizeTopicsTail('- UK visa')).toBe('- UK visa');
	});

	it('passes multi-line text through verbatim', () => {
		expect(normalizeTopicsTail('- a\nb\n- c')).toBe('- a\nb\n- c');
		expect(normalizeTopicsTail('a paragraph\nwith a second line')).toBe(
			'a paragraph\nwith a second line'
		);
	});

	it('returns an empty bullet for an empty tail', () => {
		expect(normalizeTopicsTail('')).toBe('- ');
		expect(normalizeTopicsTail('\n\n')).toBe('- ');
	});
});
