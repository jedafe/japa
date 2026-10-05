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
	import { roadmapErrorKey, roadmapStatusOptions } from '$lib/enumOptions';
	import type { RoadmapStatus } from '$lib/types';

	interface Props {
		itemId: string;
		status: RoadmapStatus;
		compact?: boolean;
	}
	let { itemId, status, compact = true }: Props = $props();

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
