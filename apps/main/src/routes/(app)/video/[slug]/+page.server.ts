import { error, isHttpError, isRedirect, redirect } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { loadContentArticle } from '$lib/server/content-article';
import { apiContext, apiGet, ApiError } from '$lib/server/api';
import { contentHref } from '$lib/utils/contentLink';
import type { VideoSource } from '$lib/types';

/**
 * Canonical video single page (§106).
 *
 * Two resolutions, in order:
 * 1. Content slug — a published `content_type='video'` post renders the full
 *    article page (player, byline, body, engagement, comments). Non-video
 *    content hitting this route is 301'd to its own canonical family so every
 *    content item keeps exactly one canonical URL (ADR-058).
 * 2. video_sources slug — /sources/{channel} item cards for channel videos
 *    that were never published as posts still open in-app on a lean player
 *    page instead of bouncing to the external platform URL. A UUID param
 *    still resolves, then 301s to the canonical slug.
 */
export const load: PageServerLoad = async (event) => {
	const ctx = apiContext(event);

	try {
		const data = await loadContentArticle(event);

		if (data.article.content_type !== 'video') {
			throw redirect(301, `${contentHref(data.article)}${event.url.search}`);
		}

		// The stored body only carries an empty .video-embed placeholder —
		// resolve the linked video_sources row for the player (best-effort).
		let videoSource: VideoSource | null = null;
		if (data.article.video_source_id) {
			try {
				videoSource = await apiGet<VideoSource>(ctx, `/videos/${data.article.video_source_id}`);
			} catch {
				videoSource = null;
			}
		}

		return { ...data, videoSource, sourceOnly: false as const, canSummarize: false };
	} catch (e) {
		if (isRedirect(e)) {
			throw e;
		}
		if (!isHttpError(e) || e.status !== 404) {
			throw e;
		}
		// Fall through — try resolving the param as a video_sources row
		// (the backend accepts both the public slug and the UUID).
	}

	const id = event.params.slug!;

	// §106: POST /videos/{id}/summarize enqueues an AI draft — exposed to
	// video.create holders only when video.ai_summary.enabled is on. The
	// permission lookup doesn't depend on the video fetch — same wave.
	const siteSettings =
		((await event.parent()) as { siteSettings?: Record<string, unknown> }).siteSettings ?? {};
	const canSummarizePromise =
		event.locals.user && siteSettings['video.ai_summary.enabled'] === true
			? apiGet<{ permissions: string[] }>(ctx, '/me/permissions')
					.then((perms) => (perms?.permissions ?? []).includes('video.create'))
					.catch(() => false)
			: Promise.resolve(false);

	try {
		const videoSource = await apiGet<VideoSource>(ctx, `/videos/${id}`);
		// Canonicalize — a UUID param that resolves to a slugged row redirects
		// to /video/{slug} so the video never answers at two addresses.
		if (videoSource.slug && videoSource.slug !== id) {
			throw redirect(301, `/video/${videoSource.slug}${event.url.search}`);
		}

		const canSummarize = await canSummarizePromise;

		return { videoSource, sourceOnly: true as const, canSummarize, currentUserId: event.locals.user?.id ?? null };
	} catch (err) {
		if (isRedirect(err)) {
			throw err;
		}
		if (err instanceof ApiError && err.status === 404) {
			throw error(404, { code: 'not_found', message: 'Video not found' });
		}
		throw error(502, { code: 'upstream_error', message: 'Failed to load video' });
	}
};
