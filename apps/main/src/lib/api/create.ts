import { api } from '$lib/api/client';
import { localDateTimeToIso } from '$lib/utils/datetime';
import type { ContentRecord, MediaAsset, MeSpace, SaveErrorClass, SearchHit, BlogSeries, SpaceType, Space } from '$lib/types';
import { searchPath, mapUserHits, mapContentHits, mapTagHits, mapCouponHits, mapLiteralHits } from '$lib/tiptap/search';

export interface CreateContentInput {
	content_type: string;
	title: string;
	slug?: string;
	body?: string;
	body_html?: string;
	field_values?: Record<string, unknown>;
	canonical_url?: string | null;
	header_image_id?: string | null;
	featured_image_id?: string | null;
	/** §106 video — linked video_sources row (content_type === 'video'). */
	video_source_id?: string | null;
	meta_title?: string | null;
	meta_description?: string | null;
	og_image_url?: string | null;
	seo_indexable?: boolean;
	organization_id?: string | null;
	status?: string;
	visibility?: string;
	publish_at?: string | null;
	space_id?: string | null;
	geo_space_id?: string | null;
	guide_category?: string | null;
	// Journey-aware recommendation metadata (backend normalizes values).
	journey_stage?: string | null;
	visa_route?: string | null;
	source_country?: string | null;
	target_country?: string | null;
}

/** Map an HTTP status to a save-failure class (drives retry vs surface behavior). */
export function classifyError(status: number): SaveErrorClass {
	if (status === 0) return 'network';
	if (status === 409) return 'conflict';
	if (status >= 500) return 'server';
	return 'validation'; // any other 4xx
}

/* ---- content lifecycle (standard form) ---- */
export const createContent = (input: CreateContentInput) => api.post<ContentRecord>('/content', input);
export const patchContent = (id: string, patch: Partial<CreateContentInput>) =>
	api.patch<ContentRecord>(`/content/${id}`, patch);
export const getContentSpaces = (id: string) =>
	api.get<{ data: { space_id: string; is_primary: boolean }[] }>(`/content/${id}/spaces`);
export const patchContentSpaces = (
	id: string,
	input: { primary_space_id: string; additional_space_ids: string[] }
) => api.patch<{ updated: boolean }>(`/content/${id}/spaces`, input);
export const publishContent = (id: string) => api.post<ContentRecord>(`/content/${id}/publish`);
// publish_at arrives as a raw datetime-local value — the API decodes it into
// a time.Time, which requires RFC 3339, so normalise to an ISO instant here.
export const scheduleContent = (id: string, publish_at: string) =>
	api.post<ContentRecord>(`/content/${id}/schedule`, { publish_at: localDateTimeToIso(publish_at) });

/* ---- tags (deferred linkage) ----
 * The composer tracks tags as slugs (from TagInput), not UUIDs. The spec-
 * compliant POST /content/{id}/tags accepts { tag_slugs: string[] } and
 * resolves slugs → tag IDs server-side (Section 14). The DELETE endpoint
 * accepts either a UUID or a slug in the {tagId} path segment (the handler
 * tries UUID first, then falls back to slug resolution). */
export const attachTag = (id: string, tagSlug: string) =>
	api.post(`/content/${id}/tags`, { tag_slugs: [tagSlug] });
export const detachTag = (id: string, tagSlug: string) =>
	api.delete(`/content/${id}/tags/${encodeURIComponent(tagSlug)}`);

/* ---- series (deferred linkage; standard form only) ---- */
export const listSeries = () => api.get<BlogSeries[]>('/blog/series');
export const searchSeries = (q: string) =>
	api.get<BlogSeries[]>(`/blog/series?q=${encodeURIComponent(q)}`);

/* ---- spaces (debounced autocomplete search) ---- */
export const searchSpaces = (q: string, limit = 50) =>
	api.get<MeSpace[]>(`/me/spaces?q=${encodeURIComponent(q)}&page_size=${limit}`);
export const listSpaceTypes = () =>
	api.get<SpaceType[]>('/space-types');
export const createSpace = (input: {
	name: string;
	slug: string;
	space_type_id: string;
	level: string;
	description?: string;
	geo_code?: string;
	parent_id?: string;
	parent_space_ids?: string[];
}) => api.post<Space>('/spaces', input);
export const createSeries = (input: {
	title: string;
	slug: string;
	description?: string;
	cover_image_id?: string;
}) => api.post<BlogSeries>('/blog/series', input);
export const linkSeries = (seriesId: string, contentId: string, position: number) =>
	api.post(`/blog/series/${seriesId}/posts`, { content_id: contentId, position });
export const unlinkSeries = (seriesId: string, contentId: string) =>
	api.delete(`/blog/series/${seriesId}/posts/${contentId}`);

/* ---- media (2-step presigned-URL upload) ----
 * The backend's POST /media/upload takes a JSON body {filename, mime_type,
 * file_size} and returns {media_id, storage_path, presigned_url, expires_at}.
 * The client PUTs the raw file bytes directly to presigned_url (SeaweedFS),
 * then calls POST /media/confirm/{id}. uploadToPresigned is a thin fetch
 * wrapper that does NOT go through the JSON api client (no envelope, no auth
 * header — the presigned URL is self-authorizing). */
export interface InitiateUploadInput {
	filename: string;
	mime_type: string;
	file_size: number;
}
export interface InitiateUploadResult {
	media_id: string;
	storage_path: string;
	presigned_url: string;
	expires_at: string;
}
export const initiateUpload = (input: InitiateUploadInput) =>
	api.post<InitiateUploadResult>('/media/upload', input);

