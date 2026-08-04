import type { PageServerLoad, Actions } from './$types';
import { apiContext, apiList, apiPost, ApiError } from '$lib/server/api';
import { redirect } from '@sveltejs/kit';
import type { ContentImport, ImportSource } from '$lib/types';

/**
 * Content Import page at /me/import (Section 65, ADR-083).
 * Auth-gated page where the user starts a content import (Medium, Substack,
 * RSS, ZIP) and views their import history with status.
 *
 * Ref: japaconnect-engineering/sections/65-content-import.md
 */
export const load: PageServerLoad = async (event) => {
	if (!event.locals.user) {
		throw redirect(307, '/login?redirect=/me/import');
	}

	const ctx = apiContext(event);

	let imports: ContentImport[] = [];
	try {
		imports = (await apiList<ContentImport>(ctx, '/me/imports')).items;
	} catch {
		imports = [];
	}

	// Fetch the allowed source types from the backend so the frontend never
	// hardcodes the enum (Rule 16). Degrades to an empty list on failure.
	let sources: ImportSource[] = [];
	try {
		sources = (await apiList<ImportSource>(ctx, '/me/imports/sources')).items;
	} catch {
		sources = [];
	}

	return { imports, sources };
};

export const actions: Actions = {
	start: async (event) => {
		if (!event.locals.user) {
			return { error: 'auth_required' };
		}

		const ctx = apiContext(event);
		const form = await event.request.formData();
		const sourceType = (form.get('source_type') as string | null)?.trim() ?? '';
		const sourceUrl = (form.get('source_url') as string | null)?.trim() ?? '';

		if (!sourceType) {
			return { error: 'missing_source_type' };
		}

		try {
			const result = await apiPost<ContentImport>(ctx, '/me/imports', {
				source_type: sourceType,
				source_url: sourceUrl || undefined
			});
			return { ok: true, import: result };
		} catch (err) {
			if (err instanceof ApiError) {
				return { error: err.code, message: err.message };
			}
			return { error: 'import_failed' };
		}
	}
};
