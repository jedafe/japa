<script lang="ts">
	// Search launcher button (icon + ⌘K hint) that opens the SearchPalette
	// modal. Same look/behavior as the Docs app's SearchLauncher (ADR-060).
	import { Search } from 'lucide-svelte';
	import { t } from '$lib/i18n';
	import SearchPalette from './SearchPalette.svelte';

	interface Props {
		/** Whether this launcher registers the global ⌘/Ctrl+K shortcut. */
		keyboardShortcut?: boolean;
		/** Optional override for the palette input placeholder (sidebar widget `placeholder` prop). */
		placeholder?: string;
	}

	let { keyboardShortcut = true, placeholder = '' }: Props = $props();

	let open = $state(false);

	function onKeydown(e: KeyboardEvent) {
		// ⌘K / Ctrl+K opens the palette globally.
		if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === 'k') {
			e.preventDefault();
			open = true;
		}
	}

	$effect(() => {
		if (!keyboardShortcut) return;
		window.addEventListener('keydown', onKeydown);
		return () => window.removeEventListener('keydown', onKeydown);
	});
</script>

<button
	type="button"
	class="launcher"
	onclick={() => (open = true)}
>
	<Search size={16} />
	<span class="label">{t('search.heading', 'Search')}</span>
	<span class="kbd" aria-hidden="true">⌘K</span>
</button>

{#if open}
	<SearchPalette onClose={() => (open = false)} {placeholder} />
{/if}

<style>
	/* Field-styled launcher: fills the topbar row so the icon widgets
	   (theme, language, account) sit flush right. */
	.launcher {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		flex: 1;
		min-width: 0;
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
	.label {
		flex: 1;
		min-width: 0;
		overflow: hidden;
		text-align: start;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.kbd {
		flex-shrink: 0;
		font-family: var(--font-mono);
		font-size: 0.6875rem;
		padding: 0.0625rem 0.25rem;
		border: 1px solid var(--color-border);
		border-radius: 4px;
		color: var(--color-muted);
	}
</style>