/** PUT the raw file bytes to the presigned URL returned by initiateUpload. */
export async function uploadToPresigned(
	presignedUrl: string,
	file: File
): Promise<{ ok: boolean; status: number }> {
	try {
		const res = await fetch(presignedUrl, { method: 'PUT', body: file });
		return { ok: res.ok, status: res.status };
	} catch {
		return { ok: false, status: 0 };
	}
}

export const confirmMedia = (id: string) => api.post<{ confirmed: boolean }>(`/media/confirm/${id}`);

/* ---- media library (GET /me/media — ADR-034 Modal Picker) ----
 * Returns the caller's own uploaded media, paginated. GET /me/media
 * responds with { data, total, total_pages }; getEnvelope keeps the
 * sidecar envelope while still exposing res.data as the array.
 * Optional filters: type (image|video|…), q (filename search),
 * page_size/page_offset (server-side pagination). */
export interface ListMyMediaParams {
	type?: string;
	q?: string;
	status?: string;
	flagged?: boolean;
	page_size?: number;
	page_offset?: number;
}
export interface ListMyMediaEnvelope {
	data: MediaAsset[];
	total: number;
	total_pages: number;
}
export const listMyMedia = (params: ListMyMediaParams = {}) => {
	const qs = new URLSearchParams();
	if (params.type) qs.set('type', params.type);
	if (params.q) qs.set('q', params.q);
	if (params.status) qs.set('status', params.status);
	if (params.flagged === true) qs.set('flagged', 'true');
	if (params.page_size) qs.set('page_size', String(params.page_size));
	if (params.page_offset) qs.set('page_offset', String(params.page_offset));
	const suffix = qs.toString() ? `?${qs.toString()}` : '';
	return api.getEnvelope<MediaAsset[]>(`/me/media${suffix}`);
};

export const getMedia = (id: string) => api.get<MediaAsset>(`/media/${id}`);

/* ---- media metadata + delete (PATCH/DELETE /media/{id} — §34) ---- */
export const updateMedia = (id: string, body: { alt_text?: string; caption?: string }) =>
	api.patch<{ updated: boolean }>(`/media/${id}`, body);

export const deleteMedia = (id: string) => api.delete<{ deleted: boolean }>(`/media/${id}`);

export const bulkDeleteMedia = (mediaIds: string[]) =>
	api.post<{ deleted: number }>('/media/bulk-delete', { media_ids: mediaIds });

/* ---- trigger search ---- */
export async function searchHits(
	kind: 'users' | 'content' | 'tags' | 'coupons' | 'literals',
	query: string
): Promise<SearchHit[]> {
	const path = searchPath(kind, query);
	if (!path) return [];
	const res = await api.get<unknown>(path);
	if (res.error || res.data == null) return [];
	// /search endpoints return { results: [...] }; /tags, /coupons, and /literals return [...] directly.
	const payload = res.data as { results?: unknown[] } | unknown[];
	const rows = Array.isArray(payload) ? payload : (payload.results ?? []);
	if (kind === 'users') return mapUserHits(rows);
	if (kind === 'content') return mapContentHits(rows);
	if (kind === 'coupons') return mapCouponHits(rows);
	if (kind === 'literals') return mapLiteralHits(rows);
	return mapTagHits(rows);
}

/* ---- JCE comparisons ---- */
export const patchComparison = (id: string, patch: Record<string, unknown>) =>
	api.patch(`/jce/comparisons/${id}`, patch);
export const addComparisonItem = (
	id: string,
	input: { title: string; description?: string; image_id?: string; geo_space_id?: string; position: number; item_ref?: Record<string, unknown> }
) => api.post<{ id: string }>(`/jce/comparisons/${id}/items`, input);
export const addComparisonDimension = (
	id: string,
	input: { label: string; dimension_type: string; position: number }
) => api.post<{ id: string }>(`/jce/comparisons/${id}/dimensions`, input);
/** DELETE /jce/comparisons/items/{id} — remove a comparison item (server-first). */
export const deleteComparisonItem = (id: string) => api.delete(`/jce/comparisons/items/${id}`);
/** DELETE /jce/comparisons/dimensions/{id} — remove a comparison dimension (server-first). */
export const deleteComparisonDimension = (id: string) =>
	api.delete(`/jce/comparisons/dimensions/${id}`);
/** PATCH /jce/comparisons/items/{id} — update item metadata (title, etc.). */
export const patchComparisonItem = (id: string, patch: Record<string, unknown>) =>
	api.patch(`/jce/comparisons/items/${id}`, patch);
/** PATCH /jce/comparisons/dimensions/{id} — update dimension metadata (label, etc.). */
export const patchComparisonDimension = (id: string, patch: Record<string, unknown>) =>
	api.patch(`/jce/comparisons/dimensions/${id}`, patch);
/** POST /jce/comparisons/auto-dimensions — heuristic dimension suggestions (no AI tokens). */
export const autoGenerateDimensions = (items: string[]) =>
	api.post<{ dimensions: string[] }>('/jce/comparisons/auto-dimensions', { items });
/** PUT /jce/comparisons/dimensions/{id}/values — batch upsert of cell values. */
export const setDimensionValues = (
	dimensionId: string,
	values: Array<{ item_id: string; value?: string; value_rating?: number; value_boolean?: boolean }>
) => api.put(`/jce/comparisons/dimensions/${dimensionId}/values`, { values });
