// Media library client helpers (§34) — feeds the shared MediaPickerModal so
// the docs editor's image button opens the same picker as the main app's
// /create composer. Mirrors the two-step presigned-URL upload and the stock
// photo endpoints used by main; client-side only.

import { env } from '$env/dynamic/public';
import { buildApiUrl } from '@japaconnect/api-url';
import { apiPost, getCookie } from './client';
import type {
	MediaPickerItem,
	StockApi,
	StockProvider,
	StockSearchResult
} from '@japaconnect/ui';

const API_BASE = (env.PUBLIC_JC_API_URL ?? '/api/v1').replace(/\/+$/, '');
const JC_SURFACE = 'docs';

function authHeaders(): Record<string, string> {
	const headers: Record<string, string> = {
		Accept: 'application/json',
		'X-JC-Surface': JC_SURFACE
	};
	const token = getCookie('jc_token');
	if (token) headers['Authorization'] = `Bearer ${token}`;
	return headers;
}

async function apiGetAuthed<T>(path: string): Promise<T> {
	const res = await fetch(buildApiUrl(path, API_BASE), {
		credentials: 'include',
		headers: authHeaders()
	});
	const json = await res.json().catch(() => null);
	if (!res.ok) throw new Error(json?.error?.message ?? res.statusText ?? 'Request failed');
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

	const put = await fetch(init.data.presigned_url, {
		method: 'PUT',
		body: file,
		credentials: 'include',
		headers: authHeaders()
	});
	if (!put.ok) throw new Error('File transfer failed');

	const conf = await apiPost<{ confirmed: boolean }>(`/media/confirm/${init.data.media_id}`);
	if (!conf.ok) throw new Error(conf.error);

	const res = await fetch(buildApiUrl(`/media/${init.data.media_id}`, API_BASE), {
		credentials: 'include',
		headers: authHeaders()
	});
	const json = await res.json().catch(() => null);
	const asset = res.ok ? ((json?.data ?? json) as MediaPickerItem | null) : null;

	return asset?.url
		? asset
		: { id: init.data.media_id, url: `/api/v1/media/serve/media/${init.data.storage_path}`, filename: file.name, mime_type: file.type };
}

/** Stock photo providers (Unsplash/Pexels et al.) — Stock tab of the picker. */
export const stockApi: StockApi = {
	listProviders: async () => {
		const res = await apiGetAuthed<{ providers?: StockProvider[] } | StockProvider[]>('/stock/providers');
		return Array.isArray(res) ? res : (res.providers ?? []);
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
