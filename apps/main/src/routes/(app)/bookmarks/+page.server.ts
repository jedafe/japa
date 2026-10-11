import { redirect } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';

// The seeded main_sidebar / main_header menus link "Bookmarks" to /bookmarks,
// which has no dedicated route in the main app (the academy app has its own
// /bookmarks). Redirect to the canonical bookmarks surface so the seeded
// link resolves instead of 404ing.
// Ref: japaconnect-engineering/sections/50-user-dashboard-settings.md
export const load: PageServerLoad = async () => {
	throw redirect(301, '/dashboard/bookmarks');
};
