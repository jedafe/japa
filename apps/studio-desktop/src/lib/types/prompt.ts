/**
 * Prompt builder form values and generated output.
 * Ref: sections/105-japa-content-studio-desktop.md
 */

export interface StudioContentType {
	type: string;
	label: string;
	can_auto_publish: boolean;
	guide_categories?: string[];
}

export interface ContentTypeResponse {
	types: StudioContentType[];
	default_persona_id?: string;
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

export interface SelectOption {
	value: string;
	label: string;
}

export type PromptMode = 'generate' | 'improve' | 'extend' | 'shorten';
export type PromptFramework = '' | 'aida' | 'pas';
export type PromptTone =
	| 'professional'
	| 'friendly'
	| 'concise'
	| 'persuasive'
	| 'academic'
	| 'travel_blogger'
	| 'visa_guide'
	| 'student_advisor'
	| 'community_storyteller'
	| 'immigration_consultant'
	| 'practical'
	| 'empathetic'
	| 'motivational'
	| 'casual';
export type HumanizeLevel = 'off' | 'light' | 'medium' | 'heavy';

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
	visibility: 'public' | 'private' | 'unlisted';
	status: 'draft' | 'in_review' | 'published' | 'scheduled';
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
	min_words: 800,
	paragraph_count: 5,
	heading_count: 5,
	min_paragraphs_per_heading: 2,
	external_links: 2,
	internal_links: 1,
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
	publish_at: ''
};

export const FRAMEWORK_BY_CONTENT_TYPE: Record<string, PromptFramework> = {
	blog_post: 'aida',
	comparison: 'aida',
	discussion: 'aida',
	forum_thread: 'aida',
	question: 'pas'
};

export const TONE_OPTIONS: SelectOption[] = [
	{ value: 'professional', label: 'Professional' },
	{ value: 'friendly', label: 'Friendly' },
	{ value: 'concise', label: 'Concise' },
	{ value: 'persuasive', label: 'Persuasive' },
	{ value: 'academic', label: 'Academic' },
	{ value: 'travel_blogger', label: 'Travel Blogger' },
	{ value: 'visa_guide', label: 'Visa Guide' },
	{ value: 'student_advisor', label: 'Student Advisor' },
	{ value: 'community_storyteller', label: 'Community Storyteller' },
	{ value: 'immigration_consultant', label: 'Immigration Consultant' },
	{ value: 'practical', label: 'Practical' },
	{ value: 'empathetic', label: 'Empathetic' },
	{ value: 'motivational', label: 'Motivational' },
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

export const VISIBILITY_OPTIONS: SelectOption[] = [
	{ value: 'public', label: 'Public' },
	{ value: 'private', label: 'Private' },
	{ value: 'unlisted', label: 'Unlisted' }
];

export const STATUS_OPTIONS: SelectOption[] = [
	{ value: 'draft', label: 'Draft' },
	{ value: 'in_review', label: 'In Review' },
	{ value: 'published', label: 'Published' },
	{ value: 'scheduled', label: 'Scheduled' }
];

export interface GeneratedPrompt {
	text: string;
	projectInstruction: string;
}
