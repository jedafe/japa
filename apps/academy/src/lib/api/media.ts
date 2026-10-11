import { api } from '$lib/api/client';
import type {
	MediaPickerItem,
	StockApi,
	StockProvider,
	StockSearchResult
} from '@japaconnect/ui';

/**
 * Two-step presigned-URL media upload (§34) — ported from the main app's
 * create.ts. POST /media/upload returns {media_id, presigned_url}; the file
 * bytes go straight to SeaweedFS via PUT (self-authorizing URL — no auth
 * header, no JSON envelope); POST /media/confirm/{id} finalizes the row.
 *
 * Used by the curriculum editor to attach video/document/audio files to
 * lessons (academy_lessons.media_id).
 */

export interface InitiateUploadInput {
	filename: string;
	mime_type: string;
	file_size: number;
	/** 'media' (default) covers video/audio; 'document' covers pdf/doc/etc. */
	context?: 'media' | 'document' | 'avatar';
}

export interface InitiateUploadResult {
	media_id: string;
	storage_path: string;
	presigned_url: string;
	expires_at: string;
}

export const initiateUpload = (input: InitiateUploadInput) =>
	api.post<InitiateUploadResult>('/media/upload', input);

export const confirmMedia = (id: string) =>
	api.post<{ confirmed: boolean }>(`/media/confirm/${id}`);

/** PUT the raw file bytes to the presigned URL (outside the api client). */
async function uploadToPresigned(presignedUrl: string, file: File): Promise<boolean> {
	try {
		const res = await fetch(presignedUrl, { method: 'PUT', body: file });
		return res.ok;
	} catch {
		return false;
	}
}

/**
 * Full upload pipeline — returns the confirmed media_id or an error message.
 * `context` picks the backend MIME allowlist ('document' for pdf/doc files).
 */
export async function uploadLessonFile(
	file: File,
	context: 'media' | 'document' = 'media'
): Promise<{ mediaId: string | null; error: string | null }> {
	const init = await initiateUpload({
		filename: file.name,
		mime_type: file.type || 'application/octet-stream',
		file_size: file.size,
		context
	});
	if (init.error || !init.data) {
		return { mediaId: null, error: init.error?.message ?? 'Upload initiation failed' };
	}

	const uploaded = await uploadToPresigned(init.data.presigned_url, file);
	if (!uploaded) {
		return { mediaId: null, error: 'File transfer failed' };
	}

	const conf = await confirmMedia(init.data.media_id);
	if (conf.error) {
		return { mediaId: null, error: conf.error.message };
	}

	return { mediaId: init.data.media_id, error: null };
}

/* ---- media library picker (shared MediaPickerModal → onPickImage) ---- */

/** GET /me/media — the caller's own library (media.library.view_own). */
export async function listMyMedia(params: { type?: string; q?: string } = {}): Promise<MediaPickerItem[]> {
	const qs = new URLSearchParams();
	if (params.type) qs.set('type', params.type);
	if (params.q) qs.set('q', params.q);
	const suffix = qs.toString() ? `?${qs.toString()}` : '';
	const res = await api.get<MediaPickerItem[]>(`/me/media${suffix}`);
	if (res.error || !res.data) throw new Error(res.error?.message ?? 'Failed to load media');
	return res.data;
}

/**
 * Upload pipeline shaped for MediaPickerModal's `onUpload`: initiate → PUT →
 * confirm → fetch the confirmed row for its served URL. The file arrives
 * already converted by the picker's imageConvert step.
 */
export async function uploadMediaImage(
	file: File,
	context: 'media' | 'avatar' | 'document' = 'media'
): Promise<MediaPickerItem> {
	const init = await initiateUpload({
		filename: file.name,
		mime_type: file.type || 'application/octet-stream',
		file_size: file.size,
		context
	});
	if (init.error || !init.data) throw new Error(init.error?.message ?? 'Upload initiation failed');
	if (!(await uploadToPresigned(init.data.presigned_url, file))) throw new Error('File transfer failed');
	const conf = await confirmMedia(init.data.media_id);
	if (conf.error) throw new Error(conf.error.message);

	const asset = await api.get<MediaPickerItem>(`/media/${init.data.media_id}`);
	if (asset.data?.url) return asset.data;
	return {
		id: init.data.media_id,
		url: `/api/v1/media/serve/media/${init.data.storage_path}`,
		filename: file.name,
		mime_type: file.type
	};
}

/** Stock photo providers (Unsplash/Pexels et al.) — Stock tab of the picker. */
export const stockApi: StockApi = {
	listProviders: async () => {
		const res = await api.get<{ providers?: StockProvider[] }>('/stock/providers');
		if (res.error || !res.data) throw new Error(res.error?.message ?? 'Failed to load stock providers');
		return res.data.providers ?? [];
	},
	search: async (params) => {
		const qs = new URLSearchParams({ q: params.q });
		if (params.provider) qs.set('provider', params.provider);
		if (params.page) qs.set('page', String(params.page));
		if (params.per_page) qs.set('per_page', String(params.per_page));
		const res = await api.get<StockSearchResult>(`/stock/search?${qs.toString()}`);
		if (res.error || !res.data) throw new Error(res.error?.message ?? 'Stock search failed');
		return res.data;
	},
	import: async (provider, id) => {
		const res = await api.post<{ media_id?: string }>('/stock/import', { provider, id });
		if (res.error || !res.data) throw new Error(res.error?.message ?? 'Import failed');
		return res.data;
	}
};
