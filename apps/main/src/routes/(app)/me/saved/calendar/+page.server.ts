// Ref: japaconnect-engineering/sections/86-community-calendar.md
// BUG-003: /me/saved/calendar — user dashboard widget for saved calendar
// entries. §86 Integration: "Saved entries appear on the user dashboard
// alongside visa checklists."
import { redirect } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { apiContext, apiList } from '$lib/server/api';
import type { CalendarEntry } from '$lib/types';

export const load: PageServerLoad = async (event) => {
	if (!event.locals.user) {
		throw redirect(307, `/login?redirect=${encodeURIComponent(event.url.pathname)}`);
	}

	const ctx = apiContext(event);

	try {
		const { items, meta } = await apiList<CalendarEntry>(ctx, '/me/calendar/saved?page_size=20');
		return { entries: items, meta, loadError: false };
	} catch {
		return { entries: [] as CalendarEntry[], meta: null, loadError: true };
	}
};
