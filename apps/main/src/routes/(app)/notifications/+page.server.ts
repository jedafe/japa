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

	const [listRes, unreadCount] = await Promise.all([
		// §35: the list endpoint reports { total, has_more } alongside `data`
		// so clients can stop paginating deterministically.
		apiList<Notification>(ctx, `/me/notifications?page_size=${PAGE_SIZE}`)
			.then((res) => ({ items: res.items, total: res.meta?.total ?? null as number | null }))
			.catch(() => ({ items: [] as Notification[], total: null as number | null })),
		apiGet<{ count: number }>(ctx, '/me/notifications/unread-count')
			.then((res) => res.count)
			.catch(() => 0)
	]);
	const notifications = listRes.items;
	const total = listRes.total;

	return { notifications, total, unreadCount, pageSize: PAGE_SIZE };
};
