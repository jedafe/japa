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
export const patchProfile = (v: Partial<MeProfile>) => {
	// All keys are already snake_case (matching backend json tags).
	return api.patch<MeProfile>('/me/profile', v);
};
export const patchAccount = (v: Record<string, unknown>) => api.patch<MeProfile>('/me/account', v);
export const patchPrivacy = (v: PrivacySettings) => api.patch<PrivacySettings>('/me/privacy', v);
export const getPrivacyOptions = () =>
	api.get<{ dm_privacy: string[]; profile_visibility: string[] }>('/me/privacy/options');

/* ---- preferences (language / dark_mode) — PUT /me/preferences ---- */
export const putPreferences = (v: Record<string, unknown>) => api.put('/me/preferences', v);

/* ---- security ---- */
export const changePassword = (current: string, next: string) =>
	api.post('/me/change-password', { current_password: current, new_password: next });
export const totpStatus = () => api.get<{ enabled: boolean }>('/me/totp/status');
export const totpEnable = () => api.post<{ secret: string; otpauth_url: string }>('/me/totp/enable');
export const totpConfirm = (code: string) =>
	api.post<{ backup_codes: string[] }>('/me/totp/confirm', { code });
export const totpDisable = (code: string) => api.delete('/me/totp', { code });
export const revokeSession = (id: string) => api.delete(`/me/sessions/${id}`);
export const revokeOtherSessions = () => api.delete('/me/sessions');

/* ---- connected accounts ---- */
export const unlinkAccount = (provider: string) => api.delete(`/me/connected-accounts/${provider}`);

/* ---- notifications ---- */
export const patchNotifications = (v: Record<string, unknown>) =>
	api.patch('/me/notification-preferences', v);

/* ---- devices (mobile push tokens) ---- */
export const listDevices = () => api.get<import('$lib/types').DeviceInfo[]>('/me/devices');
export const deleteDevice = (deviceID: string) =>
	api.delete(`/me/devices/${encodeURIComponent(deviceID)}`);

/* ---- browser push subscriptions ---- */
export const getVapidKey = () => api.get<{ publicKey: string }>('/push/vapid-key');
export const getPushTokens = () => api.get<Array<{ id: string; endpoint: string }>>('/me/push-tokens');
export const subscribePush = (body: { endpoint: string; p256dh: string; auth: string }) =>
	api.post('/me/push-tokens', body);
export const deletePushToken = (id: string) => api.delete(`/me/push-tokens/${id}`);

/* ---- api keys ---- */
export const createApiKey = (v: { name: string; scopes: string[] }) =>
	api.post<{ id: string; key: string }>('/me/api-keys', v);
// Browser-extension mint (Section 47 v1.3) — narrow read scopes are fixed
// server-side; install_id is optional (the page can't know the extension's
// generated ID).
export const createExtensionToken = (v?: { install_id?: string }) =>
	api.post<{ id: string; key: string }>('/me/extension-tokens', v ?? {});
export const rotateApiKey = (id: string) => api.post<{ key: string }>(`/me/api-keys/${id}/rotate`);
export const updateApiKey = (id: string, v: { name?: string; scopes?: string[] }) =>
	api.patch(`/me/api-keys/${id}`, v);
export const revokeApiKey = (id: string) => api.delete(`/me/api-keys/${id}`);

/* ---- affiliate ---- */
// Ref: japaconnect-engineering/sections/56-user-affiliate-program.md.
// The backend CreateLinkInput expects { product_type, destination_url,
// description? } — NOT { target_url, label }. The previous mismatch caused
// every create to 400 with missing_field (BUG-014).
export const createAffiliate = (v: {
	product_type: string;
	destination_url: string;
	description?: string;
}) => api.post('/me/affiliate-links', v);
export const deleteAffiliate = (id: string) => api.delete(`/me/affiliate-links/${id}`);
export const enrollAffiliate = () => api.post('/affiliate-program/enroll');

