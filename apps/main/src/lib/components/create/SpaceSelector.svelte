<script lang="ts">
	import { t } from '$lib/i18n';
	import { ChevronDown, Search, Check } from 'lucide-svelte';
	import type { MeSpace } from '$lib/types';

	interface Props {
		spaces: MeSpace[];
		selectedId: string | null;
		onSelect: (id: string) => void;
		onfocus?: () => void;
	}

	let { spaces, selectedId, onSelect, onfocus }: Props = $props();

	let open = $state(false);
	let query = $state('');
	let triggerEl: HTMLElement | null = null;

	// Click-outside handler
	function onWindowClick(e: MouseEvent) {
		if (!open) return;
		const target = e.target as Node;
		if (triggerEl && !triggerEl.contains(target)) {
			// Check if click is inside the dropdown panel
			const panel = document.querySelector('.space-dropdown');
			if (!panel || !panel.contains(target)) {
				open = false;
			}
		}
	}

	function onKeydown(e: KeyboardEvent) {
		if (e.key === 'Escape') {
			open = false;
			triggerEl?.focus();
		}
	}

	const selectedSpace = $derived(spaces.find((s) => s.id === selectedId) ?? null);

	const filtered = $derived.by(() => {
		const q = query.trim().toLowerCase();
		if (!q) return spaces;
		return spaces.filter((s) => s.name.toLowerCase().includes(q) || s.slug.toLowerCase().includes(q));
	});

	function toggle() {
		open = !open;
		if (open) {
			onfocus?.();
			query = '';
			// Focus the search input after the dropdown renders
			queueMicrotask(() => {
				const input = document.querySelector<HTMLInputElement>('.space-search-input');
				input?.focus();
			});
		}
	}

	function pick(id: string) {
		onSelect(id);
		open = false;
		triggerEl?.focus();
	}
</script>

<svelte:window onclick={onWindowClick} onkeydown={onKeydown} />

<div class="space-field">
	<button
		type="button"
		class="space-trigger"
		bind:this={triggerEl}
		onclick={toggle}
		aria-haspopup="listbox"
		aria-expanded={open}
	>
		<span class="space-trigger-text" class:placeholder={!selectedSpace}>
			{#if selectedSpace}
				{#if selectedSpace.is_primary}<span class="space-star">★</span>{/if}
				{selectedSpace.name}
			{:else}
				{t('create.placeholder.space')}
			{/if}
		</span>
		<ChevronDown size={16} class={'space-chevron' + (open ? ' is-open' : '')} />
	</button>

	{#if open}
		<div class="space-dropdown" role="listbox">
			<div class="space-search">
				<Search size={14} class="space-search-icon" />
				<input
					type="text"
					class="space-search-input"
					placeholder={t('create.placeholder.space_search')}
					bind:value={query}
				/>
			</div>
			<div class="space-options">
				{#if filtered.length === 0}
					<div class="space-empty">{t('create.placeholder.space_no_results')}</div>
				{:else}
					{#each filtered as s (s.id)}
						<button
							type="button"
							class="space-option"
							class:selected={s.id === selectedId}
							role="option"
							aria-selected={s.id === selectedId}
							onclick={() => pick(s.id)}
						>
							<span class="space-option-name">
								{#if s.is_primary}<span class="space-star">★</span>{/if}
								{s.name}
							</span>
							{#if s.id === selectedId}
								<Check size={14} class="space-check" />
							{/if}
						</button>
					{/each}
				{/if}
			</div>
		</div>
	{/if}
</div>

<style>
	.space-field {
		position: relative;
	}

	.space-trigger {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 8px;
		width: 100%;
		padding: 10px 14px;
		border: 1px solid var(--color-border);
		border-radius: var(--radius);
		background: var(--color-surface);
		color: var(--color-text);
		font-size: 1rem;
		text-align: left;
		cursor: pointer;
		transition: border-color 0.15s, box-shadow 0.15s;
	}
	.space-trigger:hover {
		border-color: var(--color-text-subtle);
	}
	.space-trigger:focus,
	.space-trigger[aria-expanded='true'] {
		border-color: var(--color-primary);
		box-shadow: 0 0 0 3px color-mix(in srgb, var(--color-primary) 15%, transparent);
		outline: none;
	}
	.space-trigger-text {
		display: flex;
		align-items: center;
		gap: 4px;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.space-trigger-text.placeholder {
		color: var(--color-muted);
		font-weight: 500;
	}
	.space-star {
		color: var(--color-primary);
		font-weight: 700;
	}
	.space-trigger :global(.space-chevron) {
		flex-shrink: 0;
		color: var(--color-muted);
		transition: transform 0.15s;
	}
	.space-trigger :global(.space-chevron.is-open) {
		transform: rotate(180deg);
	}

	/* Dropdown panel */
	.space-dropdown {
		position: absolute;
		top: 100%;
		left: 0;
		right: 0;
		z-index: 40;
		margin-top: 4px;
		border: 1px solid var(--color-border);
		border-radius: var(--radius);
		background: var(--color-surface);
		box-shadow: 0 8px 24px rgb(0 0 0 / 0.12);
		overflow: hidden;
	}

	.space-search {
		position: relative;
		padding: 8px;
		border-bottom: 1px solid var(--color-border);
	}
	.space-search :global(.space-search-icon) {
		position: absolute;
		left: 16px;
		top: 50%;
		transform: translateY(-50%);
		color: var(--color-muted);
		pointer-events: none;
	}
	.space-search-input {
		width: 100%;
		padding: 6px 10px 6px 32px;
		border: 1px solid var(--color-border);
		border-radius: calc(var(--radius) - 2px);
		background: var(--color-surface);
		color: var(--color-text);
		font-size: 0.8125rem;
		outline: none;
		transition: border-color 0.15s;
	}
	.space-search-input:focus {
		border-color: var(--color-primary);
	}
	.space-search-input::placeholder {
		color: var(--color-muted);
	}

	.space-options {
		max-height: 220px;
		overflow-y: auto;
		padding: 4px;
	}
	.space-option {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 8px;
		width: 100%;
		padding: 8px 10px;
		border: none;
		border-radius: calc(var(--radius) - 2px);
		background: transparent;
		color: var(--color-text);
		font-size: 0.875rem;
		text-align: left;
		cursor: pointer;
		transition: background 0.1s;
	}
	.space-option:hover {
		background: var(--color-surface-subtle);
	}
	.space-option.selected {
		background: color-mix(in srgb, var(--color-primary) 10%, transparent);
		color: var(--color-primary);
		font-weight: 500;
	}
	.space-option-name {
		display: flex;
		align-items: center;
		gap: 4px;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.space-option :global(.space-check) {
		flex-shrink: 0;
		color: var(--color-primary);
	}
	.space-empty {
		padding: 1rem;
		text-align: center;
		color: var(--color-muted);
		font-size: 0.8125rem;
	}
</style>
