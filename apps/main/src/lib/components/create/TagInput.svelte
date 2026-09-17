<script lang="ts">
	import { t } from '$lib/i18n';
	import { X, Search } from 'lucide-svelte';
	import { searchHits } from '$lib/api/create';
	import type { SearchHit } from '$lib/types';

	interface Props {
		tags: string[];
		onTags: (next: string[]) => void;
		onfocus?: () => void;
		/** Max tags per content item, from system setting content.max_tags (default 5). */
		maxTags?: number;
	}

	let { tags, onTags, onfocus, maxTags = 5 }: Props = $props();

	let draft = $state('');
	let suggestions = $state<SearchHit[]>([]);
	let loading = $state(false);
	let activeIndex = $state(-1);
	let showSuggestions = $state(false);
	let searchTimer: ReturnType<typeof setTimeout> | null = null;
	let boxEl: HTMLElement | null = null;
	let placement: 'bottom' | 'top' = $state('bottom');
	let dropdownMaxHeight = $state(240);

	function addTag(value: string) {
		const v = value.trim().toLowerCase().replace(/\s+/g, '-');
		// Enforce content.max_tags (default 5) client-side (Section 14).
		// The backend also enforces this, but preventing the user from
		// adding more than the limit avoids a wasted API round-trip.
		if (v && !tags.includes(v) && tags.length < maxTags) onTags([...tags, v]);
		draft = '';
		suggestions = [];
		activeIndex = -1;
		showSuggestions = false;
	}

	function removeTag(index: number) {
		onTags(tags.filter((_, i) => i !== index));
	}

	function onInput(v: string) {
		draft = v;
		activeIndex = -1;
		if (searchTimer) clearTimeout(searchTimer);
		const q = v.trim();
		if (q.length < 2) {
			suggestions = [];
			loading = false;
			showSuggestions = false;
			return;
		}
		showSuggestions = true;
		loading = true;
		searchTimer = setTimeout(async () => {
			suggestions = await searchHits('tags', q);
			// Filter out tags already added
			suggestions = suggestions.filter((s) => !tags.includes(s.slug ?? s.label.toLowerCase()));
			loading = false;
		}, 250);
	}

	function onKeydown(e: KeyboardEvent) {
		if (e.key === 'Enter' || e.key === ',') {
			e.preventDefault();
			if (showSuggestions && activeIndex >= 0 && suggestions[activeIndex]) {
				const hit = suggestions[activeIndex];
				addTag(hit.slug ?? hit.label.toLowerCase());
			} else if (draft.trim()) {
				addTag(draft);
			}
		} else if (e.key === 'ArrowDown' && showSuggestions && suggestions.length > 0) {
			e.preventDefault();
			activeIndex = Math.min(activeIndex + 1, suggestions.length - 1);
		} else if (e.key === 'ArrowUp' && showSuggestions && suggestions.length > 0) {
			e.preventDefault();
			activeIndex = Math.max(activeIndex - 1, 0);
		} else if (e.key === 'Backspace' && draft === '' && tags.length > 0) {
			onTags(tags.slice(0, -1));
		} else if (e.key === 'Escape') {
			showSuggestions = false;
			activeIndex = -1;
		}
	}

	function pickSuggestion(hit: SearchHit) {
		addTag(hit.slug ?? hit.label.toLowerCase());
	}

	function onWindowClick(e: MouseEvent) {
		if (!showSuggestions) return;
		if (boxEl && !boxEl.contains(e.target as Node)) {
			showSuggestions = false;
			activeIndex = -1;
		}
	}

	function measurePlacement() {
		if (!boxEl) return;
		const rect = boxEl.getBoundingClientRect();
		const desired = 240;
		const spaceBelow = Math.max(0, window.innerHeight - rect.bottom);
		const spaceAbove = Math.max(0, rect.top);
		if (spaceBelow >= spaceAbove) {
			placement = 'bottom';
			dropdownMaxHeight = Math.min(desired, Math.max(spaceBelow - 16, 120));
		} else {
			placement = 'top';
			dropdownMaxHeight = Math.min(desired, Math.max(spaceAbove - 16, 120));
		}
	}

	function onFocus() {
		onfocus?.();
		if (draft.trim().length >= 2) showSuggestions = true;
		queueMicrotask(measurePlacement);
	}

	// Reposition the dropdown as the list grows or the page scrolls.
	$effect(() => {
		if (showSuggestions) {
			queueMicrotask(measurePlacement);
		}
	});
</script>

<svelte:window onclick={onWindowClick} onscroll={measurePlacement} onresize={measurePlacement} />

