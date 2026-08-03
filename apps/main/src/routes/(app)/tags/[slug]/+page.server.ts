import { error } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { apiContext, apiGet, apiList } from '$lib/server/api';
import type { TagDetail } from '$lib/types';
import type { ContentArticle } from '$lib/types/content';

/**
 * Tag hub at /tags/[slug] — SSR, indexable. Center column shows the tag
 * header (name, description, follower count, follow button) and a grid of
 * tagged content. The right sidebar shows the tag card with follow action.
 *
 * Ref: japaconnect-engineering/sections/08-tags.md
 *      ADR-058 (renders inside the app shell; not a documented tab surface)
 */
const PAGE_SIZE = 12;

interface TagFieldDef {
	key: string;
	label: string;
	field_type: string;
	placeholder?: string;
	default_value?: string;
	is_required: boolean;
}

export const load: PageServerLoad = async (event) => {
	const ctx = apiContext(event);
	const slug = event.params.slug;

	// Tag detail — 404 propagates as a SvelteKit error.
	let tag: TagDetail;
	try {
		tag = await apiGet<TagDetail>(ctx, `/tags/${slug}`);
	} catch (e) {
		const status = e instanceof Error && 'status' in e ? (e as { status: number }).status : 500;
		throw error(status === 404 ? 404 : 500, {
			code: status === 404 ? 'not_found' : 'server_error',
			message: status === 404 ? 'Tag not found' : 'Failed to load tag'
		});
	}

	// Tagged content — degrades to an empty list on failure so a missing feed
	// never blanks the whole tag view.
	let items: ContentArticle[] = [];
	let loadError = false;
	try {
		items = (
			await apiList<ContentArticle>(ctx, `/tags/${slug}/content?page_size=${PAGE_SIZE}`)
		).items;
	} catch {
		loadError = true;
	}

	// Fetch tag custom field definitions so the page can render field labels
	// alongside the stored field_values. Best-effort: degrades to empty list.
	// Deduplicate by key — multiple groups may define fields with the same key,
	// which would break Svelte's keyed each block.
	let fieldDefs: TagFieldDef[] = [];
	try {
		const groups = await apiList<{ fields?: TagFieldDef[] }>(ctx, '/custom-field-groups?entity_class=tag');
		const seen = new Set<string>();
		for (const g of groups.items) {
			if (g.fields) {
				for (const f of g.fields) {
					if (!seen.has(f.key)) {
						seen.add(f.key);
						fieldDefs.push(f);
					}
				}
			}
		}
	} catch {
		fieldDefs = [];
	}

	// Hydrate the follow button's initial state. Best-effort: a failure
	// degrades to `false` (not following). Only fetch when the viewer is
	// authenticated — guests can't follow.
	const initiallyFollowing = event.locals.user
		? apiGet<{ following: boolean }>(ctx, `/users/${tag.id}/follow-status?entity_type=tag`)
				.then((r) => r.following)
				.catch(() => false)
		: Promise.resolve(false);

	// Check if the viewer can manage tags (tags.manage permission or super_admin
	// group membership — super_admin bypasses all permission checks at the RBAC
	// middleware level, but /me/permissions only returns explicitly resolved
	// nodes, so the group check is needed to surface the edit button for them).
	// Best-effort: degrades to false so a failing lookup never blocks the page.
	let canManage = false;
	if (event.locals.user) {
		const isSuperAdmin = (event.locals.user.groups ?? []).some(
			(g) => g === 'Super Admin' || g === 'super_admin'
		);
		if (isSuperAdmin) {
			canManage = true;
		} else {
			try {
				const permRes = await apiGet<{ permissions: string[] }>(ctx, '/me/permissions');
				canManage = (permRes?.permissions ?? []).includes('tags.manage');
			} catch {
				canManage = false;
			}
		}
	}

	const isFollowing = await initiallyFollowing;

	return {
		tag,
		items,
		pageSize: PAGE_SIZE,
		loadError,
		fieldDefs,
		canManage,
		initiallyFollowing: isFollowing
	};
};
