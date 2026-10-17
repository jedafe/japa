import { siteName } from '$lib/i18n';
/**
 * Chat templates — named generation-spec presets for the /ai composer.
 * A template carries the same structured drafting options the studio
 * prompt builder and /ai/assist/draft use (framework, counts, links,
 * include_* flags). While a template is active the composer appends a
 * serialized spec to each outgoing message; in agent mode the model maps
 * it verbatim onto the content.generate_draft tool arguments, and in
 * plain chat mode it reads as plain drafting constraints.
 *
 * Templates persist inside user_preferences.ai_prefs.chat_templates
 * (GET/PUT /me/ai-preferences) — Values is opaque to the backend, so
 * this file is the sole owner of the shape.
 */

export type ChatTemplateFramework = '' | 'aida' | 'pas';
export type ChatTemplateHumanize = 'off' | 'light' | 'medium' | 'heavy';

export interface ChatTemplateValues {
	/** Send intent — 'preview' renders drafts in chat (generate_draft),
	 * 'create' steers the agent to persist items (create_draft/bulk_create). */
	intent: 'preview' | 'create';
	content_type: string;
	/** Guide-category slug — only meaningful when the selected content
	 * type declares categories (has_category), e.g. 'guide'. Drives the
	 * high-stakes status filter in the Settings tab. */
	guide_category: string;
	tone: string;
	framework: ChatTemplateFramework;
	humanize_level: ChatTemplateHumanize;
	/** Persona voice for the reply — maps to the /ai/chat voice_id request
	 * field (transport-level, not a generate_draft arg). Empty = default. */
	voice_id: string;
	audience: string;
	/** Extra free-text drafting guidance appended to the tool instructions. */
	instructions: string;
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
	// Journey tab — same relocation-context fields the studio prompt
	// builder emits into frontmatter (§14).
	journey_stage: string;
	visa_route: string;
	source_country: string;
	target_country: string;
	// Settings tab — publishing metadata. These aren't DraftInput args;
	// they serialize under spec.context as advisory hints for the model.
	space_id: string;
	series_id: string;
	publish_at: string;
	tags: string[];
	status: string;
	visibility: string;
	seo_indexable: boolean;
	field_values: Record<string, string>;
}

export const DEFAULT_CHAT_TEMPLATE_VALUES: ChatTemplateValues = {
	intent: 'preview',
	content_type: 'guide',
	guide_category: '',
	tone: 'professional',
	framework: '',
	humanize_level: 'medium',
	voice_id: '',
	audience: '',
	instructions: '',
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
	journey_stage: '',
	visa_route: '',
	source_country: '',
	target_country: '',
	space_id: '',
	series_id: '',
	publish_at: '',
	tags: [],
	// Mirrors DEFAULT_PROMPT_VALUES — generated output defaults to review,
	// not straight to published.
	status: 'in_review',
	visibility: 'public',
	seo_indexable: true,
	field_values: {}
};

/** Canonical JSON for dirty comparisons — sorts keys recursively so
 * nested field_values reordering doesn't read as a change. */
export function templateFingerprint(values: ChatTemplateValues): string {
	const sortDeep = (v: unknown): unknown => {
		if (Array.isArray(v)) return v.map(sortDeep);
		if (v && typeof v === 'object') {
			return Object.fromEntries(
				Object.entries(v as Record<string, unknown>)
					.sort(([a], [b]) => a.localeCompare(b))
					.map(([k, x]) => [k, sortDeep(x)])
			);
		}
		return v;
	};
	return JSON.stringify(sortDeep(values));
}

/** A saved preset row inside ai_prefs.chat_templates. */
export interface ChatTemplate {
	id: string;
	name: string;
	/** Slash-separated folder path ('visa/guides') — '' means ungrouped.
	 * Folders are derived from template paths; there are no folder rows. */
	folder: string;
	values: ChatTemplateValues;
	created_at?: string;
	updated_at?: string;
}

/** Normalize a stored/typed folder path — trims each segment, drops
 * empty segments (collapses '//' and edge slashes). Non-strings (older
 * templates saved before folders existed) come back ungrouped. */
export function normalizeTemplateFolder(raw: unknown): string {
	if (typeof raw !== 'string') return '';
	return raw
		.split('/')
		.map((s) => s.trim())
		.filter(Boolean)
		.join('/');
}

/**
 * Normalize a stored values blob into ChatTemplateValues — missing keys
 * (older templates, hand-edited prefs) fall back to defaults.
 */
