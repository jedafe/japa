// Tips & Donations API client — §125.
// Ref: japaconnect-engineering/sections/125-donations-tips.md
import { api } from './client';

export interface TipExternalLink {
	id?: string;
	kind: 'paypal' | 'kofi' | 'buymeacoffee' | 'opencollective' | 'other';
	label: string;
	url: string;
	position?: number;
}

export interface TipConfig {
	enabled: boolean;
	methods: string[];
	min_tip_amount?: number;
	fiat_currency?: string;
	thank_you_message?: string | null;
	oc_collective_slug?: string | null;
	external_links?: TipExternalLink[];
}

export interface TipSettings {
	id?: string;
	owner_type: string;
	tips_enabled: boolean;
	accept_points: boolean;
	accept_credits: boolean;
	accept_fiat: boolean;
	accept_external: boolean;
	oc_collective_slug?: string | null;
	has_oc_token?: boolean;
	oc_webhook_url?: string | null;
	min_tip_amount: number;
	thank_you_message?: string | null;
}

export interface Tip {
	id: string;
	sender_user_id?: string | null;
	sender_name?: string;
	recipient_type: string;
	recipient_user_id?: string | null;
	recipient_org_id?: string | null;
	recipient_name?: string;
	content_id?: string | null;
	method: string;
	points_amount?: number;
	credits_amount?: number;
	fiat_amount?: number | null;
	fiat_currency?: string | null;
	fee_amount?: number;
	net_credits?: number;
	message?: string | null;
	is_anonymous: boolean;
	status: string;
	created_at: string;
}

export interface CreateTipResult {
	tip: Tip;
	checkout_url?: string;
}

export interface TipSummary {
	count: number;
	credits_total: number;
	points_total: number;
}

export const getUserTipConfig = (username: string) => api.get<TipConfig>(`/users/${username}/tip-config`);
export const getOrgTipConfig = (orgId: string) => api.get<TipConfig>(`/organizations/${orgId}/donation-config`);
export const getPlatformDonateConfig = () => api.get<TipConfig>(`/donate`);
export const getContentTipSummary = (contentId: string) => api.get<TipSummary>(`/content/${contentId}/tip-summary`);

export const sendTip = (body: {
	recipient_type: string;
	recipient_id?: string;
	content_id?: string;
	method: string;
	amount: number;
	currency?: string;
	message?: string;
	is_anonymous?: boolean;
}, idempotencyKey: string) =>
	api.post<CreateTipResult>('/tips', body, { 'idempotency-key': idempotencyKey });

export const getMyTipSettings = () =>
	api.get<{ settings: TipSettings; external_links: TipExternalLink[] }>('/me/tip-settings');

export const updateMyTipSettings = (body: Partial<TipSettings>) =>
	api.put<TipSettings>('/me/tip-settings', body);

export const createTipLink = (body: TipExternalLink) => api.post<TipExternalLink>('/me/tip-settings/links', body);
export const updateTipLink = (id: string, body: TipExternalLink) =>
	api.patch<TipExternalLink>(`/me/tip-settings/links/${id}`, body);
export const deleteTipLink = (id: string) => api.delete<void>(`/me/tip-settings/links/${id}`);

export const connectOC = (body: { collective_slug: string; api_token?: string }) =>
	api.put<TipSettings>('/me/tip-settings/opencollective', body);
export const disconnectOC = () => api.delete<void>('/me/tip-settings/opencollective');

export const listTipsSent = (page = 1, pageSize = 20) =>
	api.get<Tip[]>(`/me/tips/sent?page=${page}&page_size=${pageSize}`);
export const listTipsReceived = (page = 1, pageSize = 20) =>
	api.get<Tip[]>(`/me/tips/received?page=${page}&page_size=${pageSize}`);

export const newIdempotencyKey = () => crypto.randomUUID();
