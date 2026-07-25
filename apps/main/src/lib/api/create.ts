import { api } from '$lib/api/client';
import type { ContentRecord, MediaAsset, SaveErrorClass, SearchHit, BlogSeries } from '$lib/types';
import { searchPath, mapUserHits, mapContentHits, mapTagHits } from '$lib/tiptap/search';

export interface CreateContentInput {
	content_type: string;
	title: string;
	slug?: string;
	body?: string;
	body_html?: string;
	field_values?: Record<string, unknown>;
	canonical_url?: string | null;
	status?: string;
	visibility?: string;
	publish_at?: string | null;
	space_id?: string | null;
	geo_space_id?: string | null;
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
export const publishContent = (id: string) => api.post<ContentRecord>(`/content/${id}/publish`);
export const scheduleContent = (id: string, publish_at: string) =>
	api.post<ContentRecord>(`/content/${id}/schedule`, { publish_at });

/* ---- tags (deferred linkage) ---- */
export const attachTag = (id: string, tagId: string) =>
	api.post(`/content/${id}/tags`, { tag_id: tagId });
export const detachTag = (id: string, tagId: string) => api.delete(`/content/${id}/tags/${tagId}`);

/* ---- series (deferred linkage; standard form only) ---- */
export const listSeries = () => api.get<BlogSeries[]>('/blog/series');
export const createSeries = (input: { title: string; slug: string; description?: string }) =>
	api.post<BlogSeries>('/blog/series', input);
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

export const confirmMedia = (id: string) => api.post<MediaAsset>(`/media/confirm/${id}`);

/* ---- trigger search ---- */
export async function searchHits(
	kind: 'users' | 'content' | 'tags',
	query: string
): Promise<SearchHit[]> {
	const path = searchPath(kind, query);
	if (!path) return [];
	const res = await api.get<unknown>(path);
	if (res.error || res.data == null) return [];
	if (kind === 'users') return mapUserHits(res.data);
	if (kind === 'content') return mapContentHits(res.data);
	return mapTagHits(res.data);
}

/* ---- comparisons ---- */
export const createComparison = (input: {
	title: string;
	body?: string;
	visibility?: string;
	space_id?: string;
}) => api.post<{ id: string }>('/comparisons', input);
export const patchComparison = (id: string, patch: Record<string, unknown>) =>
	api.patch(`/comparisons/${id}`, patch);
export const addComparisonItem = (
	id: string,
	input: { title: string; description?: string; image_id?: string; geo_space_id?: string; position: number }
) => api.post<{ id: string }>(`/comparisons/${id}/items`, input);
export const addComparisonDimension = (
	id: string,
	input: { label: string; dimension_type: string; position: number }
) => api.post<{ id: string }>(`/comparisons/${id}/dimensions`, input);
/** PUT /comparisons/dimensions/{id}/values — batch upsert of cell values. */
export const setDimensionValues = (
	dimensionId: string,
	values: Array<{ item_id: string; value?: string; value_rating?: number; value_boolean?: boolean }>
) => api.put(`/comparisons/dimensions/${dimensionId}/values`, { values });
