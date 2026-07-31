<script lang="ts">
	import { Search } from 'lucide-svelte';
	import { t } from '$lib/i18n';
	import SearchPalette from '$lib/components/docs/SearchPalette.svelte';

	let open = $state(false);

	function onKeydown(e: KeyboardEvent) {
		// ⌘K / Ctrl+K opens the palette globally.
		if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === 'k') {
			e.preventDefault();
			open = true;
		}
	}
</script>

<svelte:window onkeydown={onKeydown} />

<button
	type="button"
	class="launcher"
	onclick={() => (open = true)}
	aria-label={t('a11y.open_search')}
>
	<Search size={16} />
	<span class="kbd">⌘K</span>
</button>

{#if open}
	<SearchPalette onClose={() => (open = false)} />
{/if}

<style>
	.launcher {
		display: inline-flex;
		align-items: center;
		gap: 0.5rem;
		height: 2.25rem;
		padding: 0 0.625rem;
		border: 1px solid var(--color-border);
		border-radius: var(--radius);
		background: var(--color-surface);
		color: var(--color-muted);
		cursor: pointer;
		font-size: 0.8125rem;
	}
	.launcher:hover { border-color: var(--color-border-input-hover); color: var(--color-text); }
	.kbd {
		font-family: var(--font-mono);
		font-size: 0.6875rem;
		padding: 0.0625rem 0.25rem;
		border: 1px solid var(--color-border);
		border-radius: 4px;
		color: var(--color-muted);
	}
</style>
