// Ref: .jeos/specs/2026-09-18-studio-web-prompts-tab.md — Prompts tab.
// Loads the option lists the builder and import flow need (all existing
// public/member routes — Rule 16) and saves generated output as a contents
// draft via POST/PATCH /content.
import { redirect } from '@sveltejs/kit';
import type { PageServerLoad, Actions } from './$types';
import { apiContext, apiGet, apiList, apiPost, apiPatch, apiDelete, ApiError, type ApiContext } from '#lib/server/api.js';
import type { ContentTypeMeta, MeSpace, ContentRecord, ContentTag, Visibility, BlogSeries, AssessmentForm } from '#lib/types/index.js';
import type { Persona } from '#lib/studio/types.js';
import { validateFrontmatter, validateBody, isHighStakesCategory } from '#lib/studio/validation.js';
import { markdownToHtml } from '#lib/studio/markdown-html.js';

const STUDIO_PROMPTS_TAG = 'app:studio-prompts';

interface VoiceOption {
	id: string;
	name: string;
	voice_prompt?: string;
}

/**
 * Fetch every page of a paginated list endpoint. Some handlers reject a
 * page_size above their cap by falling back to the default page size
 * (rather than clamping), so callers must not guess the cap — walk
 * page_offset against meta.total instead. Bounded so a pathological
 * backend cannot loop forever.
 */
async function apiListAll<T>(ctx: ApiContext, basePath: string): Promise<T[]> {
	const out: T[] = [];
	for (let page = 0; page < 10; page++) {
		const sep = basePath.includes('?') ? '&' : '?';
		const { items, meta } = await apiList<T>(
			ctx,
			`${basePath}${sep}page_size=100&page_offset=${out.length}`
		);
		out.push(...items);
		if (meta?.total === undefined || out.length >= meta.total || items.length === 0) break;
	}
	return out;
}

export const load: PageServerLoad = async (event) => {
	if (!event.locals.user) {
		throw redirect(
			307,
			`/login?redirect=${encodeURIComponent(event.url.pathname + event.url.search)}`
		);
	}

	event.depends(STUDIO_PROMPTS_TAG);
	const ctx = apiContext(event);

	const [typesRes, spacesRes, draftsRes, permsRes, visibilityRes, statusRes, seriesRes, assessmentRes] =
		await Promise.allSettled([
			apiList<ContentTypeMeta>(ctx, '/content-types'),
			// member_only cap — SpaceSelector live-searches via /me/spaces?q=
			// anyway; the prop only seeds initial suggestions. Uncapped this
			// returns every platform space (~10k rows / 1.7MB for admins).
			apiList<MeSpace>(ctx, '/me/spaces?member_only=1&page_size=50'),
			// Advisory list for the slug-conflict check — the backend's slug
			// handling is authoritative on save.
			apiList<ContentRecord>(ctx, '/me/drafts?page_size=100'),
			apiGet<{ permissions: string[] }>(ctx, '/me/permissions'),
			apiGet<{ value: Visibility; label: string }[]>(ctx, '/content/visibility-options'),
			// Rule 16 — the status list comes from the backend, not a
			// hardcoded constant (includes 'archived').
			apiGet<{ value: string; label: string }[]>(ctx, '/content/status-options'),
			apiList<BlogSeries>(ctx, '/series'),
			// §71 — active assessment form; its dimensions populate the
			// Journey panel's roadmap-dimension picker.
			apiGet<AssessmentForm>(ctx, '/assessment')
		]);

	// A failed lookup previously degraded to an empty select with no signal;
	// collect the labels so the page can warn the user that option lists are
	// incomplete instead of looking deliberately empty.
	const lookupWarnings: string[] = [];
	const settled = <T,>(res: PromiseSettledResult<T>, label: string): T | undefined => {
		if (res.status === 'fulfilled') return res.value;
		lookupWarnings.push(label);
		return undefined;
	};

	const canPersona =
		permsRes.status === 'fulfilled' &&
		(permsRes.value?.permissions ?? []).includes('persona.view');
	if (permsRes.status !== 'fulfilled') lookupWarnings.push('permissions');

	let voices: VoiceOption[] = [];
	let personas: Persona[] = [];
	if (canPersona) {
		// Operator selectors (§105): voices via the persona.view-gated admin
		// route — /ai/voices is gated by the ai.* nodes, so a persona.view
		// operator without AI access would get a silent 403. ?slim=1 is the
		// {id, name} picker projection; full rows carry voice_prompt (~1.7KB
		// each → ~2.6MB). PromptBuilder lazy-fetches the selected voice's
		// prompt via GET /admin/persona-voices/{id} (same gate).
		// Personas paginate through apiListAll — a page_size above the
		// handler's cap is rejected (falls back to the default 20), so the
		// request must not out-request the cap.
		const [voicesRes, personasRes] = await Promise.allSettled([
			apiList<VoiceOption>(ctx, '/admin/persona-voices?slim=1&page_size=5000'),
			apiListAll<Persona>(ctx, '/admin/personas')
		]);
		// Strip stays as a cheap guard — only {id, name} lands in the page.
		voices = (settled(voicesRes, 'voices')?.items ?? []).map((v) => ({
			id: v.id,
			name: v.name
		}));
		personas = settled(personasRes, 'personas') ?? [];
	}

	return {
		contentTypes: settled(typesRes, 'content types')?.items ?? [],
		spaces: settled(spacesRes, 'spaces')?.items ?? [],
		drafts: settled(draftsRes, 'drafts')?.items ?? [],
		canPersona,
		voices,
		personas,
		visibilityOptions: settled(visibilityRes, 'visibility options') ?? [],
		statusOptions: settled(statusRes, 'status options') ?? [],
		series: settled(seriesRes, 'series')?.items ?? [],
		assessmentDimensions: settled(assessmentRes, 'journey dimensions')?.dimensions ?? [],
		lookupWarnings
	};
};

