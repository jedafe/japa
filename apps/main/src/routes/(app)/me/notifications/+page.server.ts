import { redirect } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';

/**
 * /me/notifications — BUG-016: previously this page rendered newsletter
 * preferences (digest/campaign toggles), which was misleading since the
 * route name implies notification-type preferences. The actual notification
 * preferences live in the settings hub at /settings/notifications
 * (NotificationsSection.svelte, §35). Redirect there.
 *
 * The newsletter preferences page has been moved to /me/newsletter.
 *
 * Ref: japaconnect-engineering/sections/35-notification-system.md
 *      japaconnect-engineering/sections/50-user-dashboard-settings.md
 */
export const load: PageServerLoad = async () => {
	throw redirect(307, '/settings/notifications');
};
