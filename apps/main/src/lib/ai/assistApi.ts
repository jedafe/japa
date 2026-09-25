import { api } from '$lib/api/client';
import type { ClientResult } from '$lib/api/client';

/**
 * Client wrappers for the Section 20 V1 in-editor assist endpoints:
 * POST /api/v1/ai/assist/{improve,summarize,translate,generate-tags,generate-og}.
 * All are RBAC-gated by ai.editor.assist and short-circuit with 503
 * ai_assist_disabled when the ai.content_assist_enabled setting is off.
 */

/** Raw dispatcher response shape returned by the assist endpoints. */
export interface AssistResponse {
	provider_ref?: string;
	provider_name?: string;
	status?: string;
	raw?: Record<string, unknown>;
}

export interface AssistImproveInput {
	text: string;
	tone?: string;
	model?: string;
	voice_id?: string;
}

export interface AssistSummarizeInput {
	text: string;
	max_words?: number;
	model?: string;
	voice_id?: string;
}

export interface AssistTranslateInput {
	text: string;
	target_locale: string;
	model?: string;
	voice_id?: string;
}

export interface AssistGenerateTagsInput {
	title: string;
	text?: string;
	model?: string;
	voice_id?: string;
}

export interface AssistGenerateOgInput {
	title: string;
	text?: string;
	content_type?: string;
	model?: string;
	voice_id?: string;
}

export function assistImprove(input: AssistImproveInput): Promise<ClientResult<AssistResponse>> {
	return api.post<AssistResponse>('/ai/assist/improve', input);
}

export function assistSummarize(input: AssistSummarizeInput): Promise<ClientResult<AssistResponse>> {
	return api.post<AssistResponse>('/ai/assist/summarize', input);
}

export function assistTranslate(input: AssistTranslateInput): Promise<ClientResult<AssistResponse>> {
	return api.post<AssistResponse>('/ai/assist/translate', input);
}

export function assistGenerateTags(input: AssistGenerateTagsInput): Promise<ClientResult<AssistResponse>> {
	return api.post<AssistResponse>('/ai/assist/generate-tags', input);
}

export function assistGenerateOg(input: AssistGenerateOgInput): Promise<ClientResult<AssistResponse>> {
	return api.post<AssistResponse>('/ai/assist/generate-og', input);
}

/**
 * Pull the generated text out of a dispatcher Response.raw — mirrors the
 * backend's ai.ExtractReplyText: OpenAI-compatible providers return
 * choices[].message.content; simpler providers return { reply: string }.
 */
export function extractAssistText(res: AssistResponse | undefined | null): string {
	const raw = res?.raw;
	if (!raw) return '';
	if (typeof raw.reply === 'string' && raw.reply) return raw.reply;
	const choices = raw.choices;
	if (Array.isArray(choices) && choices.length > 0) {
		const first = choices[0] as Record<string, unknown> | undefined;
		const message = first?.message as Record<string, unknown> | undefined;
		if (typeof message?.content === 'string') return message.content;
	}
	if (typeof raw.text === 'string') return raw.text;
	return '';
}

/**
 * Parse a JSON array of strings out of generated text. Providers often wrap
 * the array in prose or a markdown code fence — locate the first balanced
 * [...] block and JSON.parse it.
 */
export function extractTagList(text: string): string[] {
	const start = text.indexOf('[');
	const end = text.lastIndexOf(']');
	if (start < 0 || end <= start) return [];
	try {
		const parsed = JSON.parse(text.slice(start, end + 1));
		if (!Array.isArray(parsed)) return [];
		return parsed.filter((t): t is string => typeof t === 'string').map((t) => t.trim()).filter(Boolean);
	} catch {
		return [];
	}
}

/** Parsed OG metadata from generate-og output. Keys arrive as og:title etc. */
export interface OgMetadata {
	title?: string;
	description?: string;
	imageAlt?: string;
}

/**
 * Parse the OG metadata JSON object out of generated text. Accepts both
 * "og:title" and plain "title" key spellings.
 */
export function extractOgMetadata(text: string): OgMetadata {
	const start = text.indexOf('{');
	const end = text.lastIndexOf('}');
	if (start < 0 || end <= start) return {};
	try {
		const parsed = JSON.parse(text.slice(start, end + 1)) as Record<string, unknown>;
		const pick = (...keys: string[]): string | undefined => {
			for (const k of keys) {
				const v = parsed[k];
				if (typeof v === 'string' && v.trim()) return v.trim();
			}
			return undefined;
		};
		return {
			title: pick('og:title', 'og_title', 'title'),
			description: pick('og:description', 'og_description', 'description'),
			imageAlt: pick('og:image:alt', 'og_image_alt', 'image_alt', 'imageAlt')
		};
	} catch {
		return {};
	}
}

/** Slugify a free-form tag label the same way TagInput does. */
export function tagSlug(value: string): string {
	return value.trim().toLowerCase().replace(/\s+/g, '-');
}

/** Strip HTML markup to plain text, preserving block-level breaks. */
export function htmlToPlainText(html: string): string {
	if (!html) return '';
	return html
		.replace(/<(br|\/p|\/li|\/h[1-6]|\/blockquote|\/pre)[^>]*>/gi, '\n')
		.replace(/<[^>]*>/g, '')
		.replace(/&nbsp;/gi, ' ')
		.replace(/&amp;/gi, '&')
		.replace(/&lt;/gi, '<')
		.replace(/&gt;/gi, '>')
		.replace(/&quot;/gi, '"')
		.replace(/\n{3,}/g, '\n\n')
		.trim();
}

/** Wrap plain generated text in <p> tags so it renders in the block editor. */
export function plainTextToHtml(text: string): string {
	const esc = (s: string) =>
		s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;');
	return text
		.split(/\n{2,}/)
		.map((p) => p.trim())
		.filter(Boolean)
		.map((p) => `<p>${esc(p).replace(/\n/g, '<br>')}</p>`)
		.join('');
}
