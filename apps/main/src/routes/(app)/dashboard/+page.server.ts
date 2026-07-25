import { redirect } from '@sveltejs/kit';
import { DEFAULT_VIEW } from '$lib/components/hub/dashboard/dashboardViews';

// /dashboard redirects to the default view. SvelteKit 2: redirect() throws.
export const load = () => {
	throw redirect(307, `/dashboard/${DEFAULT_VIEW}`);
};
