import { redirect } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';

/**
 * Wallet has moved to the settings hub.
 * Ref: japaconnect-engineering/sections/50-user-dashboard-settings.md
 */
export const load: PageServerLoad = async () => {
	throw redirect(308, '/settings/wallet');
};