/* ---- wallet / subscription ---- */
export const getCurrencyPreference = () => api.get<{ preferred_currency: string }>('/me/preferences/currency');
export const setCurrencyPreference = (currencyCode: string) =>
	api.patch<{ preferred_currency: string }>('/me/preferences/currency', { currency_code: currencyCode });

export const listCreditPacks = () => api.get<Array<unknown>>('/wallet/credit-packs');
export const purchaseCredits = (body: { pack_id?: string; credit_amount?: number; currency?: string; provider?: string }) =>
	api.post<{ checkout_url?: string }>('/wallet/purchase-credits', body, {
		// Rule 9: financial endpoints require an Idempotency-Key.
		'Idempotency-Key': crypto.randomUUID()
	});

export const listWithdrawalProviders = () => api.get<string[]>('/wallet/withdraw/providers');

export const calculateWithdrawal = (body: { credit_amount: number; currency_code?: string }) =>
	api.post<{
		credit_amount: number;
		currency_code: string;
		credits_per_unit: number;
		fiat_amount: number;
		fee_amount: number;
		fee_basis: string;
		fee_rate: number;
		net_amount: number;
	}>('/wallet/withdraw/calculate', body);

export const createWithdrawal = (body: { credit_amount: number; currency_code?: string; payout_method_id?: string }) =>
	api.post<{
		id: string;
		credit_amount: number;
		currency_code: string;
		credits_per_unit: number;
		fiat_amount: number;
		fee_amount: number;
		net_amount: number;
		status: string;
		payout_provider?: string | null;
	}>(
		'/me/wallet/withdraw',
		body,
		{
			'Idempotency-Key': crypto.randomUUID()
		}
	);

export const createWithdrawalMethod = (body: {
	provider: string;
	label?: string;
	account: string;
	currency_code: string;
	is_default?: boolean;
	metadata?: unknown;
}) =>
	api.post<{
		id: string;
		provider: string;
		label?: string | null;
		account: string;
		currency_code: string;
		is_verified: boolean;
		is_default: boolean;
		is_active: boolean;
		metadata?: unknown;
	}>('/me/wallet/withdrawal-methods', body);

export const convertWallet = (points: number) =>
	api.post('/me/wallet/convert', { points_to_convert: points }, {
		// Rule 9: financial endpoints require an Idempotency-Key so the backend
		// dedupes retries of the same conversion attempt (and distinct attempts
		// get distinct keys). A fresh UUID per call is the correct granularity.
		'Idempotency-Key': crypto.randomUUID()
	});
export const cancelSubscription = () =>
	api.post('/me/subscription/cancel', undefined, {
		// Rule 9: financial state-change endpoints require an Idempotency-Key.
		'Idempotency-Key': crypto.randomUUID()
	});
export const reactivateSubscription = () =>
	api.post('/me/subscription/reactivate', undefined, {
		// Rule 9: financial state-change endpoints require an Idempotency-Key.
		'Idempotency-Key': crypto.randomUUID()
	});

export interface PaymentProvider {
	id: string;
	name: string;
	description: string;
	enabled: boolean;
}

export const listPaymentProviders = () => api.get<PaymentProvider[]>('/payment-providers');

export const createSubscription = (plan: string, provider: string) =>
	api.post('/subscriptions', { plan, provider }, {
		// Rule 9: financial state-change endpoints require an Idempotency-Key.
		'Idempotency-Key': crypto.randomUUID()
	});

/* ---- danger ---- */
export const exportData = () => api.post('/me/data-export');
export const cancelDataExport = () => api.delete('/me/data-export');
export const deactivateAccount = () => api.post('/me/deactivate');
export const deleteAccount = () => api.delete('/me');

/* ---- content manager (dashboard) ---- */
export const publishContent = (id: string) => api.post(`/content/${id}/publish`);
export const archiveContent = (id: string) => api.post(`/content/${id}/archive`);
export const deleteContent = (id: string) => api.delete(`/content/${id}`);

