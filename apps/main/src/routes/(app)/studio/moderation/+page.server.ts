// Ref: .jeos/specs/2026-09-23-studio-moderation-tab.md — Moderation tab.
// Scoped moderation queue: the backend resolves the caller's scope (platform
// node, moderated spaces/tags, assigned reports) and returns only visible
// items. Non-moderators get a 403.
import { redirect, error } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { apiContext, apiGet, ApiError } from '$lib/server/api';

export interface ScopedEntity {
	id: string;
	name: string;
	slug: string;
}

export interface ModerationScope {
	platform_moderator: boolean;
	moderated_spaces: ScopedEntity[];
	moderated_tags: ScopedEntity[];
	assigned_count: number;
	open_count: number;
}

export interface ModerationQueueItem {
	id: string;
	entity_type: string;
	entity_id: string;
	reason: string;
	notes: string | null;
	evidence_urls: string[] | null;
	status: string;
	reviewed_by: string | null;
	resolution_note: string | null;
	resolved_at: string | null;
	created_at: string;
	reporter_username: string | null;
	reporter_display_name: string | null;
	entity_title: string;
	entity_excerpt: string | null;
	entity_slug: string;
	entity_author_username: string | null;
	resolved_space_id: string | null;
	entity_space_name: string | null;
	entity_space_slug: string | null;
	scope_assigned: boolean;
	scope_space: boolean;
	scope_tag: boolean;
}

interface QueuePage {
	items: ModerationQueueItem[];
	page: number;
	total: number;
	total_pages: number;
}

export const load: PageServerLoad = async (event) => {
	if (!event.locals.user) {
		throw redirect(
			307,
			`/login?redirect=${encodeURIComponent(event.url.pathname + event.url.search)}`
		);
	}

	const ctx = apiContext(event);
	const params = event.url.searchParams;

	let scope: ModerationScope | null = null;
	try {
		scope = await apiGet<ModerationScope>(ctx, '/me/moderation/scope');
	} catch (e) {
		if (e instanceof ApiError && e.status === 403) throw error(403, { code: 'forbidden', message: 'No moderation access' });
		scope = null;
	}

	const active =
		!!scope &&
		(scope.platform_moderator ||
			(scope.moderated_spaces ?? []).length > 0 ||
			(scope.moderated_tags ?? []).length > 0 ||
			scope.assigned_count > 0);
	if (!active) {
		throw error(403, { code: 'forbidden', message: 'No moderation access' });
	}

	const qs = new URLSearchParams();
	for (const k of ['status', 'entity_type', 'space_id', 'tag_id', 'assigned', 'q', 'page'] as const) {
		const v = params.get(k);
		if (v) qs.set(k, v);
	}

	let queue: QueuePage = { items: [], page: 0, total: 0, total_pages: 0 };
	try {
		const res = await apiGet<QueuePage>(ctx, `/me/moderation/queue?${qs.toString()}`);
		if (res) queue = { ...res, items: res.items ?? [] };
	} catch {
		// Degrade to an empty queue rather than 500ing the tab when the
		// endpoint is unhealthy.
	}

	// Rule 16 — status/entity-type filter options come from the backend
	// catalogs (auth-only endpoints, so moderators can read them).
	const [statusOptions, typeOptions] = await Promise.all([
		apiGet<{ value: string; label: string }[]>(ctx, '/admin/catalogs/reports.status').then((r) => r ?? []).catch(() => []),
		apiGet<{ value: string; label: string }[]>(ctx, '/admin/catalogs/reports.entity_type').then((r) => r ?? []).catch(() => [])
	]);

	return { scope, queue, statusOptions, typeOptions };
};
