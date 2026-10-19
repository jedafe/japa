import { api } from '$lib/api/client';
import type { ByokProviderOption, UserAIKeyView } from '$lib/ai/agentTypes';

/**
 * BYOK client wrappers — user-owned AI provider keys (spec:
 * docs/superpowers/specs/2026-10-19-ai-byok-design.md).
 */

export const listByokProviders = () => api.get<ByokProviderOption[]>('/ai/byok/providers');
export const listMyAIKeys = () => api.get<UserAIKeyView[]>('/me/ai-keys');
export const createAIKey = (v: { byok_provider_id: string; api_key: string; label?: string }) =>
	api.post<UserAIKeyView>('/me/ai-keys', v);
export const updateAIKey = (id: string, v: { label?: string; api_key?: string }) =>
	api.patch<UserAIKeyView>(`/me/ai-keys/${id}`, v);
export const deleteAIKey = (id: string) => api.delete(`/me/ai-keys/${id}`);
export const verifyAIKey = (id: string) => api.post<{ status: string }>(`/me/ai-keys/${id}/verify`);

/** Fire-and-forget affiliate click counter — never blocks the link. */
export const trackByokReferral = (providerId: string) => {
	void api.post(`/ai/byok/providers/${providerId}/referral-click`).catch(() => undefined);
};
