import type { LayoutServerLoad } from './$types';

export const load: LayoutServerLoad = async ({ locals, cookies }) => {
	const token = cookies.get('learn_token') ?? cookies.get('jc_token');
	const localeCode = cookies.get('jc_locale') ?? 'en';

	let translations: Record<string, string> = {};
	try {
		translations = await import(`$lib/locales/${localeCode}.json`).then((m) => m.default);
	} catch {
		translations = await import('$lib/locales/en.json').then((m) => m.default);
	}

	return {
		user: locals.user,
		token: token ?? null,
		locale: localeCode,
		translations
	};
};
