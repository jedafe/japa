// §122 Next-Action Suggestions — admin rule management client.
import { api } from '$lib/api';
import type { ApiResult } from '$lib/api/client';

export interface NextActionRule {
	id: string;
	slug: string;
	name: string;
	description: string;
	enabled: boolean;
	priority: number;
	upsell: boolean;
	audience: Record<string, unknown>;
	conditions: Record<string, unknown>;
	payload: { title: string; body: string; cta_label: string; cta_url: string; icon: string; badge?: string };
	channels: string[];
	completion_signal: Record<string, unknown> | null;
	frequency: Record<string, unknown>;
	valid_from: string | null;
	valid_until: string | null;
	created_at: string;
	updated_at: string;
}

export interface NextActionStat {
	slug: string;
	shown: number;
	clicked: number;
	dismissed: number;
	completed: number;
}

export interface NextActionRuleList {
	data: NextActionRule[];
	total: number;
	page: number;
	total_pages: number;
}

export interface ListRulesParams {
	q?: string;
	enabled?: '' | 'true' | 'false';
	sort?: string;
	sort_dir?: 'asc' | 'desc';
	page_offset?: number;
	page_size?: number;
}

export const listRules = (params: ListRulesParams = {}) => {
	const sp = new URLSearchParams();
	if (params.q) sp.set('q', params.q);
	if (params.enabled) sp.set('enabled', params.enabled);
	if (params.sort) sp.set('sort', params.sort);
	if (params.sort_dir) sp.set('sort_dir', params.sort_dir);
	if (params.page_offset != null) sp.set('page_offset', String(params.page_offset));
	if (params.page_size != null) sp.set('page_size', String(params.page_size));
	const qs = sp.toString();
	return api<NextActionRuleList>(`/admin/next-actions/rules${qs ? `?${qs}` : ''}`);
};
export const getRule = (id: string) => api<NextActionRule>(`/admin/next-actions/rules/${id}`);
export const createRule = (body: Partial<NextActionRule>) =>
	api<NextActionRule>('/admin/next-actions/rules', { method: 'POST', body });
export const updateRule = (id: string, body: Partial<NextActionRule>) =>
	api<NextActionRule>(`/admin/next-actions/rules/${id}`, { method: 'PATCH', body });
export const deleteRule = (id: string) =>
	api<null>(`/admin/next-actions/rules/${id}`, { method: 'DELETE' });
export const getStats = () => api<NextActionStat[]>('/admin/next-actions/stats');

export interface NextActionVocab {
	channels: string[];
	user_classes: string[];
	surfaces: string[];
}
export const getVocab = () => api<NextActionVocab>('/admin/next-actions/vocab');

export interface PredicateSpec {
	name: string;
	args: { key: string; type: string; required?: boolean; options?: string }[];
}
export const getPredicateSpecs = () => api<PredicateSpec[]>('/admin/next-actions/predicates');