/* ---- relationships (dashboard) ---- */
export const leaveSpace = (slug: string) => api.delete(`/spaces/${slug}/leave`);
export const unfollowTag = (slug: string) => api.delete(`/tags/${slug}/follow`);
export const unblockUser = (id: string) => api.delete(`/users/${id}/block`);
export const blockUser = (id: string) => api.post(`/users/${id}/block`);
export const muteUser = (id: string) => api.post(`/users/${id}/mute`);
export const unmuteUser = (id: string) => api.delete(`/users/${id}/mute`);
export const unmuteSpace = (slug: string) => api.delete(`/spaces/${slug}/mute`);

export type FollowableType = 'user' | 'organization' | 'space' | 'tag' | 'source';

export interface FollowToggleResponse {
	following?: boolean;
	is_following?: boolean;
	follower_count?: number;
}

export const toggleFollow = (
	type: FollowableType,
	id: string,
	slug: string | undefined,
	currently: boolean
) => {
	const target = type === 'space' || type === 'tag' || type === 'source' ? (slug || id) : id;
	switch (type) {
		case 'user':
			return currently
			? api.delete<FollowToggleResponse>(`/users/${target}/follow`)
				: api.post<FollowToggleResponse>(`/users/${target}/follow`);
		case 'organization':
			return currently
			? api.delete<FollowToggleResponse>(`/organizations/${target}/follow`)
				: api.post<FollowToggleResponse>(`/organizations/${target}/follow`);
		case 'space':
			return currently
			? api.delete<FollowToggleResponse>(`/spaces/${target}/follow`)
				: api.post<FollowToggleResponse>(`/spaces/${target}/follow`);
		case 'tag':
			return currently
			? api.delete<FollowToggleResponse>(`/tags/${target}/follow`)
				: api.post<FollowToggleResponse>(`/tags/${target}/follow`);
		case 'source':
			return currently
			? api.delete<FollowToggleResponse>(`/sources/${target}/follow`)
				: api.post<FollowToggleResponse>(`/sources/${target}/follow`);
	}
};

/* ---- reports (Section 51) ---- */
export const reportUser = (id: string, body: { reason: string; notes?: string; evidence_urls?: string[] }) =>
	api.post<{ id: string }>(`/users/${id}/report`, body);
export const reportContent = (id: string, body: { reason: string; notes?: string; evidence_urls?: string[] }) =>
	api.post<{ id: string }>(`/content/${id}/report`, body);
export const reportComment = (id: string, body: { reason: string; notes?: string; evidence_urls?: string[] }) =>
	api.post<{ id: string }>(`/comments/${id}/report`, body);
export const reportSpace = (id: string, body: { reason: string; notes?: string; evidence_urls?: string[] }) =>
	api.post<{ id: string }>(`/spaces/${id}/report`, body);
export const reportSource = (slug: string, body: { reason: string; notes?: string; evidence_urls?: string[] }) =>
	api.post<{ id: string }>(`/sources/${slug}/report`, body);

/* ---- newsletter preferences (Section 62, ADR-080) ---- */
export const patchNewsletterPreferences = (v: { digest_enabled?: boolean; campaign_enabled?: boolean }) =>
	api.patch<{ digest_enabled: boolean; campaign_enabled: boolean }>('/newsletter/preferences', v);

export const unsubscribeNewsletter = (email: string) =>
	api.delete<{ unsubscribed: boolean }>(`/newsletter/unsubscribe?email=${encodeURIComponent(email)}`);

/* ---- mentorship (Section 102, ADR-110) ---- */
export const acceptMentorship = (id: string) =>
	api.patch<unknown>(`/me/mentorship/incoming/${id}/accept`);
export const declineMentorship = (id: string) =>
	api.patch<unknown>(`/me/mentorship/incoming/${id}/decline`);
export const cancelOutgoingMentorship = (id: string) =>
	api.delete<unknown>(`/me/mentorship/outgoing/${id}`);
export const cancelMentorship = (id: string) =>
	api.patch<unknown>(`/mentorship/${id}/cancel`);
export const completeMentorship = (id: string) =>
	api.patch<unknown>(`/mentorship/${id}/complete`);
