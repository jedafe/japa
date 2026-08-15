import { error, redirect } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { apiContext } from '$lib/server/api';
import { resolveSection } from '$lib/components/hub/settings/settingsSections';
import { SETTINGS_LOADERS } from '$lib/components/hub/settings/settingsLoaders.server';

/**
 * Dispatch to the section's server loader. 404s on unknown slugs, sections
 * flagged backendReady:false (e.g. `devices`), sections whose component
 * is not yet built (Plan 2/3), and sections whose feature flag is disabled
 * — the rail shows those disabled.
 *
 * Ref: japaconnect-engineering/sections/23-main-site-frontend.md (Settings hub)
 */
export const load: PageServerLoad = async (event) => {
	// /settings is auth-gated — a guest has no profile/privacy to edit.
	if (!event.locals.user) {
		throw redirect(307, `/login?redirect=${encodeURIComponent(event.url.pathname + event.url.search)}`);
	}

	const meta = resolveSection(event.params.section);
	if (!meta || !meta.backendReady || !meta.component) {
		throw error(404, { code: 'not_found', message: 'Unknown settings section' });
	}

	// Feature flag gate: hide the section entirely when its flag is disabled.
	if (meta.featureFlag) {
		const { features } = await event.parent();
		if (features[meta.featureFlag] !== true) {
			throw error(404, { code: 'feature_disabled', message: 'This section is not available' });
		}
	}

	const loader = SETTINGS_LOADERS[meta.slug];
	const sectionData = loader ? await loader(apiContext(event)) : null;
	return { section: meta.slug, sectionData };
};
