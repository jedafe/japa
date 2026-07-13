import { API_V1_BASE } from '$lib/server/api';
import { getCached } from '$lib/server/cache';

export async function load({ locals, cookies }: { locals: App.Locals; cookies: { get(name: string): string | undefined } }) {
	const token = cookies.get('jc_token');
	const localeCode = cookies.get('jc_locale') ?? 'en';

	const translations = await getCached<Record<string, string>>(`translations:${localeCode}`, 5 * 60 * 1000, async () => {
		try {
			const res = await fetch(`${API_V1_BASE}/i18n/packs/${localeCode}`);
			if (res.ok) {
				const body = await res.json();
				return body?.data ?? body ?? {};
			}
		} catch {
			// Graceful degradation
		}
		return {};
	});

	const raw = locals.user as Record<string, unknown> | null;

	return {
		token: token ?? null,
		user: raw ? {
			id: raw.id as string,
			email: raw.email as string,
			username: raw.username as string,
			display_name: raw.display_name as string,
			user_group_name: raw.user_group_name as string,
			avatar_url: (raw.avatar_url as string) ?? null,
			user_group_id: (raw.user_group_id as string) ?? '',
			theme_preference: ((raw.theme_preference as string) as 'light' | 'dark' | 'system') ?? 'dark',
			timezone: (raw.timezone as string) ?? 'UTC',
			is_verified: (raw.is_verified as boolean) ?? false,
			created_at: (raw.created_at as string) ?? '',
			permissions: (raw.permissions as string[]) ?? [],
		} : null,
		locale: localeCode,
		translations,
	};
}
