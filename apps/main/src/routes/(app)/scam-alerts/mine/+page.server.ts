// Ref: japaconnect-engineering/sections/81-scam-alert-board.md
import { error, redirect } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { apiContext, apiGet, apiList } from '$lib/server/api';
import type { ScamReport, ScamAlertSubscription } from '$lib/types';

export const load: PageServerLoad = async (event) => {
	if (!event.locals.user) {
		throw redirect(307, `/login?redirect=${encodeURIComponent(event.url.pathname)}`);
	}
	const { features } = await event.parent();
	if (!features['scam_alert.enabled']) {
		throw error(404, { code: 'feature_disabled', message: 'Scam alert board is not available' });
	}
	const ctx = apiContext(event);
	const [reports, subscriptions] = await Promise.allSettled([
		apiList<ScamReport>(ctx, '/me/scam-alerts?limit=100'),
		apiGet<ScamAlertSubscription[]>(ctx, '/me/scam-alert-subscriptions')
	]);
	return {
		reports: reports.status === 'fulfilled' ? reports.value.items : [],
		subscriptions: subscriptions.status === 'fulfilled' ? (subscriptions.value ?? []) : [],
		user: event.locals.user
	};
};