function getStr(obj: Record<string, unknown>, key: string): string {
	const v = obj[key];
	return typeof v === 'string' ? v.trim() : '';
}

function getTags(payload: Record<string, unknown>): string[] {
	const v = payload.tags;
	if (Array.isArray(v)) return v.map((t) => String(t).trim()).filter(Boolean);
	if (typeof v === 'string') return v.split(',').map((t) => t.trim()).filter(Boolean);
	return [];
}

function normalisePublishAt(value: string): string | undefined {
	if (!value) return undefined;
	// A bare `YYYY-MM-DDTHH:mm` comes from a datetime-local input — it is
	// browser-local time, so convert to a real UTC instant instead of
	// naively tagging it 'Z' (which would store local wall-time as UTC).
	if (/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}$/.test(value)) {
		const d = new Date(value);
		return isNaN(d.getTime()) ? undefined : d.toISOString();
	}
	return value;
}

/** Build the POST/PATCH /content payload from parsed frontmatter + body. */
function buildApiPayload(fm: Record<string, unknown>, status: string): Record<string, unknown> {
	const body = getStr(fm, 'body');
	const payload: Record<string, unknown> = {
		content_type: getStr(fm, 'content_type') || 'guide',
		title: getStr(fm, 'title') || 'Untitled',
		slug: getStr(fm, 'slug'),
		body,
		// Spec §5 — the body_html pipeline is marked → isomorphic-dompurify
		// (the backend re-sanitizes on save as well, but the payload should
		// arrive already clean).
		body_html: markdownToHtml(body, { sanitize: true }),
		status,
		visibility: getStr(fm, 'visibility') || 'public',
		seo_indexable: fm.seo_indexable !== false
	};

	const optional = [
		'space_id',
		'guide_category',
		'journey_stage',
		'visa_route',
		'source_country',
		'target_country',
		'meta_title',
		'meta_description',
		'og_image_url',
		'canonical_url',
		'header_image_id',
		'featured_image_id'
	] as const;
	for (const key of optional) {
		const v = getStr(fm, key) || getStr((fm.field_values as Record<string, unknown>) ?? {}, key);
		if (v) payload[key] = v;
	}

	const publishAt = normalisePublishAt(getStr(fm, 'publish_at'));
	if (publishAt) payload.publish_at = publishAt;

	const fieldValues = fm.field_values;
	if (fieldValues && typeof fieldValues === 'object' && !Array.isArray(fieldValues)) {
		payload.field_values = fieldValues;
	}

	return payload;
}

