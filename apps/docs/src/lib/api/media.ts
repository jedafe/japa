// Media library client helpers (§34) — feeds the shared MediaPickerModal so
// the docs editor's image button opens the same picker as the main app's
// /create composer. Mirrors the two-step presigned-URL upload and the stock
// photo endpoints used by main; client-side only.

import { PUBLIC_JC_API_URL } from '$app/env/public';
import { buildApiUrl } from '@japaconnect/api-url';
import { apiPost } from './client';
import type {
	MediaPickerItem,
	StockApi,
	StockProvider,
	StockSearchResult
} from '@japaconnect/ui';

const API_BASE = (PUBLIC_JC_API_URL ?? '/api/v1').replace(/\/+$/, '');
const JC_SURFACE = 'docs';

async function apiGetAuthed<T>(path: string): Promise<T> {
	// Same-origin authed proxy — the session Bearer cannot be attached from
	// browser JS (HttpOnly cookie), so calls needing auth bounce through it.
	const res = await fetch(`/api/authed${path}`, {
		headers: { Accept: 'application/json', 'X-JC-Surface': JC_SURFACE }
	});
	const json = await res.json().catch(() => null);
	if (!res.ok) throw new Error(json?.error?.message ?? json?.message ?? res.statusText ?? 'Request failed');
	return (json?.data ?? json) as T;
}

/** GET /me/media — the caller's own library (media.library.view_own). */
export async function listMyMedia(params: { type?: string; q?: string } = {}): Promise<MediaPickerItem[]> {
	const qs = new URLSearchParams();
	if (params.type) qs.set('type', params.type);
	if (params.q) qs.set('q', params.q);
	const suffix = qs.toString() ? `?${qs.toString()}` : '';
	return apiGetAuthed<MediaPickerItem[]>(`/me/media${suffix}`);
}

interface InitiateUploadResult {
	media_id: string;
	storage_path: string;
	presigned_url: string;
	expires_at: string;
}

/**
 * Full upload pipeline for editor images: POST /media/upload → PUT bytes to
 * the self-authorizing presigned URL → POST /media/confirm/{id} → GET the
 * confirmed row for its served URL. The file arrives already converted by
 * the picker's imageConvert step.
 */
export async function uploadMediaImage(
	file: File,
	context: 'media' | 'avatar' | 'document' = 'media'
): Promise<MediaPickerItem> {
	const init = await apiPost<InitiateUploadResult>('/media/upload', {
		filename: file.name,
		mime_type: file.type || 'application/octet-stream',
		file_size: file.size,
		context
	});
	if (!init.ok) throw new Error(init.error);

	// The presigned_url targets the API's /seaweedfs/upload/{bucket}/{path}
	// route — RequireAuth + owner-bound, so browser JS (HttpOnly cookie) must
	// bounce the bytes through the same-origin authed proxy.
	const presigned = new URL(init.data.presigned_url);
	const putPath = presigned.pathname.replace(/^\/api\/v1/, '');
	const put = await fetch(`/api/authed${putPath}${presigned.search}`, {
		method: 'PUT',
		body: file,
		headers: { 'content-type': file.type || 'application/octet-stream' }
	});
	if (!put.ok) throw new Error('File transfer failed');

	const conf = await apiPost<{ confirmed: boolean }>(`/media/confirm/${init.data.media_id}`);
	if (!conf.ok) throw new Error(conf.error);

	const res = await fetch(`/api/authed/media/${init.data.media_id}`, {
		headers: { Accept: 'application/json', 'X-JC-Surface': JC_SURFACE }
	});
	const json = await res.json().catch(() => null);
	const asset = res.ok ? (json?.data ?? json) as MediaPickerItem | null : null;

	// Fallback: initiateUpload carries no bucket, but the presigned URL
	// embeds it as /seaweedfs/upload/{bucket}/{path} — a hardcoded 'media'
	// bucket here 404s into the placeholder SVG.
	const bucket = init.data.presigned_url.split('/seaweedfs/upload/')[1]?.split('/')[0] ?? 'media';
	return asset?.url
		? asset
		: { id: init.data.media_id, url: buildApiUrl(`/media/serve/${bucket}/${init.data.storage_path}`, API_BASE), filename: file.name, mime_type: file.type };
}

/** PATCH /media/{id} — update the caller's own media metadata (media.edit_own). */
export async function updateMediaImage(
	id: string,
	updates: { alt_text?: string | null; caption?: string | null }
): Promise<void> {
	const res = await fetch(`/api/authed/media/${id}`, {
		method: 'PATCH',
		headers: {
			Accept: 'application/json',
			'Content-Type': 'application/json',
			'X-JC-Surface': JC_SURFACE
		},
		body: JSON.stringify(updates)
	});
	if (!res.ok) {
		const json = await res.json().catch(() => null);
		throw new Error(json?.error?.message ?? json?.message ?? 'Update failed');
	}
}

/** Stock photo providers (Unsplash/Pexels et al.) — Stock tab of the picker. */
export const stockApi: StockApi = {
	listProviders: async () => {
		const res = await apiGetAuthed<{ providers?: StockProvider[] } | StockProvider[]>('/stock/providers');
		return Array.isArray(res) ? res : res.providers ?? [];
	},
	search: async (params) => {
		const qs = new URLSearchParams({ q: params.q });
		if (params.provider) qs.set('provider', params.provider);
		if (params.page) qs.set('page', String(params.page));
		if (params.per_page) qs.set('per_page', String(params.per_page));
		return apiGetAuthed<StockSearchResult>(`/stock/search?${qs.toString()}`);
	},
	import: async (provider, id) => {
		const res = await apiPost<{ media_id?: string }>('/stock/import', { provider, id });
		if (!res.ok) throw new Error(res.error);
		return res.data;
	}
};
