import { api, getToken } from './api';

export function trackEvent(event: string, data: Record<string, unknown> = {}) {
	const token = getToken();
	if (!token) return;

	api.trackAnalyticsEvent({
		event,
		data,
		timestamp: new Date().toISOString()
	}).catch(() => {});
}

export const AnalyticsEvents = {
	PAGE_VIEW: 'page_view',
	CONTENT_VIEW: 'content_view',
	CONTENT_CREATE: 'content_create',
	SEARCH: 'search',
	LOGIN: 'login',
	REGISTER: 'register',
	FOLLOW: 'follow',
	UNFOLLOW: 'unfollow',
	REACTION: 'reaction',
	COMMENT: 'comment',
	SHARE: 'share',
	PAYMENT_INITIATE: 'payment_initiate',
	PAYMENT_COMPLETE: 'payment_complete'
} as const;