<div class="tag-field" bind:this={boxEl}>
	<div class="tag-box" class:has-tags={tags.length > 0}>
		<div class="tag-input-wrap">
			<Search size={13} class="tag-input-icon" />
			<input
				type="text"
				class="tag-input"
				placeholder={tags.length === 0 ? t('create.placeholder.tag_search') : ''}
				value={draft}
				onfocus={onFocus}
				oninput={(e) => onInput(e.currentTarget.value)}
				onkeydown={onKeydown}
				onblur={() => {
					// Delay to allow click on suggestion
					setTimeout(() => {
						if (draft.trim()) addTag(draft);
					}, 150);
				}}
			/>
		</div>
	</div>

	{#if showSuggestions}
		<div
			class="tag-dropdown"
			class:top={placement === 'top'}
			class:bottom={placement === 'bottom'}
			role="listbox"
			style:max-height="{dropdownMaxHeight}px"
		>
			{#if loading}
				<div class="tag-dropdown-state">{t('media.picker.loading')}</div>
			{:else}
				{#if suggestions.length > 0}
					{#each suggestions as hit, i (hit.id)}
						<button
							type="button"
							class="tag-suggestion"
							class:active={i === activeIndex}
							role="option"
							aria-selected={i === activeIndex}
							onclick={() => pickSuggestion(hit)}
							onmouseenter={() => (activeIndex = i)}
						>
							<span class="tag-suggestion-name">#{hit.label}</span>
							{#if hit.slug && hit.slug !== hit.label.toLowerCase()}
								<span class="tag-suggestion-slug">{hit.slug}</span>
							{/if}
						</button>
					{/each}
				{:else}
					<div class="tag-dropdown-state tag-dropdown-create">
						{t('create.placeholder.tag_no_results')}
					</div>
				{/if}
				{#if draft.trim() && !suggestions.some((s) => (s.slug ?? s.label.toLowerCase()) === draft.trim().toLowerCase().replace(/\s+/g, '-'))}
					<button
						type="button"
						class="tag-suggestion tag-suggestion-create"
						onclick={() => addTag(draft)}
						onmouseenter={() => (activeIndex = suggestions.length)}
					>
						+ <strong>{draft.trim().toLowerCase().replace(/\s+/g, '-')}</strong>
					</button>
				{/if}
			{/if}
		</div>
	{/if}
</div>

{#if tags.length > 0}
	<div class="selected-tags">
		{#each tags as tag, i (i)}
			<span class="tag-chip">
				#{tag}
				<button type="button" onclick={() => removeTag(i)} aria-label={t('create.action.remove_tag')}>
					<X size={12} />
				</button>
			</span>
		{/each}
	</div>
{/if}

<style>
	.tag-field {
		position: relative;
		width: 100%;
	}
	.tag-box {
		display: flex;
		align-items: center;
		gap: 6px;
		height: 2.375rem;
		padding: 0 14px;
		border: 1px solid var(--color-border);
		border-radius: var(--radius);
		background: var(--color-surface);
		transition: border-color 0.15s, box-shadow 0.15s;
	}
	.tag-box:focus-within {
		border-color: var(--color-primary);
		box-shadow: 0 0 0 3px color-mix(in srgb, var(--color-primary) 15%, transparent);
	}

	.tag-input-wrap {
		position: relative;
		display: flex;
		align-items: center;
		flex: 1;
		min-width: 140px;
	}
	.tag-input-wrap :global(.tag-input-icon) {
		flex-shrink: 0;
		color: var(--color-muted);
		pointer-events: none;
		margin-right: 4px;
	}
	.tag-input {
		flex: 1;
		min-width: 100px;
		border: none;
		background: transparent;
		color: var(--color-text);
		font-size: 0.875rem;
		outline: none;
	}
	.tag-input::placeholder {
		color: var(--color-muted);
	}

	.selected-tags {
		display: flex;
		flex-wrap: wrap;
		gap: 6px;
		margin-top: 8px;
	}
	.tag-chip {
		display: inline-flex;
		align-items: center;
		gap: 4px;
		padding: 3px 8px;
		border-radius: 9999px;
		background: var(--color-surface-subtle);
		color: var(--color-primary);
		font-size: 0.75rem;
		font-weight: 500;
	}
	.tag-chip button {
		display: inline-flex;
		align-items: center;
		justify-content: center;
		min-width: 32px;
		min-height: 32px;
		padding: 0;
		border: none;
		border-radius: 50%;
		background: transparent;
		color: var(--color-muted);
		cursor: pointer;
		transition: color 0.15s, background 0.15s;
	}
	.tag-chip button:hover {
		color: var(--color-text);
		background: color-mix(in srgb, var(--color-text) 8%, transparent);
	}
	.tag-chip button :global(svg) {
		width: 14px;
		height: 14px;
	}

	/* Dropdown */
	.tag-dropdown {
		position: absolute;
		left: 0;
		right: 0;
		z-index: 40;
		border: 1px solid var(--color-border);
		border-radius: var(--radius);
		background: var(--color-surface);
		box-shadow: 0 8px 24px rgb(0 0 0 / 0.12);
		overflow: hidden;
		overflow-y: auto;
		padding: 4px;
	}
	.tag-dropdown.bottom {
		top: 100%;
		bottom: auto;
		margin-top: 4px;
	}
	.tag-dropdown.top {
		bottom: 100%;
		top: auto;
		margin-bottom: 4px;
	}
	.tag-dropdown-state {
		padding: 0.75rem;
		text-align: center;
		color: var(--color-muted);
		font-size: 0.8125rem;
	}
	.tag-dropdown-create {
		color: var(--color-text-subtle);
	}
	.tag-suggestion {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 8px;
		width: 100%;
		padding: 7px 10px;
		border: none;
		border-radius: calc(var(--radius) - 2px);
		background: transparent;
		color: var(--color-text);
		font-size: 0.8125rem;
		text-align: left;
		cursor: pointer;
		transition: background 0.1s;
	}
	.tag-suggestion:hover,
	.tag-suggestion.active {
		background: var(--color-surface-subtle);
	}
	.tag-suggestion.active {
		color: var(--color-primary);
	}
	.tag-suggestion-name {
		font-weight: 500;
	}
	.tag-suggestion-slug {
		color: var(--color-muted);
		font-size: 0.75rem;
	}
	.tag-suggestion-create {
		color: var(--color-primary);
		border-top: 1px solid var(--color-border);
		border-radius: 0 0 calc(var(--radius) - 2px) calc(var(--radius) - 2px);
		margin-top: 2px;
		padding-top: 8px;
	}
	.tag-suggestion-create strong {
		font-weight: 600;
	}
</style>
