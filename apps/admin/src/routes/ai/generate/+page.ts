import type { PageLoad } from './$types';
import { api } from '$lib/api';

/**
 * Admin AI Generate page load — fetch feature flags so the UI can hide the
 * generation surface when feature.ai_content_gen is disabled (BUG-015).
 *
 * Ref: sections/42-feature-flags.md, internal/server/server.go:4349.
 */
export const load: PageLoad = async () => {
	const res = await api<Record<string, boolean>>('/feature-flags');
	const enabled = res.ok && res.data['feature.ai_content_gen'] === true;
	return { enabled };
};
