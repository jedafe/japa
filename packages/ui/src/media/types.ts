/**
 * Shared types for the reusable media library picker (extracted from the
 * main app's MediaLibraryModal — §34 media, ADR-034).
 */

/** One media-library asset — the subset of the backend media row the picker
 *  needs (apps may pass richer objects like main's MediaAsset). */
export interface MediaPickerItem {
	id?: string;
	url: string;
	filename?: string;
	alt_text?: string | null;
	caption?: string | null;
	mime_type?: string;
	file_size?: number;
	width?: number;
	height?: number;
	status?: string;
	/** Stock attribution (present when media was imported from a provider). */
	source_platform?: string;
	source_photographer?: string;
	source_photographer_url?: string;
	source_license?: string;
	source_license_url?: string;
}

export interface StockProvider {
	name: string;
	display_name: string;
}

export interface StockPhoto {
	id: string;
	provider: string;
	thumbnail_url: string;
	full_url: string;
	width?: number;
	height?: number;
	photographer?: string;
	photographer_url?: string;
	license?: string;
	license_url?: string;
	description?: string;
}

export interface StockSearchResult {
	photos: StockPhoto[];
	has_next_page: boolean;
}

/** Host-provided stock-photo APIs — omit to hide the Stock tab. */
export interface StockApi {
	listProviders: () => Promise<StockProvider[]>;
	search: (params: {
		q: string;
		provider?: string;
		page?: number;
		per_page?: number;
	}) => Promise<StockSearchResult>;
	import: (provider: string, id: string) => Promise<{ media_id?: string }>;
}
