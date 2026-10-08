<script lang="ts">
	// Ref: japaconnect-engineering/sections/113-public-product-roadmap.md
	// Moderator inline status control — visible only to viewers carrying the
	// roadmap.moderate node (checked by the parent). PATCHes
	// /admin/roadmap/items/{id}/status then invalidateAll() so the board and
	// detail views re-fetch. Options are constrained to the valid lifecycle
	// transitions (§113); the backend re-validates regardless.
	import { invalidateAll } from '$app/navigation';
	import AutocompleteSelect from '$lib/components/ui/AutocompleteSelect.svelte';
	import { api } from '$lib/api/client';
	import { t } from '$lib/i18n';
	import { roadmapErrorKey, roadmapStatusLabel, roadmapStatusOptions } from '$lib/enumOptions';
	import { Flag } from 'lucide-svelte';
	import type { RoadmapStatus } from '$lib/types';

	interface Props {
		itemId: string;
		status: RoadmapStatus;
		/** §4.2 — linked items derive status from the flag's rollout phase;
		 *  manual transitions 409 (status_derived). When set, the control
		 *  renders as an inert chip instead of the dropdown. */
		featureFlagKey?: string | null;
		compact?: boolean;
	}
	let { itemId, status, featureFlagKey = null, compact = true }: Props = $props();

	let pending = $state(false);
	let errorKey = $state('');

	async function changeStatus(value: string) {
		if (value === status || pending) return;
		pending = true;
		errorKey = '';
		const res = await api.patch(`/admin/roadmap/items/${itemId}/status`, { status: value });
		pending = false;
		if (res.error) {
			// Map backend codes (invalid_status_transition, unauthorized, …) to
			// a translated message; unknown codes get the generic failure text.
			errorKey = roadmapErrorKey(res.error.code, 'roadmap.status_update_failed');
			return;
		}
		await invalidateAll();
	}
</script>

{#if featureFlagKey}
	<!-- Flag-linked item: status is owned by the flag's rollout phase, so
	     render the current status as an inert chip with the derivation
	     hint rather than a dropdown whose every option would 409. -->
	<span
		class="inline-flex items-center gap-1.5 rounded-[var(--radius)] border border-[var(--color-border)] px-2 py-1 text-[11px] font-medium text-[var(--color-muted)]"
		title={t('roadmap.status_derived_hint', 'Status is derived from the linked feature flag — change the flag’s phase to move it.')}
	>
		<Flag size={12} />
		{roadmapStatusLabel(t, status)}
	</span>
{:else}
	<div class="flex flex-col items-end gap-1">
		<AutocompleteSelect
			value={status}
			options={roadmapStatusOptions(t, status)}
			onValueChange={changeStatus}
			disabled={pending}
			{compact}
			ariaLabel={t('roadmap.change_status', 'Change status')}
			class="w-36"
		/>
		{#if errorKey}
			<p class="text-[11px] text-[var(--color-danger)]" role="alert">{t(errorKey)}</p>
		{/if}
	</div>
{/if}
