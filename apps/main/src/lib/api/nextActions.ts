// api/nextActions.ts — §122 Next-Action Suggestions (ADR-132).
// Wraps /api/v1/next-actions — OptionalAuth, serves guests + members.
// All functions degrade gracefully (null/empty on failure) so suggestion
// UI never blocks the shell.
import { api } from '$lib/api/client';
import type { NextAction } from '$lib/types';

export async function getNextActions(surface: string): Promise<NextAction[]> {
	try {
		const res = await api.get<NextAction[]>(`/next-actions?surface=${surface}`);
		return res.data ?? [];
	} catch {
		return [];
	}
}

export function dismissNextAction(id: string, snoozeHours?: number) {
	return api.post(`/next-actions/${id}/dismiss`, snoozeHours ? { snooze_hours: snoozeHours } : {}).catch(() => {});
}

export function completeNextAction(id: string) {
	return api.post(`/next-actions/${id}/complete`, {}).catch(() => {});
}

export function trackNextAction(id: string, event: 'shown' | 'clicked') {
	void api.post(`/next-actions/${id}/track`, { event }).catch(() => {});
}
