import { t } from '$lib/stores/i18n.svelte';

/** Thrown when a user-supplied JSON string cannot be parsed. */
export class JsonParseError extends Error {
	constructor(
		public field: string,
		message: string
	) {
		super(message);
	}
}

/**
 * Safely parse a JSON value.
 *
 * - `null` / `undefined` / empty string return the default.
 * - Already-parsed objects/arrays are returned as-is.
 * - Strings are parsed with `JSON.parse`; on failure a `JsonParseError` is
 *   thrown so the caller can surface a field-level error instead of silently
 *   falling back to the default.
 */
export function parseJson<T>(raw: unknown, field: string, defaultValue: T): T {
	if (raw == null || raw === '') return defaultValue;
	if (typeof raw === 'string') {
		try {
			return JSON.parse(raw) as T;
		} catch (e) {
			const reason = e instanceof Error ? e.message : String(e);
			throw new JsonParseError(
				field,
				t('common.invalid_json_field', 'Invalid JSON in {field}').replace('{field}', field) +
					': ' +
					reason
			);
		}
	}
	return raw as T;
}

/**
 * Format an unknown value as an indented JSON string for a textarea.
 */
export function jsonToString(raw: unknown, defaultValue: string): string {
	if (raw == null || raw === '') return defaultValue;
	if (typeof raw === 'string') return raw;
	return JSON.stringify(raw, null, 2);
}
