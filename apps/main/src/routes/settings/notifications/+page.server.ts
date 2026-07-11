import type { PageServerLoad, Actions } from './$types';
import { apiFetch } from '$lib/server/api';
import { fail } from '@sveltejs/kit';

interface PreferenceEntry {
	notification_type: string;
	type_label: string;
	in_app: boolean;
	email: boolean;
	push: boolean;
}

interface PrefsMap {
	[key: string]: { in_app: boolean; email: boolean; push: boolean };
}

const NOTIFICATION_TYPES = [
	'email',
	'push',
	'follow',
	'comment',
	'mention',
	'dm',
	'group',
	'marketing'
];

const DEFAULT_PREFS: PrefsMap = {
	email: { in_app: true, email: true, push: true },
	push: { in_app: true, email: true, push: true },
	follow: { in_app: true, email: false, push: false },
	comment: { in_app: true, email: false, push: false },
	mention: { in_app: true, email: false, push: false },
	dm: { in_app: true, email: false, push: false },
	group: { in_app: true, email: false, push: false },
	marketing: { in_app: false, email: false, push: false }
};

export const load: PageServerLoad = async ({ cookies }) => {
	const token = cookies.get('jc_token');
	const headers: Record<string, string> = token ? { Authorization: `Bearer ${token}` } : {};

	const prefsResult = await apiFetch<PreferenceEntry[]>('/me/notification-preferences', { headers });

	let prefs: PrefsMap = { ...DEFAULT_PREFS };

	if (prefsResult.ok && prefsResult.data) {
		for (const entry of prefsResult.data) {
			if (NOTIFICATION_TYPES.includes(entry.notification_type)) {
				prefs[entry.notification_type] = {
					in_app: entry.in_app,
					email: entry.email,
					push: entry.push
				};
			}
		}
	}

	return { prefs };
};

export const actions: Actions = {
	default: async ({ request, cookies }) => {
		const token = cookies.get('jc_token');
		const headers: Record<string, string> = token ? { Authorization: `Bearer ${token}` } : {};

		const formData = await request.formData();
		let allOk = true;

		for (const nType of NOTIFICATION_TYPES) {
			const body = {
				in_app: formData.get(`${nType}_in_app`) === 'on',
				email: formData.get(`${nType}_email`) === 'on',
				push: formData.get(`${nType}_push`) === 'on'
			};

			const result = await apiFetch(`/me/notification-preferences/${nType}`, {
				method: 'PATCH',
				headers,
				body: JSON.stringify(body)
			});

			if (!result.ok) {
				allOk = false;
			}
		}

		if (!allOk) {
			return fail(500, { error: 'Failed to save some preferences' });
		}

		return { success: true };
	}
};
