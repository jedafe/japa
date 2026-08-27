/**
 * Prompt builder form values and generated output.
 * Ref: sections/105-japa-content-studio-desktop.md
 */

// Contract: a content type accepted by POST /api/v1/content.
// guide_categories is a list of SelectOption {value, label} when the backend provides labels.
export interface StudioContentType {
	type: string;
	label: string;
	guide_categories?: SelectOption[];
}

export interface PublicContentType {
	value: string;
	label: string;
	has_category?: boolean;
	categories?: Array<{ Slug: string; Label: string }>;
}

export interface Persona {
	id: string;
	profile_name: string;
	user_id: string;
	voice_id?: string;
	voice_prompt?: string;
}

export interface PersonaVoice {
	id: string;
	name: string;
	slug?: string;
	description?: string;
	voice_prompt?: string;
	is_system?: boolean;
}

export interface Space {
	id: string;
	name: string;
}

export interface ContentRecord {
	id: string;
	slug: string;
	status: string;
	visibility: string;
	content_type: string;
	title: string;
}

export interface ContentDetail {
	id: string;
	slug: string;
	content_type: string;
	title: string;
	body: string;
	body_html: string;
	field_values?: Record<string, unknown>;
	canonical_url?: string;
	header_image_id?: string;
	featured_image_id?: string;
	meta_title?: string;
	meta_description?: string;
	og_image_url?: string;
	seo_indexable?: boolean;
	status: string;
	visibility: string;
	space_id?: string;
	publish_at?: string;
	published_at?: string;
	created_at: string;
	updated_at: string;
}

export interface SelectOption {
	value: string;
	label: string;
}

export type PromptMode = 'generate' | 'improve' | 'extend' | 'shorten';
export type PromptFramework = '' | 'aida' | 'pas';
export type PromptTone =
	| 'natural'
	| 'friendly'
	| 'witty'
	| 'empathetic'
	| 'direct'
	| 'eloquent'
	| 'professional'
	| 'persuasive'
	| 'motivational'
	| 'promoter'
	| 'influencer'
	| 'marketer'
	| 'concise'
	| 'academic'
	| 'casual';
export type HumanizeLevel = 'off' | 'light' | 'medium' | 'heavy';

// Contract: voice_id resolves to a shared persona voice (voice_prompt) for the prompt.
// tone is an inline prompt instruction ("Tone Variant") and is independent of voice_id;
// it is not persisted in frontmatter; voice_id is written to frontmatter in operator mode.
export interface PromptFormValues {
	topic: string;
	title: string;
	mode: PromptMode;
	content_type: string;
	guide_category: string;
	persona_id: string;
	voice_id: string;
	tone: PromptTone;
	framework: PromptFramework;
	humanize_level: HumanizeLevel;
	min_words: number;
	paragraph_count: number;
	heading_count: number;
	min_paragraphs_per_heading: number;
	external_links: number;
	internal_links: number;
	include_intro: boolean;
	include_conclusion: boolean;
	include_faqs: boolean;
	include_inline_image: boolean;
	include_steps: boolean;
	include_key_takeaways: boolean;
	space_id: string;
	visibility: string;
	status: string;
	tags: string[];
	source_country: string;
	target_country: string;
	audience: string;
	journey_stage: string;
	visa_route: string;
	canonical_url: string;
	meta_title: string;
	meta_description: string;
	og_image_url: string;
	seo_indexable: boolean;
	publish_at: string;
	header_image_id: string;
	featured_image_id: string;
	field_values: Record<string, string>;
}

