<script lang="ts">
	import { onMount, onDestroy } from 'svelte';
	import { Search, X, CornerDownLeft } from 'lucide-svelte';
	import { t } from '$lib/i18n';
	import { goto } from '$app/navigation';
	import type { DocSearchResult } from '$lib/types';
	import { AUDIENCE_LABEL_KEY } from '$lib/types';

	interface Props {
		onClose: () => void;
	}

	let { onClose }: Props = $props();

	let query = $state('');
	let results = $state<DocSearchResult[]>([]);
	let loading = $state(false);
	let activeIndex = $state(0);
	let inputEl: HTMLInputElement | undefined = undefined;

	// Debounced fetch — 200ms → /docs/search?q=&limit= (ADR-060).
	let debounceTimer: ReturnType<typeof setTimeout> | undefined = undefined;

	$effect(() => {
		clearTimeout(debounceTimer);
		const q = query.trim();
		if (q.length < 2) {
			results = [];
			loading = false;
			activeIndex = 0;
			return;
		}
		loading = true;
		debounceTimer = setTimeout(async () => {
			try {
				const res = await fetch(`/api/v1/docs/search?q=${encodeURIComponent(q)}&limit=10`);
				if (!res.ok) throw new Error('search failed');
				const json = await res.json();
				results = (json.data as DocSearchResult[]) ?? [];
			} catch {
				results = [];
			} finally {
				loading = false;
				activeIndex = 0;
			}
		}, 200);
	});

	onMount(() => {
		inputEl?.focus();
		document.body.style.overflow = 'hidden';
	});
	onDestroy(() => {
		document.body.style.overflow = '';
	});

	function onKeydown(e: KeyboardEvent) {
		if (e.key === 'Escape') {
			e.preventDefault();
			onClose();
		} else if (e.key === 'ArrowDown' && results.length > 0) {
			e.preventDefault();
			activeIndex = (activeIndex + 1) % results.length;
		} else if (e.key === 'ArrowUp' && results.length > 0) {
			e.preventDefault();
			activeIndex = (activeIndex - 1 + results.length) % results.length;
		} else if (e.key === 'Enter' && results[activeIndex]) {
			e.preventDefault();
			navigate(results[activeIndex]);
		}
	}

	function navigate(r: DocSearchResult) {
		goto(`/docs/${r.audience}/${r.slug}`);
		onClose();
	}
</script>

<svelte:window on:keydown={onKeydown} />

<div class="overlay">
	<button type="button" class="backdrop" onclick={onClose} aria-label={t('a11y.close_menu')}></button>
	<div class="palette" role="dialog" aria-label={t('search.title')} tabindex="-1">
		<div class="input-row">
			<Search size={18} class="search-icon" />
			<input
				bind:this={inputEl}
				bind:value={query}
				type="text"
				class="input"
				placeholder={t('search.placeholder')}
				aria-label={t('search.placeholder')}
			/>
			<button type="button" class="close-btn" onclick={onClose} aria-label={t('a11y.close_menu')}>
				<X size={16} />
			</button>
		</div>

		<div class="results">
			{#if loading}
				<p class="hint">{t('search.loading')}</p>
			{:else if query.trim().length < 2}
				<p class="hint">{t('search.start_typing')}</p>
			{:else if results.length === 0}
				<p class="hint">{t('search.empty')}</p>
			{:else}
				<ul class="result-list" role="listbox">
					{#each results as r, i (r.id)}
						<li>
							<button
								type="button"
								role="option"
								aria-selected={i === activeIndex}
								class="result"
								class:active={i === activeIndex}
								onclick={() => navigate(r)}
								onmouseenter={() => (activeIndex = i)}
							>
								<span class="result-title">{r.title}</span>
								<span class="result-meta">
									<span class="result-audience">{t(AUDIENCE_LABEL_KEY[r.audience], r.audience)}</span>
									{#if r.excerpt}<span class="result-excerpt">{r.excerpt}</span>{/if}
								</span>
							</button>
						</li>
					{/each}
				</ul>
				<div class="footer">
					<span class="kbd-hint"><CornerDownLeft size={12} /> enter</span>
					<span class="kbd-hint">↑↓ navigate</span>
					<span class="kbd-hint">esc close</span>
				</div>
			{/if}
		</div>
	</div>
</div>

<style>
	.overlay {
		position: fixed;
		inset: 0;
		z-index: 100;
	}
	.backdrop {
		position: fixed;
		inset: 0;
		background: rgb(0 0 0 / 0.5);
		border: none;
		cursor: default;
	}
	.palette {
		position: fixed;
		top: 12vh;
		left: 50%;
		transform: translateX(-50%);
		width: min(640px, 92vw);
		background: var(--color-surface);
		border: 1px solid var(--color-border);
		border-radius: var(--radius-card);
		box-shadow: 0 20px 60px rgb(0 0 0 / 0.3);
		overflow: hidden;
	}
	.input-row {
		display: flex;
		align-items: center;
		gap: 0.625rem;
		padding: 0.75rem 1rem;
		border-bottom: 1px solid var(--color-border);
	}
	:global(.search-icon) { color: var(--color-muted); flex-shrink: 0; }
	.input {
		flex: 1;
		border: none;
		outline: none;
		background: transparent;
		font-size: 1rem;
		color: var(--color-text);
	}
	.close-btn {
		display: grid;
		place-items: center;
		height: 1.75rem;
		width: 1.75rem;
		border-radius: var(--radius);
		color: var(--color-muted);
		background: transparent;
		border: none;
		cursor: pointer;
	}
	.close-btn:hover { background: var(--color-surface-subtle); color: var(--color-text); }

	.results { max-height: 60vh; overflow-y: auto; }
	.hint { padding: 1.5rem 1rem; text-align: center; color: var(--color-muted); font-size: 0.875rem; }
	.result-list { list-style: none; margin: 0; padding: 0.25rem; }
	.result {
		display: flex;
		flex-direction: column;
		gap: 0.125rem;
		width: 100%;
		padding: 0.625rem 0.75rem;
		border-radius: var(--radius);
		background: transparent;
		border: none;
		cursor: pointer;
		text-align: left;
	}
	.result:hover, .result.active { background: var(--color-surface-subtle); }
	.result-title { font-size: 0.875rem; font-weight: 500; color: var(--color-text); }
	.result-meta { display: flex; align-items: center; gap: 0.5rem; }
	.result-audience {
		font-size: 0.6875rem;
		text-transform: uppercase;
		letter-spacing: 0.05em;
		color: var(--color-primary);
		font-weight: 600;
		flex-shrink: 0;
	}
	.result-excerpt {
		font-size: 0.75rem;
		color: var(--color-muted);
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.footer {
		display: flex;
		gap: 0.75rem;
		padding: 0.5rem 0.75rem;
		border-top: 1px solid var(--color-border);
		font-size: 0.6875rem;
		color: var(--color-muted);
	}
	.kbd-hint { display: inline-flex; align-items: center; gap: 0.25rem; }
</style>
