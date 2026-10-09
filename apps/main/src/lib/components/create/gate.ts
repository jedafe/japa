import type { ComposerStatus } from '$lib/types';
import type { FormKind } from './contentTypeRegistry';

export interface PublishInput {
	title: string;
	primarySpaceId: string | null;
	requiredFieldsFilled: boolean;
	formKind: FormKind;
	itemCount: number;
	dimensionCount: number;
}

export function evaluatePublish(i: PublishInput): { ok: boolean; reason: string | null } {
	if (i.title.trim().length === 0) return { ok: false, reason: 'create.gate.title' };
	if (!i.primarySpaceId) return { ok: false, reason: 'create.gate.space' };
	if (i.formKind === 'comparison') {
		if (i.itemCount < 2) return { ok: false, reason: 'create.gate.items' };
		if (i.dimensionCount < 1) return { ok: false, reason: 'create.gate.dimensions' };
	}
	if (i.formKind === 'list') {
		if (i.itemCount < 1) return { ok: false, reason: 'create.gate.list_items' };
	}
	if (!i.requiredFieldsFilled) return { ok: false, reason: 'create.gate.fields' };
	return { ok: true, reason: null };
}

const STATUS_KEYS: Record<ComposerStatus, string> = {
	pristine: 'create.status.draft',
	creating: 'create.status.saving',
	saved: 'create.status.saved',
	dirty: 'create.status.unsaved',
	saving: 'create.status.saving',
	published: 'create.status.published',
	scheduled: 'create.status.scheduled',
	error: 'create.status.error'
};

export function statusLabel(status: ComposerStatus): string {
	return STATUS_KEYS[status];
}

export function isExceptionRoute(routeId: string | null): boolean {
	// ADR-058 Exception #1: blog single & CPT singles collapse the left nav.
	// /story/[slug] and /comparisons/[slug] are CPT singles (§79 / §30).
	// ADR-058 Exception #2: the whole /studio surface is a dedicated
	// full-bleed surface in composer chrome — no left rail, no right
	// sidebar, no mobile tab bar. (The /create route 308-redirects to
	// /studio/create but is kept here for safety.)
	return routeId === '/(app)/create' || routeId === '/(app)/blog/[slug]' || routeId === '/(app)/content/[slug]' || routeId === '/(app)/guide/[slug]' || routeId === '/(app)/story/[slug]' || routeId === '/(app)/comparisons/[slug]' || routeId === '/(app)/video/[slug]' || routeId === '/(app)/list/[slug]' || routeId?.startsWith('/(app)/studio') === true;
}

// Routes that replace the shell's mobile tab bar with a page-owned
// fixed action bar (e.g. the forum thread's vote/bookmark/share strip,
// the service detail's StickyBookBar).
// Kept separate from isExceptionRoute — these routes keep the normal
// left nav, right sidebar, and compose chrome on desktop.
const NO_TAB_BAR_ROUTES = new Set(['/(app)/forum/[slug]', '/(app)/services/[slug]']);

export function isNoTabBarRoute(routeId: string | null): boolean {
	return routeId !== null && NO_TAB_BAR_ROUTES.has(routeId);
}
