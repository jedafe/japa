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
}

/** Success response from POST /api/v1/ai/assist/draft. */
export interface DraftResponse {
	title: string;
	body_html: string;
	field_values: Record<string, unknown>;
	tokens_used: number;
}
