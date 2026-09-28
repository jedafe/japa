import { redirect } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { apiContext, apiList, apiGet } from '$lib/server/api';
import type { Notification } from '$lib/types';

/**
 * Notifications at /notifications — auth-gated. SSR-fetches the first page so
 * the list paints immediately; the unread count drives the page header badge.
 * Mark-read / mark-all-read happen client-side via the api client.
 *
 * Ref: japaconnect-engineering/sections/11-notifications.md
 */
const PAGE_SIZE = 30;

export const load: PageServerLoad = async (event) => {
	if (!event.locals.user) {
		throw redirect(307, '/login?redirect=/notifications');
	}

	const ctx = apiContext(event);

	let notifications: Notification[] = [];
	let total: number | null = null;
	let unreadCount = 0;
	try {
		const res = await apiList<Notification>(ctx, `/me/notifications?page_size=${PAGE_SIZE}`);
		notifications = res.items;
		// §35: the list endpoint reports { total, has_more } alongside `data`
		// so clients can stop paginating deterministically.
		total = res.meta?.total ?? null;
	} catch {
		notifications = [];
	}

	try {
		const res = await apiGet<{ count: number }>(ctx, '/me/notifications/unread-count');
		unreadCount = res.count;
	} catch {
		unreadCount = 0;
	}

	return { notifications, total, unreadCount, pageSize: PAGE_SIZE };
};