export function normalizeTemplateValues(raw: unknown): ChatTemplateValues {
	const v = (raw && typeof raw === 'object' ? raw : {}) as Record<string, unknown>;
	const num = (x: unknown) => (typeof x === 'number' && isFinite(x) && x > 0 ? Math.floor(x) : 0);
	const str = (x: unknown) => (typeof x === 'string' ? x : '');
	const bool = (x: unknown, d: boolean) => (typeof x === 'boolean' ? x : d);
	const framework = str(v.framework);
	const humanize = str(v.humanize_level);
	const tags = Array.isArray(v.tags)
		? v.tags.filter((x): x is string => typeof x === 'string' && x.trim() !== '')
		: [];
	const fieldValues: Record<string, string> = {};
	if (v.field_values && typeof v.field_values === 'object' && !Array.isArray(v.field_values)) {
		for (const [k, val] of Object.entries(v.field_values as Record<string, unknown>)) {
			if (typeof val === 'string' && k.trim()) fieldValues[k.trim()] = val;
		}
	}
	return {
		intent: v.intent === 'create' ? 'create' : 'preview',
		content_type: str(v.content_type) || DEFAULT_CHAT_TEMPLATE_VALUES.content_type,
		guide_category: str(v.guide_category),
		tone: str(v.tone) || DEFAULT_CHAT_TEMPLATE_VALUES.tone,
		framework:
			framework === 'aida' || framework === 'pas' ? framework : DEFAULT_CHAT_TEMPLATE_VALUES.framework,
		humanize_level: (['off', 'light', 'medium', 'heavy'] as const).includes(
			humanize as ChatTemplateHumanize
		)
			? (humanize as ChatTemplateHumanize)
			: DEFAULT_CHAT_TEMPLATE_VALUES.humanize_level,
		voice_id: str(v.voice_id),
		audience: str(v.audience),
		instructions: str(v.instructions),
		min_words: num(v.min_words),
		paragraph_count: num(v.paragraph_count),
		heading_count: num(v.heading_count),
		min_paragraphs_per_heading: num(v.min_paragraphs_per_heading),
		external_links: num(v.external_links),
		internal_links: num(v.internal_links),
		include_intro: bool(v.include_intro, true),
		include_conclusion: bool(v.include_conclusion, true),
		include_faqs: bool(v.include_faqs, false),
		include_inline_image: bool(v.include_inline_image, false),
		include_steps: bool(v.include_steps, true),
		include_key_takeaways: bool(v.include_key_takeaways, true),
		journey_stage: str(v.journey_stage),
		visa_route: str(v.visa_route),
		source_country: str(v.source_country),
		target_country: str(v.target_country),
		space_id: str(v.space_id),
		series_id: str(v.series_id),
		publish_at: str(v.publish_at),
		tags,
		status: str(v.status) || DEFAULT_CHAT_TEMPLATE_VALUES.status,
		visibility: str(v.visibility) || DEFAULT_CHAT_TEMPLATE_VALUES.visibility,
		seo_indexable: bool(v.seo_indexable, true),
		field_values: fieldValues
	};
}

/**
 * Serialize a template into the content.generate_draft tool-argument
 * shape (DraftInput): content_type + tone + instructions + humanize +
 * options. Only non-default fields are emitted so the JSON stays a
 * readable spec.
 */
export function buildTemplateSpec(values: ChatTemplateValues): Record<string, unknown> {
	const options: Record<string, unknown> = { mode: 'generate' };
	if (values.framework) options.framework = values.framework;
	if (values.min_words > 0) options.min_words = values.min_words;
	if (values.paragraph_count > 0) options.paragraph_count = values.paragraph_count;
	if (values.heading_count > 0) options.heading_count = values.heading_count;
	if (values.min_paragraphs_per_heading > 0)
		options.min_paragraphs_per_heading = values.min_paragraphs_per_heading;
	if (values.external_links > 0) options.external_links = values.external_links;
	if (values.internal_links > 0) options.internal_links = values.internal_links;
	if (values.include_intro) options.include_intro = true;
	if (values.include_conclusion) options.include_conclusion = true;
	if (values.include_faqs) options.include_faqs = true;
	if (values.include_inline_image) options.include_inline_image = true;
	if (values.include_steps) options.include_steps = true;
	if (values.include_key_takeaways) options.include_key_takeaways = true;

	const instructions = [
		values.audience ? `Audience: ${values.audience}` : '',
		values.instructions.trim()
	]
		.filter(Boolean)
		.join('\n');

	// Journey + settings ride under "context" — they aren't DraftInput
	// fields, but the model maps them onto the tool's context arg or
	// honors them as drafting/publishing hints.
	const context: Record<string, unknown> = {};
	if (values.guide_category) context.guide_category = values.guide_category;
	if (values.journey_stage) context.journey_stage = values.journey_stage;
	if (values.visa_route) context.visa_route = values.visa_route;
	if (values.source_country) context.source_country = values.source_country;
	if (values.target_country) context.target_country = values.target_country;
	if (values.space_id) context.space_id = values.space_id;
	if (values.series_id) context.series_id = values.series_id;
	if (values.tags.length) context.tags = values.tags;
	// Status/visibility/indexable only emit when they differ from the
	// composer defaults — an untouched Settings tab stays out of the spec.
	if (values.status && values.status !== DEFAULT_CHAT_TEMPLATE_VALUES.status)
		context.status = values.status;
	if (values.visibility && values.visibility !== DEFAULT_CHAT_TEMPLATE_VALUES.visibility)
		context.visibility = values.visibility;
	if (!values.seo_indexable) context.seo_indexable = false;
	if (values.publish_at) context.publish_at = values.publish_at;
	if (Object.keys(values.field_values).length) context.field_values = values.field_values;

	const spec: Record<string, unknown> = {
		content_type: values.content_type || 'guide'
	};
	if (Object.keys(context).length) spec.context = context;
	if (values.tone) spec.tone = values.tone;
	if (instructions) spec.instructions = instructions;
	if (values.humanize_level !== 'off') {
		spec.humanize = true;
		spec.humanize_level = values.humanize_level;
	}
	spec.options = options;
	return spec;
}

