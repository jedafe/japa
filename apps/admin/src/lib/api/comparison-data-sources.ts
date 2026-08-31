import { apiFetch } from '$lib/api/client';
import type { DataSource, IngestionJob } from '$lib/types/jce';

type ApiCallResult<T> = { ok: true; data: T } | { ok: false; error: string };
type PaginatedDataSources = { data: DataSource[]; meta: { total: number; page: number; page_size: number; total_pages: number } };

export const api = {
	get: <T>(path: string) => apiFetch<T>(path, { method: 'GET' }),
	post: <T>(path: string, body?: unknown) =>
		apiFetch<T>(path, { method: 'POST', body: body ? JSON.stringify(body) : undefined }),
	patch: <T>(path: string, body?: unknown) =>
		apiFetch<T>(path, { method: 'PATCH', body: body ? JSON.stringify(body) : undefined }),
	delete: <T>(path: string) => apiFetch<T>(path, { method: 'DELETE' })
};

export async function listDataSources(params: { page?: number; pageSize?: number } = {}): Promise<{ data: DataSource[]; total: number }> {
	const page = params.page ?? 1;
	const pageSize = params.pageSize ?? 50;
	const res = await api.get<PaginatedDataSources>(`/jce/data-sources?page=${page}&page_size=${pageSize}`);
	return { data: res.data?.data ?? [], total: res.data?.meta?.total ?? 0 };
}

export async function getDataSource(id: string): Promise<DataSource | null> {
	const res = await api.get<DataSource>(`/jce/data-sources/${id}`);
	return res.data ?? null;
}

export async function createDataSource(body: Record<string, any>): Promise<ApiCallResult<DataSource>> {
	const res = await api.post<DataSource>('/jce/data-sources', body);
	if (res.error) return { ok: false, error: res.error.message };
	if (res.data == null) return { ok: false, error: 'No data returned' };
	return { ok: true, data: res.data };
}

export async function updateDataSource(id: string, body: Record<string, any>): Promise<ApiCallResult<DataSource>> {
	const res = await api.patch<DataSource>(`/jce/data-sources/${id}`, body);
	if (res.error) return { ok: false, error: res.error.message };
	if (res.data == null) return { ok: false, error: 'No data returned' };
	return { ok: true, data: res.data };
}

export async function deleteDataSource(id: string): Promise<ApiCallResult<unknown>> {
	const res = await api.delete<unknown>(`/jce/data-sources/${id}`);
	if (res.error) return { ok: false, error: res.error.message };
	return { ok: true, data: res.data };
}

export async function listIngestionJobs(contentID: string): Promise<IngestionJob[]> {
	const res = await api.get<IngestionJob[]>(`/jce/comparisons/${contentID}/ingestion-jobs`);
	return res.data ?? [];
}
