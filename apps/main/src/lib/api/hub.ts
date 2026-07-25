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
export const patchProfile = (v: Partial<MeProfile>) => api.patch<MeProfile>('/me/profile', v);
export const patchAccount = (v: Record<string, unknown>) => api.patch<MeProfile>('/me/account', v);
export const patchPrivacy = (v: PrivacySettings) => api.patch<PrivacySettings>('/me/privacy', v);