/**
 * The block appended to an outgoing message while a template is active.
 * The JSON mirrors content.generate_draft's parameter schema so the
 * model can pass it through verbatim; the prose fallback covers turns
 * where the tool catalog is empty (pure chat).
 */
export function buildTemplateBlock(name: string, values: ChatTemplateValues): string {
	const spec = buildTemplateSpec(values);
	const label = name.trim() || 'Untitled template';
	const persist = values.intent === 'create';
	return (
		`\n\n---\n` +
		`[Chat template: "${label}"] Apply this generation spec to my request. ` +
		(persist
			? `Persist the result as real content — call content.create_draft once per topic ` +
				`(or content.bulk_create when my request lists several topics), filling ` +
				`title/body from my request and these constraints. If no content-creation ` +
				`tools are available, present the drafts in chat instead.\n\n`
			: `If the content.generate_draft tool is available, call it with these arguments, ` +
				`filling "title" and "instructions" from my request above — when my request ` +
				`lists multiple topics, make one call per topic; otherwise draft the ` +
				`content directly, honoring the same constraints.\n\n`) +
		'```json\n' +
		JSON.stringify(spec, null, 2) +
		'\n```'
	);
}

/** Display-name lookups for ids stored in the spec (space/series/voice)
 * — the composer resolves them from live catalogs so the inserted prose
 * reads as names, not uuids. */
export interface TemplateInsertLabels {
	space?: string;
	series?: string;
	voice?: string;
}

// Marker lines an inserted spec is recognized by on re-insert. The
// topics header ends every spec block; the legacy 'Topic: ' line covers
// inserts produced before the Topics list existed.
const TOPICS_MARKER = 'Topics (one per line';
const LEGACY_TOPIC_MARKER = '\nTopic: ';
const SPEC_HEADER_RE = /^(?:Write (?:a|one)|Create one)\b[^\n]*(?:draft|requirements)[^\n]*:/;

/**
 * Human-readable instruction text for the "insert into composer" flow —
 * the user reviews/edits before sending, so it reads as prose rather
 * than a tool-args payload. Covers every non-default spec field so the
 * inserted prompt is faithful to what an attached send would carry.
 * Ends at the Topics header; the caller appends the topic tail
 * (see splitComposerForInsert + normalizeTopicsTail).
 */