export const actions: Actions = {
	save: async (event) => {
		if (!event.locals.user) {
			return { error: 'auth_required' };
		}

		const ctx = apiContext(event);
		const form = await event.request.formData();

		let fm: Record<string, unknown>;
		try {
			fm = JSON.parse(String(form.get('payload') ?? '{}')) as Record<string, unknown>;
		} catch {
			return { error: 'invalid_payload' };
		}

		// Server-side enforcement of the same rules the client validated:
		// high-stakes categories can never go straight to published/scheduled.
		// Actors holding agent.skip_hitl bypass the hold (§103) — the right
		// panel already offers them published/scheduled, so the action must
		// honour the same permission instead of silently downgrading.
		const perms = event.locals.user?.permissions ?? [];
		const canSkipHitl = perms.includes('*') || perms.includes('agent.skip_hitl');
		let status = getStr(fm, 'status') || 'draft';
		const guideCategory = getStr(fm, 'guide_category');
		if (
			!canSkipHitl &&
			guideCategory &&
			isHighStakesCategory(guideCategory) &&
			(status === 'published' || status === 'scheduled')
		) {
			status = 'in_review';
		}

		// Always validate in member mode — the tab never reassigns authorship,
		// so persona_id is never a requirement here. Status/visibility are
		// validated against the live catalogs (Rule 16); validateFrontmatter
		// falls back to its defaults when a catalog is unreachable.
		const [statusOpts, visOpts] = await Promise.all([
			apiGet<{ value: string; label: string }[]>(ctx, '/content/status-options').catch(
				() => [] as { value: string; label: string }[]
			),
			apiGet<{ value: string; label: string }[]>(ctx, '/content/visibility-options').catch(
				() => [] as { value: string; label: string }[]
			)
		]);
		const fieldErrors = validateFrontmatter(
			fm,
			status,
			'member',
			statusOpts.map((o) => o.value),
			visOpts.map((o) => o.value),
			canSkipHitl
		);
		const bodyError = validateBody(getStr(fm, 'body'), status);
		if (Object.keys(fieldErrors).length > 0 || bodyError) {
			return {
				error: 'validation_failed',
				fields: fieldErrors,
				message: bodyError
			};
		}

		const contentId = getStr(fm, 'content_id');
		const tags = getTags(fm);
		const payload = buildApiPayload(fm, status);

		// Post-save linkage failures must not throw — the content row already
		// exists, so an error result would invite a retry that duplicates it.
		// Collect them as warnings the UI surfaces instead.
		const warnings: string[] = [];
		try {
			let content: ContentRecord;
			if (contentId) {
				content = await apiPatch<ContentRecord>(ctx, `/content/${contentId}`, payload);
				// Tag diff: attach new slugs, detach removed ones. A failed tag
				// lookup degrades to attach-only (the backend upsert is
				// idempotent) rather than a silent no-op.
				const existing = await apiList<ContentTag>(ctx, `/content/${contentId}/tags`).catch(
					() => {
						warnings.push('tag_lookup_failed');
						return { items: [] as ContentTag[] };
					}
				);
				const existingSlugs = new Set(
					existing.items.map((t) => t.slug ?? t.name ?? '').filter(Boolean)
				);
				for (const slug of tags) {
					if (!existingSlugs.has(slug)) {
						await apiPost(ctx, `/content/${contentId}/tags`, { tag_slugs: [slug] }).catch(
							() => warnings.push('tag_sync_failed')
						);
					}
				}
				for (const slug of existingSlugs) {
					if (!tags.includes(slug)) {
						await apiDelete(ctx, `/content/${contentId}/tags/${encodeURIComponent(slug)}`).catch(
							() => warnings.push('tag_sync_failed')
						);
					}
				}
			} else {
				content = await apiPost<ContentRecord>(ctx, '/content', payload);
				if (tags.length > 0) {
					await apiPost(ctx, `/content/${content.id}/tags`, { tag_slugs: tags }).catch(
						() => warnings.push('tag_sync_failed')
					);
				}
			}

			// Series linkage is a dedicated endpoint, not a /content field —
			// same contract the composer uses (§14).
			const seriesId = getStr(fm, 'series_id');
			if (seriesId && content.id) {
				await apiPost(ctx, `/series/${seriesId}/posts`, {
					content_id: content.id,
					position: 0
				}).catch(() => warnings.push('series_link_failed'));
			}

			return { ok: true, content, warnings };
		} catch (err) {
			if (err instanceof ApiError) {
				return { error: err.code, message: err.message };
			}
			return { error: 'save_failed' };
		}
	}
};