export const DEFAULT_PROMPT_VALUES: PromptFormValues = {
	topic: '',
	title: '',
	mode: 'generate',
	content_type: 'guide',
	guide_category: '',
	persona_id: '',
	voice_id: '',
	tone: 'professional',
	framework: '',
	humanize_level: 'medium',
	min_words: 0,
	paragraph_count: 0,
	heading_count: 0,
	min_paragraphs_per_heading: 0,
	external_links: 0,
	internal_links: 0,
	include_intro: true,
	include_conclusion: true,
	include_faqs: false,
	include_inline_image: false,
	include_steps: true,
	include_key_takeaways: true,
	space_id: '',
	visibility: 'public',
	status: 'in_review',
	tags: [],
	source_country: '',
	target_country: '',
	audience: '',
	journey_stage: '',
	visa_route: '',
	canonical_url: '',
	meta_title: '',
	meta_description: '',
	og_image_url: '',
	seo_indexable: true,
	publish_at: '',
	header_image_id: '',
	featured_image_id: '',
	field_values: {}
};

/**
 * Apply saved project defaults to the prompt builder form without
 * overwriting fallbacks for fields that have no default.
 * Ref: sections/105-japa-content-studio-desktop.md
 */
export function promptFromDefaults(defaults: {
	default_content_type?: string;
	default_guide_category?: string;
	default_persona_id?: string;
	default_voice_id?: string;
	default_space_id?: string;
	default_status?: string;
	default_visibility?: string;
	default_tags?: string[];
}): PromptFormValues {
	const values = structuredClone(DEFAULT_PROMPT_VALUES);
	if (defaults.default_content_type) values.content_type = defaults.default_content_type;
	if (defaults.default_guide_category) values.guide_category = defaults.default_guide_category;
	if (defaults.default_persona_id) values.persona_id = defaults.default_persona_id;
	if (defaults.default_voice_id) values.voice_id = defaults.default_voice_id;
	if (defaults.default_space_id) values.space_id = defaults.default_space_id;
	if (defaults.default_status) values.status = defaults.default_status as PromptFormValues['status'];
	if (defaults.default_visibility) values.visibility = defaults.default_visibility as PromptFormValues['visibility'];
	if (defaults.default_tags?.length) values.tags = [...defaults.default_tags];
	return values;
}

export const FRAMEWORK_BY_CONTENT_TYPE: Record<string, PromptFramework> = {
	blog_post: 'aida',
	comparison: 'aida',
	discussion: 'aida',
	forum_thread: 'aida',
	question: 'pas'
};

export const TONE_OPTIONS: SelectOption[] = [
	{ value: 'natural', label: 'Natural' },
	{ value: 'friendly', label: 'Friendly' },
	{ value: 'witty', label: 'Witty' },
	{ value: 'empathetic', label: 'Empathetic' },
	{ value: 'direct', label: 'Direct' },
	{ value: 'eloquent', label: 'Eloquent' },
	{ value: 'professional', label: 'Professional' },
	{ value: 'persuasive', label: 'Persuasive' },
	{ value: 'motivational', label: 'Motivational' },
	{ value: 'promoter', label: 'Promoter' },
	{ value: 'influencer', label: 'Influencer' },
	{ value: 'marketer', label: 'Marketer' },
	{ value: 'concise', label: 'Concise' },
	{ value: 'academic', label: 'Academic' },
	{ value: 'casual', label: 'Casual' }
];

export const FRAMEWORK_OPTIONS: SelectOption[] = [
	{ value: '', label: 'None' },
	{ value: 'aida', label: 'AIDA' },
	{ value: 'pas', label: 'PAS' }
];

export const MODE_OPTIONS: SelectOption[] = [
	{ value: 'generate', label: 'Generate new article' },
	{ value: 'improve', label: 'Improve an existing file' },
	{ value: 'extend', label: 'Expand an existing file' },
	{ value: 'shorten', label: 'Shorten an existing file' }
];

export const HUMANIZE_OPTIONS: SelectOption[] = [
	{ value: 'off', label: 'Off' },
	{ value: 'light', label: 'Light' },
	{ value: 'medium', label: 'Medium' },
	{ value: 'heavy', label: 'Heavy' }
];

export interface GeneratedPrompt {
	text: string;
	projectInstruction: string;
}

export interface PromptTemplate {
	id: string;
	name: string;
	values: PromptFormValues;
	created_at: string;
}
