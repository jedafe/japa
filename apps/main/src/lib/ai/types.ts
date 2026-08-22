/**
 * Shared TypeScript types for the AI body-assist "draft" endpoint.
 * Mirrors the JSON request/response shape of POST /api/v1/ai/assist/draft.
 */

/** Description of a single content field used to drive structured generation. */
export interface FieldSchema {
	key: string;
	type: string;
	label: string;
	options?: { value: string; label: string }[];
}

/** Options to tailor the generated body. */
export interface DraftOptions {
	min_words?: number;
	paragraph_count?: number;
	heading_count?: number;
	include_intro?: boolean;
	include_conclusion?: boolean;
	include_faqs?: boolean;
	include_inline_image?: boolean;
	include_steps?: boolean;
	include_key_takeaways?: boolean;
}

/** Request body for POST /api/v1/ai/assist/draft. */
export interface DraftRequest {
	content_type: string;
	title?: string;
	existing_body?: string;
	instructions?: string;
	tone?: string;
	humanize?: boolean;
	humanize_level?: string;
	field_schema?: FieldSchema[];
	context?: Record<string, unknown>;
	options?: DraftOptions;
}

/** Success response from POST /api/v1/ai/assist/draft. */
export interface DraftResponse {
	title: string;
	body_html: string;
	field_values: Record<string, unknown>;
	tokens_used: number;
}
