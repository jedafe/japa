import { api } from './client';
import type {
	ComparisonType,
	CreateQuickComparisonInput,
	JCEComparisonDetail,
	JCEComparisonSummary,
	MatrixItem,
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

export async function searchEntities(
	type: string,
	query: string,
	filters: Record<string, string> = {},
	limit?: number
): Promise<ResolvedItem[]> {
	const params = new URLSearchParams({ q: query });
	if (limit) params.set('limit', String(limit));
	for (const [k, v] of Object.entries(filters)) {
		if (v) params.set(k, v);
	}
	const res = await api.get<ResolvedItem[]>(`/jce/entities/${type}?${params.toString()}`);
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

export async function createQuickComparisonFromToken(
	token: string,
	typeID: string,
	title?: string
): Promise<JCEComparisonSummary | null> {
	const res = await api.post<JCEComparisonSummary>('/jce/comparisons/from-token', {
		token,
		type_id: typeID,
		title: title ?? 'Quick comparison'
	});
	return res.data;
}

export async function createQuickComparison(
	input: CreateQuickComparisonInput
): Promise<JCEComparisonSummary | null> {
	const res = await api.post<JCEComparisonSummary>('/jce/comparisons', {
		...input,
		depth: 'quick',
		status: 'draft',
		visibility: 'private'
	});
	return res.data;
}

export async function getQuickComparison(slug: string): Promise<JCEComparisonSummary | null> {
	const res = await api.get<JCEComparisonSummary>(`/jce/comparisons/${slug}`);
	return res.data;
}

export async function listComparisons(query: string = ''): Promise<{ items: JCEComparisonSummary[] }> {
	const res = await api.get<JCEComparisonSummary[]>(`/jce/comparisons?${query}`);
	return { items: res.data ?? [] };
}

export async function getComparison(slug: string): Promise<JCEComparisonDetail | null> {
	const res = await api.get<JCEComparisonDetail>(`/jce/comparisons/${slug}`);
	return res.data;
}

export async function getMatrix(slug: string): Promise<MatrixItem[]> {
	const res = await api.get<MatrixItem[]>(`/jce/comparisons/${slug}/matrix`);
	return res.data ?? [];
}

export async function deleteComparison(id: string): Promise<void> {
	await api.delete<unknown>(`/jce/comparisons/${id}`);
}

export async function promoteComparison(id: string): Promise<JCEComparisonSummary | null> {
	const res = await api.post<JCEComparisonSummary>(`/jce/comparisons/${id}/promote`);
	return res.data;
}
