import { api, type ClientResult } from './client';
import type {
	ComparisonType,
	CreateQuickComparisonInput,
	FreshnessSummary,
	IngestionJob,
	JCEComparisonSummary,
	ResolvedItem,
	SelectionToken
} from '$lib/types/jce';

export async function listComparisonTypes(): Promise<ComparisonType[]> {
	const res = await api.get<ComparisonType[]>('/jce/comparison-types');
	return res.data ?? [];
}

export async function getComparisonType(slug: string): Promise<ComparisonType | null> {
	const res = await api.get<ComparisonType>(`/jce/comparison-types/${slug}`);
	return res.data;
}

export interface SearchEntitiesOptions {
	page?: number;
	pageSize?: number;
	offset?: number;
}

export async function searchEntities(
	type: string,
	query: string,
	filters: Record<string, string> = {},
	options: SearchEntitiesOptions = {}
): Promise<ResolvedItem[]> {
	const params = new URLSearchParams({ q: query });
	if (options.pageSize) {
		params.set('page_size', String(options.pageSize));
	}
	if (options.page) {
		params.set('page', String(options.page));
	} else if (options.offset) {
		params.set('offset', String(options.offset));
	}
	for (const [k, v] of Object.entries(filters)) {
		if (v) params.set(k, v);
	}
	const res = await api.get<ResolvedItem[]>(`/jce/entities/${type}?${params.toString()}`);
	return res.data ?? [];
}

export async function searchSpaces(query: string, level?: string, limit = 10): Promise<{ id: string; name: string; slug: string }[]> {
	const params = new URLSearchParams({ q: query, page_size: String(limit) });
	if (level) params.set('level', level);
	const res = await api.get<{ id: string; name: string; slug: string }[]>(`/spaces?${params.toString()}`);
	return res.data ?? [];
}

export async function resolveEntity(type: string, id: string): Promise<ResolvedItem | null> {
	const res = await api.get<ResolvedItem>(`/jce/entities/${type}/${id}`);
	return res.data;
}

export async function createGuestSelection(payload: {
	type_slug: string;
	item_refs: Record<string, unknown>[];
}): Promise<string | null> {
	const res = await api.post<SelectionToken>('/jce/selections', payload);
	return res.data?.token ?? null;
}

export async function createQuickComparison(
	input: CreateQuickComparisonInput
): Promise<ClientResult<JCEComparisonSummary>> {
	return api.post<JCEComparisonSummary>('/jce/comparisons', {
		...input,
		depth: 'quick',
		status: 'draft',
		visibility: 'private'
	});
}

export async function getQuickComparison(slug: string): Promise<JCEComparisonSummary | null> {
	const res = await api.get<JCEComparisonSummary>(`/jce/comparisons/${slug}`);
	return res.data;
}

export function deleteComparison(id: string): Promise<ClientResult<unknown>> {
	return api.delete<unknown>(`/jce/comparisons/${id}`);
}

export function promoteComparison(id: string): Promise<ClientResult<JCEComparisonSummary>> {
	return api.post<JCEComparisonSummary>(`/jce/comparisons/${id}/promote`);
}

export async function getFreshness(comparisonID: string): Promise<FreshnessSummary | null> {
	const res = await api.get<FreshnessSummary>(`/jce/comparisons/${comparisonID}/freshness`);
	return res.data ?? null;
}

export async function refreshComparison(
	comparisonID: string,
	sourceKind?: string
): Promise<IngestionJob | null> {
	const body: Record<string, unknown> = {};
	if (sourceKind) body.source_kind = sourceKind;
	const res = await api.post<IngestionJob>(`/jce/comparisons/${comparisonID}/refresh`, body);
	return res.data ?? null;
}

export async function listIngestionJobs(comparisonID: string): Promise<IngestionJob[]> {
	const res = await api.get<IngestionJob[]>(`/jce/comparisons/${comparisonID}/ingestion-jobs`);
	return res.data ?? [];
}
