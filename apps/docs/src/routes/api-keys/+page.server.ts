import { redirect } from '@sveltejs/kit';
import { PUBLIC_JC_SITE_URL } from '$env/static/public';
import type { PageServerLoad } from './$types';
import { apiFetch } from '$lib/api/client';
import type { ApiKey } from '$lib/api/types';

export const load: PageServerLoad = async ({ locals, url }) => {
	const user = locals.user as { id: string } | null;
	if (!user) {
		// The docs app has no login UI of its own — authentication lives on
		// the main app, which shares the session cookie across subdomains.
		throw redirect(302, `${PUBLIC_JC_SITE_URL}/login?redirectTo=${encodeURIComponent(url.href)}`);
	}

	let keys: ApiKey[] = [];
	const result = await apiFetch<ApiKey[]>('/me/api-keys');
	if (result.data) {
		keys = result.data;
	}

	return { keys };
};
