import { env } from '$env/dynamic/private';
import type { PageServerLoad } from './$types';

const API_BASE = env.PRIVATE_API_BASE_URL ?? 'http://localhost:8080';

export const load: PageServerLoad = async ({ locals }) => {
	const token = (locals as any)?.token;
	const headers: Record<string, string> = token ? { Authorization: `Bearer ${token}` } : {};

	try {
		const [languagesRes, stringsRes] = await Promise.all([
			fetch(`${API_BASE}/api/v1/admin/i18n/languages`, { headers }),
			fetch(`${API_BASE}/api/v1/admin/i18n/strings?language_code=en`, { headers })
		]);

		const languages = languagesRes.ok ? (await languagesRes.json()).data : null;
		const strings = stringsRes.ok ? (await stringsRes.json()).data : null;

		return { languages, strings };
	} catch {
		return { languages: null, strings: null, error: 'Failed to load' };
	}
};
