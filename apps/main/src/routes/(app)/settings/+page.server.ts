import { redirect } from '@sveltejs/kit';
import { DEFAULT_SECTION } from '$lib/components/hub/settings/settingsSections';

// /settings redirects to the default section. SvelteKit 2: redirect() throws —
// do not wrap in try/catch (project memory: SvelteKit2 redirect traps).
export const load = () => {
	throw redirect(307, `/settings/${DEFAULT_SECTION}`);
};
