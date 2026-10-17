import { describe, it, expect } from 'vitest';
import { parseBatchTopics, buildBatchPrompt } from './batch';
import { defaultSettings, type StudioSettings } from './settings';
import type { PromptLookups } from './prompt';

describe('parseBatchTopics', () => {
	it('parses one title per line', () => {
		const text = 'Moving to Canada\nHousing in Toronto\n\n';
		const topics = parseBatchTopics(text);
		expect(topics).toHaveLength(2);
		expect(topics[0]).toEqual({ title: 'Moving to Canada', content_type: '', guide_category: '' });
		expect(topics[1]).toEqual({ title: 'Housing in Toronto', content_type: '', guide_category: '' });
	});

	it('parses CSV with a header row', () => {
		const text = 'title,content_type,guide_category\nCanada,guide,relocation\nBanking,guide,banking';
		const topics = parseBatchTopics(text);
		expect(topics).toEqual([
			{ title: 'Canada', content_type: 'guide', guide_category: 'relocation' },
			{ title: 'Banking', content_type: 'guide', guide_category: 'banking' }
		]);
	});

	it('parses a JSON array', () => {
		const text = JSON.stringify([
			{ title: 'Visa Guide', content_type: 'guide', guide_category: 'visa' },
			{ title: 'Jobs' }
		]);
		const topics = parseBatchTopics(text);
		expect(topics).toEqual([
			{ title: 'Visa Guide', content_type: 'guide', guide_category: 'visa' },
			{ title: 'Jobs', content_type: '', guide_category: '' }
		]);
	});

	it('returns an empty array for blank input', () => {
		expect(parseBatchTopics('')).toEqual([]);
		expect(parseBatchTopics('   \n\n   ')).toEqual([]);
	});
});

describe('buildBatchPrompt', () => {
	it('includes all topic titles and file paths', () => {
		const settings: StudioSettings = {
			...defaultSettings(),
			default_content_type: 'guide',
			default_status: 'draft',
			default_visibility: 'public'
		};
		const lookups: PromptLookups = {
			contentTypeName: 'Guide',
			guideCategoryName: '',
			personaName: 'Ada',
			voiceName: '',
			spaceName: ''
		};
		const prompt = buildBatchPrompt(
			[
				{ title: 'Moving to Canada' },
				{ title: 'Banking in the UK', content_type: 'guide', guide_category: 'banking' }
			],
			settings,
			lookups
		);
		expect(prompt).toContain('Moving to Canada');
		expect(prompt).toContain('Banking in the UK');
		expect(prompt).toContain('jc-content/guide/moving-to-canada.md');
		expect(prompt).toContain('jc-content/guide/banking-in-the-uk.md');
		expect(prompt).toContain('Ada');
	});

	it('applies default guide category and voice_id', () => {
		const settings: StudioSettings = {
			...defaultSettings(),
			default_content_type: 'guide',
			default_guide_category: 'relocation',
			default_voice_id: 'voice-1',
			default_status: 'published',
			default_visibility: 'public'
		};
		const lookups: PromptLookups = {
			contentTypeName: 'Guide',
			guideCategoryName: 'Relocation',
			personaName: 'Ada',
			voiceName: '',
			spaceName: ''
		};
		const prompt = buildBatchPrompt([{ title: 'Moving to Canada' }], settings, lookups);
		expect(prompt).toContain('guide_category: "relocation"');
		expect(prompt).toContain('voice_id: "voice-1"');
	});

	it('forces high-stakes topics to in_review', () => {
		const settings: StudioSettings = {
			...defaultSettings(),
			default_content_type: 'guide',
			default_status: 'published',
			default_visibility: 'public'
		};
		const lookups: PromptLookups = {
			contentTypeName: 'Guide',
			guideCategoryName: '',
			personaName: 'Ada',
			voiceName: '',
			spaceName: ''
		};
		const prompt = buildBatchPrompt(
			[{ title: 'Banking in the UK', content_type: 'guide', guide_category: 'banking' }],
			settings,
			lookups
		);
		expect(prompt).toContain('status: "in_review"');
	});

	it('does not include an empty field_values placeholder block', () => {
		const settings: StudioSettings = {
			...defaultSettings(),
			default_content_type: 'guide',
			default_status: 'draft'
		};
		const lookups: PromptLookups = {
			contentTypeName: 'Guide',
			guideCategoryName: '',
			personaName: 'Ada',
			voiceName: '',
			spaceName: ''
		};
		const prompt = buildBatchPrompt([{ title: 'Jobs' }], settings, lookups);
		expect(prompt).not.toContain('field_values:');
	});

	it('omits persona and voice in member mode', () => {
		const settings: StudioSettings = {
			...defaultSettings(),
			mode: 'member',
			default_content_type: 'guide',
			default_persona_id: 'persona-1',
			default_voice_id: 'voice-1',
			default_status: 'draft'
		};
		const lookups: PromptLookups = {
			contentTypeName: 'Guide',
			guideCategoryName: '',
			personaName: 'Ada',
			voiceName: '',
			spaceName: ''
		};
		const prompt = buildBatchPrompt([{ title: 'Member post' }], settings, lookups);
		expect(prompt).not.toContain('persona_id:');
		expect(prompt).not.toContain('voice_id:');
		expect(prompt).toContain('Japa.to member writing under your own profile');
	});
});
