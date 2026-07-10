import type { LayoutServerLoad } from './$types';
import { PUBLIC_JC_API_URL } from '$env/static/public';
import type { Enrollment } from '$lib/api/types';

export const load: LayoutServerLoad = async ({ locals, cookies }) => {
	const user = locals.user;
	const token = locals.token;
	const platformDisabled = locals.platformDisabled ?? false;
	const localeCode = cookies.get('jc_locale') ?? 'en';

	let translations: Record<string, string> = {};
	try {
		const res = await fetch(`${PUBLIC_JC_API_URL}/api/v1/i18n/packs/${localeCode}`);
		if (res.ok) {
			const body = await res.json();
			translations = body?.data ?? body ?? {};
		}
	} catch {
		// Graceful degradation
	}

	let enrollments: Enrollment[] = [];
	if (user && token) {
		try {
			const enrollRes = await fetch(`${PUBLIC_JC_API_URL}/api/v1/me/enrollments`, {
				headers: { Authorization: `Bearer ${token}` },
			});
			if (enrollRes.ok) {
				const { data } = await enrollRes.json();
				enrollments = data ?? [];
			}
		} catch {
			enrollments = [];
		}
	}

	const enrolledTrackIds = new Set(enrollments.map((e) => e.track_id));

	return {
		user,
		token,
		isAuth: !!user,
		platformDisabled,
		enrollments,
		enrolledTrackIds: Array.from(enrolledTrackIds),
		locale: localeCode,
		translations,
	};
};
