import { describe, it, expect } from 'vitest';
import { slugify, buildProjectInstruction } from './prompt';
import { promptFromDefaults, DEFAULT_PROMPT_VALUES } from '$lib/types/prompt';
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
