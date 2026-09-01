import { PUBLIC_JC_API_URL } from '$env/static/public';
import { api, API_BASE } from './client';
import { buildApiUrl } from './url';
import { getCsrfToken } from './csrf.svelte';
import type {
	AdvisorResult,
	ComparisonProfile,
	ComparisonProfileInput,
	ComparisonRecommendation,
	ComparisonType,
	CreateQuickComparisonInput,
	FreshnessSummary,
	IngestionJob,
	JCEComparisonSummary,
	ProfileStat,
	ResolvedItem,
	ScoreFactor,
	ScorePreviewInput,
	ScoreResult,
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

export async function deleteComparison(id: string): Promise<void> {
	await api.delete<unknown>(`/jce/comparisons/${id}`);
}

export async function promoteComparison(id: string): Promise<JCEComparisonSummary | null> {
	const res = await api.post<JCEComparisonSummary>(`/jce/comparisons/${id}/promote`);
	return res.data;
}

export async function listProfiles(): Promise<ComparisonProfile[]> {
	const res = await api.get<ComparisonProfile[]>('/jce/profiles');
	return res.data ?? [];
}

export async function createProfile(body: ComparisonProfileInput): Promise<ComparisonProfile | null> {
	const res = await api.post<ComparisonProfile>('/jce/profiles', body);
	return res.data;
}

export async function getProfile(id: string): Promise<ComparisonProfile | null> {
	const res = await api.get<ComparisonProfile>(`/jce/profiles/${id}`);
	return res.data;
}

export async function updateProfile(id: string, body: ComparisonProfileInput): Promise<ComparisonProfile | null> {
	const res = await api.patch<ComparisonProfile>(`/jce/profiles/${id}`, body);
	return res.data;
}

export async function deleteProfile(id: string): Promise<void> {
	await api.delete<unknown>(`/jce/profiles/${id}`);
}

export async function setDefaultProfile(id: string): Promise<void> {
	await api.post<unknown>(`/jce/profiles/${id}/default`);
}

export async function getProfileStats(all = false): Promise<ProfileStat[]> {
	const qs = all ? '?all=true' : '';
	const res = await api.get<ProfileStat[]>(`/jce/profiles/stats${qs}`);
	return res.data ?? [];
}

export async function listScoreFactors(typeID: string): Promise<ScoreFactor[]> {
	const res = await api.get<ScoreFactor[]>(`/jce/comparison-types/${typeID}/score-factors`);
	return res.data ?? [];
}

export async function getScore(comparisonID: string, profileID?: string): Promise<ScoreResult | null> {
	const qs = profileID ? `?profile_id=${encodeURIComponent(profileID)}` : '';
	const res = await api.get<ScoreResult>(`/jce/comparisons/${comparisonID}/score${qs}`);
	return res.data;
}

export async function scoreComparison(
	comparisonID: string,
	profileID?: string,
	smartMode?: string,
	force?: boolean
): Promise<ScoreResult | null> {
	const body: Record<string, unknown> = {};
	if (profileID) body.profile_id = profileID;
	if (smartMode) body.smart_mode = smartMode;
	if (force) body.force = true;
	const res = await api.post<ScoreResult>(`/jce/comparisons/${comparisonID}/score`, body);
	return res.data;
}

export async function listRecommendations(comparisonID: string, profileID?: string): Promise<ComparisonRecommendation[]> {
	const qs = profileID ? `?profile_id=${encodeURIComponent(profileID)}` : '';
	const res = await api.get<ComparisonRecommendation[]>(`/jce/comparisons/${comparisonID}/recommendations${qs}`);
	return res.data ?? [];
}

export async function previewScore(input: ScorePreviewInput): Promise<ScoreResult | null> {
	const res = await api.post<ScoreResult>('/jce/score-preview', input);
	return res.data;
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

export async function analyzeComparison(
	comparisonID: string,
	question: string,
	profileID?: string
): Promise<AdvisorResult | null> {
	const res = await api.post<AdvisorResult>(`/jce/comparisons/${comparisonID}/analyze`, {
		question,
		profile_id: profileID
	});
	return res.data ?? null;
}

export async function rankComparison(comparisonID: string, profileID?: string): Promise<AdvisorResult | null> {
	const res = await api.post<AdvisorResult>(`/jce/comparisons/${comparisonID}/rank`, {
		profile_id: profileID
	});
	return res.data ?? null;
}

export async function explainComparison(
	comparisonID: string,
	itemID: string,
	factorKey: string,
	question: string,
	profileID?: string
): Promise<AdvisorResult | null> {
	const res = await api.post<AdvisorResult>(`/jce/comparisons/${comparisonID}/explain`, {
		item_id: itemID,
		factor_key: factorKey,
		question,
		profile_id: profileID
	});
	return res.data ?? null;
}

export async function recommendComparison(comparisonID: string, profileID?: string): Promise<AdvisorResult | null> {
	const res = await api.post<AdvisorResult>(`/jce/comparisons/${comparisonID}/recommend`, {
		profile_id: profileID
	});
	return res.data ?? null;
}

export interface AdvisorStreamChunk {
	type: string;
	content?: string;
}

/**
 * Streams the JCE AI advisor response over a POST request.
 *
 * The backend emits a single SSE `data:` event for the full answer, but the
 * consumer receives each parsed chunk as it arrives.
 *
 * Ref: apps/jce/src/lib/api/client.ts (CSRF + API base conventions).
 */
export async function streamAdvisor(
	comparisonID: string,
	question: string,
	profileID: string | undefined,
	onChunk: (chunk: AdvisorStreamChunk) => void,
	signal?: AbortSignal
): Promise<void> {
	const token = getCsrfToken();
	const headers: Record<string, string> = {
		'content-type': 'application/json',
		accept: 'text/event-stream'
	};
	if (token) headers['x-csrf-token'] = token;

	const res = await fetch(buildApiUrl(`/jce/comparisons/${comparisonID}/advisor`, API_BASE), {
		method: 'POST',
		credentials: 'include',
		headers,
		body: JSON.stringify({ question, profile_id: profileID }),
		signal
	});

	if (!res.ok) {
		let body: unknown = null;
		try {
			body = await res.json();
		} catch {
			body = null;
		}
		const err = (body as { error?: { code: string; message: string; status?: number } } | null)?.error;
		throw new Error(err?.message ?? res.statusText);
	}

	if (!res.body) return;

	const reader = res.body.getReader();
	const decoder = new TextDecoder();
	let buffer = '';

	function parseLine(line: string) {
		const match = line.match(/^data:\s*(.*)$/);
		if (!match) return;
		const text = match[1].trim();
		if (!text) return;
		try {
			const event = JSON.parse(text) as AdvisorStreamChunk;
			onChunk(event);
		} catch {
			onChunk({ type: 'message', content: text });
		}
	}

	try {
		while (true) {
			const { done, value } = await reader.read();
			if (done) break;
			buffer += decoder.decode(value, { stream: true });
			const lines = buffer.split('\n');
			buffer = lines.pop() ?? '';
			for (const line of lines) {
				parseLine(line);
			}
		}
		if (buffer.trim()) {
			for (const line of buffer.split('\n')) {
				parseLine(line);
			}
		}
	} finally {
		reader.releaseLock();
	}
}
