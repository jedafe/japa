import { apiGet } from './api';
import type { ApiContext } from './api';
import type { DocNavTree } from '$lib/types';

/**
 * Fetch the full nav tree (all audiences) server-side. The layout load filters
 * by accessibleAudiences (ADR-060 — server-side gating only). Degrades to an
 * empty list on failure so a missing nav never blanks the shell.
 */
export async function fetchDocNav(ctx: ApiContext): Promise<DocNavTree[]> {
	try {
		return await apiGet<DocNavTree[]>(ctx, '/docs/nav');
	} catch {
		return [];
	}
}
