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
	// ADR-058 Exception #2: /create collapses the nav and hides the right sidebar.
	return routeId === '/(app)/create' || routeId === '/(app)/blog/[slug]' || routeId === '/(app)/content/[slug]';
}
