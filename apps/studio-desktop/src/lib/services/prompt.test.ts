import * as yaml from 'js-yaml';
import { describe, it, expect } from 'vitest';
import { slugify, buildProjectInstruction, buildPrompt } from './prompt';
import { promptFromDefaults, DEFAULT_PROMPT_VALUES, type PromptFormValues } from '#lib/types/prompt.js';
import type { PromptLookups } from './prompt';
import type { StudioSettings } from './settings';

describe('slugify', () => {
	it('converts titles to lowercase slugs', () => {
		expect(slugify('Moving to Canada')).toBe('moving-to-canada');
	});

	it('removes special characters', () => {
		expect(slugify('Housing & Rent in Toronto!!!')).toBe('housing-rent-in-toronto');
	});

	it('trims leading and trailing hyphens', () => {
		expect(slugify('  --Hello World--  ')).toBe('hello-world');
	});
});

describe('promptFromDefaults', () => {
	it('applies defaults over DEFAULT_PROMPT_VALUES', () => {
		const defaults: StudioSettings = {
			projectName: '',
			baseUrl: '',
			apiKey: '',
			projectRoot: '',
			mode: 'operator',
			autoSync: false,
			autoSyncContentTypes: [],
			default_content_type: 'blog_post',
			default_guide_category: '',
			default_persona_id: 'p-123',
			default_voice_id: 'v-123',
			default_space_id: 's-123',
			default_status: 'in_review',
			default_visibility: 'private',
			default_tags: ['relocation']
		};
		const values = promptFromDefaults(defaults);
		expect(values.content_type).toBe('blog_post');
		expect(values.persona_id).toBe('p-123');
		expect(values.space_id).toBe('s-123');
		expect(values.status).toBe('in_review');
		expect(values.visibility).toBe('private');
		expect(values.tags).toEqual(['relocation']);
	});

	it('keeps fallbacks when no defaults are set', () => {
		const values = promptFromDefaults({});
		expect(values).toEqual(DEFAULT_PROMPT_VALUES);
	});
});

describe('buildProjectInstruction', () => {
	it('wraps the prompt in a project instruction header', () => {
		const prompt = 'Write about Canada.';
		const instruction = buildProjectInstruction(prompt);
		expect(instruction).toContain('# Japa Content Studio — Project Instructions');
		expect(instruction).toContain(prompt);
	});
});

