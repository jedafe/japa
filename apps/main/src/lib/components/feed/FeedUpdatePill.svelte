<script lang="ts">
	import { invalidateAll } from '$app/navigation';
	import { ArrowUp, X } from 'lucide-svelte';
	import { t } from '$lib/i18n';
	import { realtime } from '$lib/realtime/realtime.svelte';

	// The pill is only relevant on the home timeline; the parent renders it
	// conditionally. Here we just react to the realtime store counter.
	let count = $derived(realtime.feedUpdateCount);
	let visible = $derived(count > 0);

	/** Refresh the feed and dismiss the pill. */
	async function refresh() {
		realtime.dismissFeedUpdatePill();
		await invalidateAll();
		// Scroll back to the top so the newly loaded items are visible.
		if (typeof window !== 'undefined') {
			window.scrollTo({ top: 0, behavior: 'smooth' });
		}
	}

	/** Dismiss without refreshing (user will scroll manually). */
	function dismiss() {
		realtime.dismissFeedUpdatePill();
	}
</script>

{#if visible}
	<div
		class="sticky top-[var(--nav-height)] z-10 mx-auto flex w-full max-w-2xl items-center justify-between gap-3 border-b border-[var(--color-border)] bg-[var(--color-surface)] px-4 py-2 text-sm shadow-sm"
		role="status"
		aria-live="polite"
	>
		<button
			type="button"
			onclick={refresh}
			class="inline-flex items-center gap-1.5 font-medium text-[var(--color-primary)] hover:underline"
		>
			<ArrowUp size={14} />
			{t('feed.new_posts', '{n} new posts').replace('{n}', String(count))}
		</button>
		<button
			type="button"
			onclick={dismiss}
			class="grid h-6 w-6 place-items-center rounded-full text-[var(--color-muted)] hover:bg-[var(--color-surface-subtle)]"
			aria-label={t('common.dismiss', 'Dismiss')}
		>
			<X size={14} />
		</button>
	</div>
{/if}
