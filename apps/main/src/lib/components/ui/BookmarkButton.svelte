<script lang="ts">
	import { Bookmark } from 'lucide-svelte';
	import { api } from '$lib/api/client';
	import { t } from '$lib/i18n';
	import { untrack } from 'svelte';
	import { requireAuthOrRedirect } from '$lib/auth/guest';

	interface Props {
		entityType: string;
		entityId: string;
		bookmarked: boolean;
		count?: number;
	}

	let {
		entityType,
		entityId,
		bookmarked: initial = false,
		count
	}: Props = $props();

	let isOn = $state(untrack(() => initial));
	let currentCount = $state(untrack(() => count));
	let loading = $state(false);

	async function toggle() {
		if (loading) return;
		// Guests can't bookmark — redirect to login instead of hitting a 401.
		if (!(await requireAuthOrRedirect())) return;
		loading = true;
		const wasOn = isOn;
		isOn = !wasOn;
		if (currentCount !== undefined) currentCount += wasOn ? -1 : 1;

		try {
			if (wasOn) {
				await api.delete(`/bookmarks/${entityType}/${entityId}`);
			} else {
				await api.post('/bookmarks', {
					entity_type: entityType,
					entity_id: entityId
				});
			}
		} catch {
			// Revert on failure.
			isOn = wasOn;
			if (currentCount !== undefined) currentCount += wasOn ? 1 : -1;
		} finally {
			loading = false;
		}
	}
</script>

<button
	type="button"
	onclick={toggle}
	disabled={loading}
	class="inline-flex items-center gap-1.5 rounded-[var(--radius)] px-2 py-1 text-sm text-[var(--color-text-subtle)] transition-colors hover:bg-[var(--color-surface-subtle)] disabled:opacity-50"
	aria-pressed={isOn}
	aria-label={t('bookmarks.toggle', 'Bookmark')}
>
	<Bookmark
		size={16}
		class={isOn ? 'fill-[var(--color-primary)] text-[var(--color-primary)]' : ''}
	/>
	{#if currentCount !== undefined}
		<span class="tabular-nums">{currentCount}</span>
	{/if}
</button>