describe('buildPrompt', () => {
	const baseValues: PromptFormValues = {
		...DEFAULT_PROMPT_VALUES,
		topic: 'Moving to Canada',
		title: 'Moving to Canada',
		content_type: 'guide',
		status: 'published',
		visibility: 'public'
	};

	const lookups: PromptLookups = {
		contentTypeName: 'Guide',
		guideCategoryName: '',
		personaName: 'Ada',
		voiceName: '',
		voicePrompt: '',
		spaceName: ''
	};

	it('includes required frontmatter fields', () => {
		const prompt = buildPrompt(baseValues, lookups);
		expect(prompt).toContain('jc-content/guide/moving-to-canada.md');
		expect(prompt).toContain('title:');
		expect(prompt).toContain('slug:');
		expect(prompt).toContain('content_type:');
	});

	it('forces high-stakes content to in_review', () => {
		const values = { ...baseValues, guide_category: 'visa' };
		const prompt = buildPrompt(values, lookups);
		expect(prompt).toContain('status: "in_review"');
	});

	it('keeps the selected high-stakes status when canSkipHitl', () => {
		const values = { ...baseValues, guide_category: 'visa' };
		const prompt = buildPrompt(values, lookups, undefined, true);
		expect(prompt).toContain('status: "published"');
	});

	it('does not emit placeholder ID fields', () => {
		const values: PromptFormValues = { ...baseValues, space_id: '', persona_id: '', voice_id: '' };
		const prompt = buildPrompt(values, lookups);
		expect(prompt).not.toContain('SPACE-ID-HERE');
		expect(prompt).not.toContain('PERSONA-ID-HERE');
		expect(prompt).not.toContain('VOICE-ID-HERE');
	});

	it('emits configured space, persona and voice ids', () => {
		const values: PromptFormValues = {
			...baseValues,
			space_id: 'space-1',
			persona_id: 'persona-1',
			voice_id: 'voice-1'
		};
		const prompt = buildPrompt(values, lookups);
		expect(prompt).toContain('space_id: "space-1"');
		expect(prompt).toContain('persona_id: "persona-1"');
		expect(prompt).toContain('voice_id: "voice-1"');
	});

	it('omits zero-value content constraints from the prompt body', () => {
		const prompt = buildPrompt(baseValues, lookups);
		expect(prompt).not.toContain('Minimum word count:');
		expect(prompt).not.toContain('Minimum number of H2 headings:');
		expect(prompt).not.toContain('Minimum total paragraphs');
		expect(prompt).not.toContain('At least');
		expect(prompt).not.toContain('internal links in the form');
		expect(prompt).not.toContain('high-quality external links');
	});

	it('includes content constraints when values are greater than zero', () => {
		const values: PromptFormValues = {
			...baseValues,
			min_words: 800,
			paragraph_count: 5,
			heading_count: 5,
			min_paragraphs_per_heading: 2,
			external_links: 2,
			internal_links: 1
		};
		const prompt = buildPrompt(values, lookups);
		expect(prompt).toContain('Minimum word count: 800');
		expect(prompt).toContain('Minimum number of H2 headings: 5');
		expect(prompt).toContain('Headings count excludes introduction, conclusion, FAQs, and key takeaways.');
		expect(prompt).toContain('At least 2 paragraphs per heading.');
		expect(prompt).toContain('Minimum total paragraphs (approximate): 5.');
		expect(prompt).toContain('Include 1 internal links in the form');
		expect(prompt).toContain('Include 2 high-quality external links');
	});

	it('omits persona and voice in member mode', () => {
		const values: PromptFormValues = {
			...baseValues,
			space_id: 'space-1',
			persona_id: 'persona-1',
			voice_id: 'voice-1'
		};
		const prompt = buildPrompt(values, lookups, 'member');
		expect(prompt).toContain('space_id: "space-1"');
		expect(prompt).not.toContain('persona_id:');
		expect(prompt).not.toContain('voice_id:');
		expect(prompt).toContain('Japa.to member writing under your own profile');
	});
});

describe('generated frontmatter block', () => {
	// Regression: frontmatterLine() emits no trailing newline, so two calls on
	// one template line concatenate ("a: 1b: 2"). Parse the emitted block with
	// a real YAML loader instead of substring checks.
	const lookups: PromptLookups = {
		contentTypeName: 'Guide',
		guideCategoryName: 'Visa',
		personaName: 'Ada',
		voiceName: 'Warm',
		voicePrompt: '',
		spaceName: ''
	};

	function emittedFrontmatter(prompt: string): Record<string, unknown> {
		const m = prompt.match(/```yaml\n---\n([\s\S]*?)\n---\n```/);
		expect(m).toBeTruthy();
		return yaml.load(m![1]) as Record<string, unknown>;
	}

	it('keeps guide_category and source_country on separate lines', () => {
		const prompt = buildPrompt(
			{ ...DEFAULT_PROMPT_VALUES, title: 'T', content_type: 'guide', guide_category: 'visa', source_country: 'ng', status: 'in_review' },
			lookups,
			'operator',
			true
		);
		const fm = emittedFrontmatter(prompt);
		expect(fm.guide_category).toBe('visa');
		expect(fm.source_country).toBe('ng');
	});

	it('keeps persona_id, voice_id and status on separate lines (operator mode)', () => {
		const prompt = buildPrompt(
			{ ...DEFAULT_PROMPT_VALUES, title: 'T', content_type: 'guide', persona_id: 'p1', voice_id: 'v1', status: 'in_review' },
			lookups,
			'operator'
		);
		const fm = emittedFrontmatter(prompt);
		expect(fm.persona_id).toBe('p1');
		expect(fm.voice_id).toBe('v1');
		expect(fm.status).toBe('in_review');
	});

	it('round-trips titles containing backslashes and quotes', () => {
		const prompt = buildPrompt(
			{ ...DEFAULT_PROMPT_VALUES, title: 'C:\\new "quoted" path', content_type: 'guide', status: 'in_review' },
			lookups,
			'operator'
		);
		const fm = emittedFrontmatter(prompt);
		expect(fm.title).toBe('C:\\new "quoted" path');
	});
});
