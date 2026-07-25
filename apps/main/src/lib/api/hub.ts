import { api } from '$lib/api/client';
import { classifyError } from '$lib/api/create';
import type { MeProfile, PrivacySettings } from '$lib/types';

export { classifyError };

/**
 * Client mutation wrappers for the user hub. Each returns the standard
 * ClientResult envelope ({ data, error, status }) so DirtyForm can classify
 * failures and optimisticAction can roll back.
 *
 * Ref: japaconnect-engineering/sections/50-current-user.md
 */

/* ---- profile / account / privacy (Plan 1) ---- */
export const patchProfile = (v: Partial<MeProfile>) => api.patch<MeProfile>('/me/profile', v);
export const patchAccount = (v: Record<string, unknown>) => api.patch<MeProfile>('/me/account', v);
export const patchPrivacy = (v: PrivacySettings) => api.patch<PrivacySettings>('/me/privacy', v);

/* ---- security ---- */
export const changePassword = (current: string, next: string) =>
	api.post('/me/change-password', { current_password: current, new_password: next });
export const totpStatus = () => api.get<{ enabled: boolean }>('/me/totp/status');
export const totpEnable = () => api.post<{ secret: string; otpauth_url: string }>('/me/totp/enable');
export const totpConfirm = (code: string) =>
	api.post<{ backup_codes: string[] }>('/me/totp/confirm', { code });
export const totpDisable = () => api.delete('/me/totp');
export const revokeSession = (id: string) => api.delete(`/me/sessions/${id}`);
export const revokeOtherSessions = () => api.delete('/me/sessions');

/* ---- connected accounts ---- */
export const unlinkAccount = (provider: string) => api.delete(`/me/connected-accounts/${provider}`);

/* ---- notifications ---- */
export const patchNotifications = (v: Record<string, unknown>) =>
	api.patch('/me/notification-preferences', v);

/* ---- api keys ---- */
export const createApiKey = (v: { name: string; scopes: string[] }) =>
	api.post<{ id: string; key: string }>('/me/api-keys', v);
export const rotateApiKey = (id: string) => api.post<{ key: string }>(`/me/api-keys/${id}/rotate`);
export const updateApiKey = (id: string, v: { name?: string; scopes?: string[] }) =>
	api.patch(`/me/api-keys/${id}`, v);
export const revokeApiKey = (id: string) => api.delete(`/me/api-keys/${id}`);

/* ---- affiliate ---- */
export const createAffiliate = (v: { target_url: string; label?: string }) =>
	api.post('/me/affiliate-links', v);
export const deleteAffiliate = (id: string) => api.delete(`/me/affiliate-links/${id}`);

/* ---- wallet / subscription ---- */
export const convertWallet = (points: number) => api.post('/me/wallet/convert', { points });
export const cancelSubscription = () => api.post('/me/subscription/cancel');
export const reactivateSubscription = () => api.post('/me/subscription/reactivate');

/* ---- danger ---- */
export const exportData = () => api.post('/me/data-export');
export const deactivateAccount = () => api.post('/me/deactivate');
export const deleteAccount = () => api.delete('/me');
