// Ref: .jeos/specs/2026-09-18-studio-web-prompts-tab.md — Prompts tab.
// Loads the option lists the builder and import flow need (all existing
// public/member routes — Rule 16) and saves generated output as a contents
// draft via POST/PATCH /content.
import { redirect } from '@sveltejs/kit';
import type { PageServerLoad, Actions } from './$types';
import { apiContext, apiGet, apiList, apiPost, apiPatch, apiDelete, ApiError } from '$lib/server/api';
import type { ContentTypeMeta, MeSpace, ContentRecord, ContentTag } from '$lib/types';
import type { Persona } from '$lib/studio/types';
import { validateFrontmatter, validateBody, isHighStakesCategory } from '$lib/studio/validation';
import { markdownToHtml } from '$lib/studio/markdown-html';

const STUDIO_PROMPTS_TAG = 'app:studio-prompts';

interface VoiceOption {
	id: string;
	name: string;
	voice_prompt?: string;
}

export const load: PageServerLoad = async (event) => {
	if (!event.locals.user) {
		throw redirect(307, '/login?redirect=/studio/prompts');
	}

	event.depends(STUDIO_PROMPTS_TAG);
	const ctx = apiContext(event);

	const [typesRes, spacesRes, draftsRes, permsRes] = await Promise.allSettled([
		apiList<ContentTypeMeta>(ctx, '/content-types'),
		apiList<MeSpace>(ctx, '/me/spaces'),
		// Advisory list for the slug-conflict check — the backend's slug
		// handling is authoritative on save.
		apiList<ContentRecord>(ctx, '/me/drafts?page_size=100'),
		apiGet<{ permissions: string[] }>(ctx, '/me/permissions')
	]);

	const canPersona =
		permsRes.status === 'fulfilled' &&
		(permsRes.value?.permissions ?? []).includes('persona.view');

	let voices: VoiceOption[] = [];
	let personas: Persona[] = [];
	if (canPersona) {
		// Operator selectors (§105): voices from the shared non-admin route
		// (same one AiDraftPanel uses); personas only exist behind the admin
		// route, which persona.view holders are allowed to read.
		const [voicesRes, personasRes] = await Promise.allSettled([
			apiList<VoiceOption>(ctx, '/ai/voices?page_size=5000'),
			apiList<Persona>(ctx, '/admin/personas?page_size=200')
		]);
		voices = voicesRes.status === 'fulfilled' ? voicesRes.value.items : [];
		personas = personasRes.status === 'fulfilled' ? personasRes.value.items : [];
	}

	return {
		contentTypes: typesRes.status === 'fulfilled' ? typesRes.value.items : [],
		spaces: spacesRes.status === 'fulfilled' ? spacesRes.value.items : [],
		drafts: draftsRes.status === 'fulfilled' ? draftsRes.value.items : [],
		canPersona,
		voices,
		personas
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
	if (/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}$/.test(value)) return `${value}:00Z`;
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
		body_html: markdownToHtml(body),
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
		let status = getStr(fm, 'status') || 'draft';
		const guideCategory = getStr(fm, 'guide_category');
		if (
			guideCategory &&
			isHighStakesCategory(guideCategory) &&
			(status === 'published' || status === 'scheduled')
		) {
			status = 'in_review';
		}

		// Always validate in member mode — the tab never reassigns authorship,
		// so persona_id is never a requirement here.
		const fieldErrors = validateFrontmatter(fm, status, 'member');
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

		try {
			let content: ContentRecord;
			if (contentId) {
				content = await apiPatch<ContentRecord>(ctx, `/content/${contentId}`, payload);
				// Tag diff: attach new slugs, detach removed ones.
				const existing = await apiList<ContentTag>(ctx, `/content/${contentId}/tags`).catch(
					() => ({ items: [] as ContentTag[] })
				);
				const existingSlugs = new Set(
					existing.items.map((t) => t.slug ?? t.name ?? '').filter(Boolean)
				);
				for (const slug of tags) {
					if (!existingSlugs.has(slug)) {
						await apiPost(ctx, `/content/${contentId}/tags`, { tag_slugs: [slug] });
					}
				}
				for (const slug of existingSlugs) {
					if (!tags.includes(slug)) {
						await apiDelete(ctx, `/content/${contentId}/tags/${encodeURIComponent(slug)}`).catch(
							() => ({})
						);
					}
				}
			} else {
				content = await apiPost<ContentRecord>(ctx, '/content', payload);
				if (tags.length > 0) {
					await apiPost(ctx, `/content/${content.id}/tags`, { tag_slugs: tags });
				}
			}
			return { ok: true, content };
		} catch (err) {
			if (err instanceof ApiError) {
				return { error: err.code, message: err.message };
			}
			return { error: 'save_failed' };
		}
	}
};
