<script lang="ts">
	import { Dialog, Command } from 'bits-ui';
	import { Search, X, CornerDownLeft } from 'lucide-svelte';
	import { t } from '$lib/i18n';
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import type { DocSearchResult, DocAudience } from '$lib/types';
	import { AUDIENCE_LABEL_KEY } from '$lib/types';

	interface Props {
		onClose: () => void;
		/**
		 * Active audience to scope results to (ADR-060 — audience-filtered search).
		 * Derived from the current route when not provided.
		 */
		audience?: DocAudience | null;
	}

	let { onClose, audience = null }: Props = $props();

	// Dialog is mounted already-open by SearchLauncher; bind:open lets Bits UI
	// manage the close transition (escape / outside-click), and onOpenChange
	// propagates the close back to the parent so it unmounts this component.
	let open = $state(true);

	let query = $state('');
	let results = $state<DocSearchResult[]>([]);
	let loading = $state(false);

	// Resolve the active audience from the route (/docs/[audience]/...) when the
	// caller doesn't pass one explicitly. ADR-060 — audience-filtered search.
	let resolvedAudience = $derived<DocAudience | null>(
		audience ?? (page.params.audience as DocAudience | undefined) ?? null
	);

	// Debounced fetch — 200ms → /docs/search?q=&audience=&limit= (ADR-060).
	let debounceTimer: ReturnType<typeof setTimeout> | undefined = undefined;

	$effect(() => {
		clearTimeout(debounceTimer);
		const q = query.trim();
		if (q.length < 2) {
			results = [];
			loading = false;
			return;
		}
		loading = true;
		debounceTimer = setTimeout(async () => {
			try {
				const params = new URLSearchParams({ q, limit: '10' });
				if (resolvedAudience) params.set('audience', resolvedAudience);
				const res = await fetch(`/api/v1/docs/search?${params}`);
				if (!res.ok) throw new Error('search failed');
				const json = await res.json();
				results = (json.data as DocSearchResult[]) ?? [];
			} catch {
				results = [];
			} finally {
				loading = false;
			}
		}, 200);
		// Cleanup on unmount or re-run: cancel the pending debounce so the
		// fetch never fires after the palette closes (ADR-060 — $effect must
		// return cleanup for setTimeout debounce).
		return () => clearTimeout(debounceTimer);
	});

	function navigate(r: DocSearchResult) {
		goto(`/docs/${r.audience}/${r.slug}`);
		onClose();
	}

	// A stable unique value per result for Command.Item (required for keyboard
	// navigation and filtering when shouldFilter={false}).
	function itemValue(r: DocSearchResult): string {
		return `${r.audience}:${r.slug}`;
	}
</script>

<Dialog.Root bind:open onOpenChange={(v) => { if (!v) onClose(); }}>
	<Dialog.Portal>
		<Dialog.Overlay class="palette-overlay" />
		<Dialog.Content class="palette" aria-label={t('search.title')}>
			<!-- Bits UI Dialog provides the focus trap, escape-key close,
			     outside-click close, and scroll lock (preventScroll default).
			     Command provides keyboard navigation (arrows/Enter) over the
			     async result list with shouldFilter={false}. -->
			<Command.Root shouldFilter={false} loop label={t('search.title')}>
				<div class="input-row">
					<Search size={18} class="search-icon" />
					<Command.Input
						bind:value={query}
						class="input"
						placeholder={t('search.placeholder')}
						aria-label={t('search.placeholder')}
					/>
					<button type="button" class="close-btn" onclick={onClose} aria-label={t('a11y.close_menu')}>
						<X size={16} />
					</button>
				</div>

				<Command.List class="results">
					<Command.Viewport>
						{#if loading}
							<Command.Loading>
								<p class="hint">{t('search.loading')}</p>
							</Command.Loading>
						{:else if query.trim().length < 2}
							<p class="hint">{t('search.start_typing')}</p>
						{:else if results.length === 0}
							<p class="hint">{t('search.empty')}</p>
						{:else}
							<ul class="result-list" role="listbox">
								{#each results as r (itemValue(r))}
									<Command.Item
										value={itemValue(r)}
										onSelect={() => navigate(r)}
										class="result"
									>
										<span class="result-title">{r.title}</span>
										<span class="result-meta">
											<span class="result-audience">
												{t(AUDIENCE_LABEL_KEY[r.audience], r.audience)}
											</span>
											{#if r.excerpt}<span class="result-excerpt">{r.excerpt}</span>{/if}
										</span>
									</Command.Item>
								{/each}
							</ul>
							<div class="footer">
								<span class="kbd-hint"><CornerDownLeft size={12} /> {t('search.kbd_enter')}</span>
								<span class="kbd-hint">{t('search.kbd_navigate')}</span>
								<span class="kbd-hint">{t('search.kbd_close')}</span>
							</div>
						{/if}
					</Command.Viewport>
				</Command.List>
			</Command.Root>
		</Dialog.Content>
	</Dialog.Portal>
</Dialog.Root>

<style>
	/* Classes passed to Bits UI primitives (Dialog.Overlay, Dialog.Content,
	   Command.Input, Command.List, Command.Item) are rendered through a
	   portal, so Svelte's scoped-CSS checker can't see them statically.
	   These class names are unique to this palette, so :global is safe. */
	:global(.palette-overlay) {
		position: fixed;
		inset: 0;
		z-index: 100;
		background: rgb(0 0 0 / 0.5);
	}
	:global(.palette) {
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
		z-index: 101;
	}
	.input-row {
		display: flex;
		align-items: center;
		gap: 0.625rem;
		padding: 0.75rem 1rem;
		border-bottom: 1px solid var(--color-border);
	}
	:global(.search-icon) { color: var(--color-muted); flex-shrink: 0; }
	:global(.palette .input) {
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

	:global(.palette .results) { max-height: 60vh; overflow-y: auto; }
	.hint { padding: 1.5rem 1rem; text-align: center; color: var(--color-muted); font-size: 0.875rem; }
	.result-list { list-style: none; margin: 0; padding: 0.25rem; }
	:global(.palette .result) {
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
		color: inherit;
		font: inherit;
	}
	/* Command.Item exposes data-highlighted for the keyboard-active state. */
	:global(.palette .result[data-highlighted]) { background: var(--color-surface-subtle); }
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
