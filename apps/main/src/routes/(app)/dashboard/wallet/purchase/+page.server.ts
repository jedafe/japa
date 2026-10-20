import { redirect } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';

/**
 * Credit purchase now lives under the settings wallet purchase tab.
 * Ref: japaconnect-engineering/sections/50-user-dashboard-settings.md
 */
export const load: PageServerLoad = async () => {
	throw redirect(308, '/settings/wallet?tab=purchase');
};