export function buildTemplateInsertText(
	values: ChatTemplateValues,
	labels: TemplateInsertLabels = {}
): string {
	const lines: string[] = [
		values.intent === 'create'
			? `Create one ${values.content_type || 'guide'} per topic in the Topics list — persist each as a real content item (content.create_draft for a single topic, content.bulk_create for several; if creation tools are unavailable, present the drafts in chat instead), with these requirements:`
			: `Write one ${values.content_type || 'guide'} draft per topic in the Topics list, with these requirements:`
	];
	if (values.framework) {
		lines.push(`- Structure it with the ${values.framework.toUpperCase()} marketing framework`);
	}
	const counts: string[] = [];
	if (values.min_words > 0) counts.push(`at least ${values.min_words} words`);
	if (values.heading_count > 0) counts.push(`${values.heading_count}+ H2 sections`);
	if (values.min_paragraphs_per_heading > 0)
		counts.push(`${values.min_paragraphs_per_heading}+ paragraphs per section`);
	if (values.paragraph_count > 0) counts.push(`~${values.paragraph_count} paragraphs total`);
	if (counts.length) lines.push(`- Length/structure: ${counts.join(', ')}`);
	if (values.internal_links > 0 || values.external_links > 0) {
		const links: string[] = [];
		if (values.internal_links > 0)
			links.push(
				`${values.internal_links} internal links as [text](/content/[slug]) — real ${siteName()} slugs only`
			);
		if (values.external_links > 0)
			links.push(`${values.external_links} authoritative external links with real URLs`);
		lines.push(`- Links: ${links.join('; ')}`);
	}
	const includes: string[] = [];
	if (values.include_intro) includes.push('an introduction');
	if (values.include_conclusion) includes.push('a conclusion');
	if (values.include_faqs) includes.push('a short FAQ section');
	if (values.include_steps) includes.push('numbered steps where applicable');
	if (values.include_key_takeaways) includes.push('key takeaways per major section');
	if (values.include_inline_image) includes.push('inline image placeholders with alt text');
	if (includes.length) lines.push(`- Include ${includes.join(', ')}`);
	if (values.tone) lines.push(`- Tone: ${values.tone}`);
	if (values.voice_id) lines.push(`- Voice: ${labels.voice || values.voice_id}`);
	if (values.humanize_level !== 'off')
		lines.push(`- Write naturally and human (${values.humanize_level} humanization)`);
	if (values.audience) lines.push(`- Audience: ${values.audience}`);
	const route = [values.source_country, values.target_country].filter(Boolean).join(' → ');
	const journey: string[] = [];
	if (values.guide_category) lines.push(`- Guide category: ${values.guide_category}`);
	if (values.journey_stage) journey.push(`the "${values.journey_stage}" journey stage`);
	if (values.visa_route) journey.push(`the ${values.visa_route} route`);
	if (route) journey.push(route);
	if (journey.length) lines.push(`- Context: readers on ${journey.join(', ')}`);
	if (values.tags.length) lines.push(`- Cover these tags/themes: ${values.tags.join(', ')}`);
	if (values.instructions.trim()) lines.push(`- ${values.instructions.trim()}`);
	// Publishing metadata — only non-default values emit, matching the
	// attach block's context contract.
	const publishing: string[] = [];
	if (values.status && values.status !== DEFAULT_CHAT_TEMPLATE_VALUES.status)
		publishing.push(`status ${values.status}`);
	if (values.visibility && values.visibility !== DEFAULT_CHAT_TEMPLATE_VALUES.visibility)
		publishing.push(`${values.visibility} visibility`);
	if (!values.seo_indexable) publishing.push('not indexable');
	if (publishing.length) lines.push(`- Publishing: ${publishing.join(', ')}`);
	if (values.publish_at) lines.push(`- Publish at: ${values.publish_at}`);
	if (values.space_id) lines.push(`- Space: ${labels.space || values.space_id}`);
	if (values.series_id) lines.push(`- Series: ${labels.series || values.series_id}`);
	const fv = Object.entries(values.field_values);
	if (fv.length)
		lines.push(`- Field values: ${fv.map(([k, v]) => `${k} = ${v}`).join('; ')}`);
	if (values.intent !== 'create') {
		lines.push('', 'Present the draft(s) in chat — do not create content items.');
	}
	lines.push(
		'',
		`${TOPICS_MARKER} — ${values.intent === 'create' ? 'one item is persisted per topic' : 'one draft is generated per topic'}):`
	);
	return lines.join('\n');
}

/**
 * Extract the topics tail from composer content for a template insert.
 * When the input starts with a spec header and carries a topics marker,
 * returns everything after the topics header (the user's topic lines +
 * any trailing text). Anything else — free text, or a spec whose topics
 * marker the user deleted — is returned whole, so an insert never
 * silently drops typed content.
 */
export function splitComposerForInsert(input: string): string {
	if (!SPEC_HEADER_RE.test(input)) return input.trim();
	const topicsIdx = input.indexOf(TOPICS_MARKER);
	if (topicsIdx >= 0) {
		const nl = input.indexOf('\n', topicsIdx);
		return nl >= 0 ? input.slice(nl + 1) : '';
	}
	const legacyIdx = input.indexOf(LEGACY_TOPIC_MARKER);
	if (legacyIdx >= 0) return input.slice(legacyIdx + LEGACY_TOPIC_MARKER.length);
	return input.trim();
}

/**
 * Normalize a topics tail — an empty tail gets one empty bullet so the
 * cursor has somewhere to land, a single-line draft gets the bullet
 * affordance, and multi-line text passes through verbatim (bulletizing
 * line-by-line would mangle paragraphs; "one per line" already makes
 * plain lines read as topics).
 */
export function normalizeTopicsTail(tail: string): string {
	const trimmed = tail.replace(/^\n+|\n+$/g, '');
	if (!trimmed) return '- ';
	if (!trimmed.includes('\n') && !trimmed.trimStart().startsWith('-'))
		return `- ${trimmed}`;
	return trimmed;
}
