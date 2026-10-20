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
	audience: Record<string, unknown>;
	conditions: Record<string, unknown>;
	payload: { title: string; body: string; cta_label: string; cta_url: string; icon: string };
	channels: string[];
	completion_signal: Record<string, unknown> | null;
	frequency: Record<string, unknown>;
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

export const listRules = () => api<NextActionRule[]>('/admin/next-actions/rules');
export const getRule = (id: string) => api<NextActionRule>(`/admin/next-actions/rules/${id}`);
export const createRule = (body: Partial<NextActionRule>) =>
	api<NextActionRule>('/admin/next-actions/rules', { method: 'POST', body: JSON.stringify(body) });
export const updateRule = (id: string, body: Partial<NextActionRule>) =>
	api<NextActionRule>(`/admin/next-actions/rules/${id}`, { method: 'PATCH', body: JSON.stringify(body) });
export const deleteRule = (id: string) =>
	api<null>(`/admin/next-actions/rules/${id}`, { method: 'DELETE' });
export const getStats = () => api<NextActionStat[]>('/admin/next-actions/stats');
