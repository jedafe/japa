import type { PageServerLoad, Actions } from './$types';
import { apiContext, apiList, apiPost, ApiError } from '$lib/server/api';
import { redirect } from '@sveltejs/kit';
import type { VideoImportJob } from '$lib/types';

const STUDIO_CACHE_TAG = 'app:studio';

/**
 * Video Studio page at /studio/videos (Section 106, ADR-117).
 * Auth-gated page where users start bulk YouTube channel/playlist/keyword
 * imports and monitor their progress.
 */
export const load: PageServerLoad = async (event) => {
	if (!event.locals.user) {
		throw redirect(307, '/login?redirect=/studio/videos');
	}

	// The video.enabled gate covers this subroute only — the rest of the
	// /studio surface is available regardless (Rule 16).
	const parent = await event.parent();
	const siteSettings = (parent as { siteSettings?: Record<string, unknown> }).siteSettings ?? {};
	if (siteSettings['video.enabled'] !== true) {
		throw redirect(307, '/studio/create');
	}

	event.depends(STUDIO_CACHE_TAG);
	const ctx = apiContext(event);

	let jobs: VideoImportJob[] = [];
	try {
		jobs = (await apiList<VideoImportJob>(ctx, '/me/video-imports')).items;
	} catch {
		jobs = [];
	}

	return { jobs };
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
		const query = (form.get('query') as string | null)?.trim() ?? '';
		const targetUsername = (form.get('target_username') as string | null)?.trim() || undefined;
		const autoPublish = form.get('auto_publish') === 'on';

		if (!sourceType) {
			return { error: 'missing_source_type' };
		}

		const body: Record<string, unknown> = {
			source_type: sourceType,
			auto_publish: autoPublish
		};
		if (sourceType === 'keyword') {
			body.query = query || undefined;
		} else {
			body.source_url = sourceUrl || undefined;
		}
		if (targetUsername) {
			body.target_username = targetUsername;
		}

		try {
			const result = await apiPost<VideoImportJob>(ctx, '/me/video-imports', body);
			return { ok: true, job: result };
		} catch (err) {
			if (err instanceof ApiError) {
				return { error: err.code, message: err.message };
			}
			return { error: 'import_failed' };
		}
	}
};
