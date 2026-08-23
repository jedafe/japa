import type { LayoutServerLoad } from './$types';

export const load: LayoutServerLoad = async ({ locals }) => {
	return {
		user: locals.user,
		theme: locals.theme,
		locale: locals.locale,
		csrfToken: locals.csrfToken,
		siteSettings: locals.siteSettings,
		features: locals.features
	};
};
