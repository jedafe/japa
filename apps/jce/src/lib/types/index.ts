/**
 * Core shared types for the Japa Comparison Engine app.
 */

export type ThemeName = 'dark' | 'light';

// Locale is a string code, but we keep it flexible.
export type Locale = string;

export interface SessionUser {
	id: string;
	username: string;
	email: string;
	full_name: string;
	avatar_url: string | null;
	is_verified: boolean;
	current_geo_space_id: string | null;
}

/** Success envelope: every endpoint wraps its payload in `{ data }`. */
export interface ApiOk<T> {
	data: T;
	meta?: ListMeta;
}

/** Error envelope: `{ error: { code, message } }`. */
export interface ApiErr {
	error: { code: string; message: string; status?: number };
}

export interface ListMeta {
	total?: number;
	page?: number;
	limit?: number;
	next_cursor?: string | null;
	sort_options?: string[];
	page_size_options?: number[];
}
