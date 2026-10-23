<script lang="ts">
	// Global search palette — opened by SearchLauncher (click or ⌘K). Debounced
	// queries hit GET /search/autocomplete; selecting a suggestion navigates to
	// it, and the trailing "See all results" row submits the full search to
	// /search?q=. Mirrors the Docs app's SearchPalette (ADR-060 ⌘K pattern).
	import { Dialog, Command } from 'bits-ui';
	import { Search, X, CornerDownLeft } from 'lucide-svelte';
	import { t } from '$lib/i18n';
	import { goto } from '$app/navigation';
	import { api } from '$lib/api/client';

	interface SearchSuggestion {
		type: string;
		id: string;
		title?: string;
		excerpt?: string;
		url?: string;
		content_type?: string;
		slug?: string;
		username?: string;
		display_name?: string;
		name?: string;
	}

	interface Props {
		onClose: () => void;
		/** Optional override for the input placeholder (sidebar widget `placeholder` prop). */
		placeholder?: string;
	}

	let { onClose, placeholder = '' }: Props = $props();

	// Dialog is mounted already-open by SearchLauncher; bind:open lets Bits UI
	// manage the close transition (escape / outside-click), and onOpenChange
	// propagates the close back to the parent so it unmounts this component.
	let open = $state(true);

	let query = $state('');
	let suggestions = $state<SearchSuggestion[]>([]);
	let loading = $state(false);

	// Debounced fetch — 200ms → /search/autocomplete?q=&limit=.
	let debounceTimer: ReturnType<typeof setTimeout> | undefined = undefined;

	$effect(() => {
		clearTimeout(debounceTimer);
		const q = query.trim();
		if (q.length < 2) {
			suggestions = [];
			loading = false;
			return;
		}
		loading = true;
		debounceTimer = setTimeout(async () => {
			try {
				const res = await api.get<{ data: { suggestions: SearchSuggestion[] } }>(
					`/search/autocomplete?q=${encodeURIComponent(q)}&limit=10`
				);
				suggestions = res.data?.data?.suggestions ?? [];
			} catch {
				suggestions = [];
			} finally {
				loading = false;
			}
		}, 200);
		// Cancel the pending debounce on unmount/re-run so the fetch never
		// fires after the palette closes.
		return () => clearTimeout(debounceTimer);
	});

	function suggestionHref(s: SearchSuggestion): string {
		if (s.url) return s.url;
		if (s.slug) {
			return `/${s.type === 'user' ? 'u' : s.type === 'space' ? 'spaces' : s.type}/${s.slug}`;
		}
		return '';
	}

	function navigate(s: SearchSuggestion) {
		const href = suggestionHref(s);
		if (!href) return;
		goto(href);
		onClose();
	}

	function submitSearch() {
		const q = query.trim();
		if (!q) return;
		goto(`/search?q=${encodeURIComponent(q)}`);
		onClose();
	}

	// A stable unique value per suggestion for Command.Item (required for
	// keyboard navigation when shouldFilter={false}).
	function itemValue(s: SearchSuggestion, i: number): string {
		return s.id ? `${s.type}:${s.id}` : `${s.type}:${i}`;
	}
</script>

<Dialog.Root bind:open onOpenChange={(v) => { if (!v) onClose(); }}>
	<Dialog.Portal>
		<Dialog.Overlay class="palette-overlay" />
		<Dialog.Content class="palette" aria-label={t('search.heading', 'Search')}>
			<!-- Bits UI Dialog provides the focus trap, escape-key close,
			     outside-click close, and scroll lock. Command provides keyboard
			     navigation (arrows/Enter) over the async result list with
			     shouldFilter={false}. -->
			<Command.Root shouldFilter={false} loop label={t('search.heading', 'Search')}>
				<div class="input-row">
					<Search size={18} class="search-icon" />
					<Command.Input
						bind:value={query}
						class="input"
						placeholder={placeholder || t('search.placeholder')}
						aria-label={placeholder || t('search.placeholder')}
					/>
					<button type="button" class="close-btn" onclick={onClose} aria-label={t('a11y.close_menu')}>
						<X size={16} />
					</button>
				</div>

				<Command.List class="results">
					<Command.Viewport>
						{#if loading}
							<Command.Loading>
								<p class="hint">{t('search.loading', 'Searching…')}</p>
							</Command.Loading>
						{:else if query.trim().length < 2}
							<p class="hint">{t('search.start_typing')}</p>
						{:else}
							<ul class="result-list" role="listbox">
								{#each suggestions as s, i (itemValue(s, i))}
									<Command.Item
										value={itemValue(s, i)}
										onSelect={() => navigate(s)}
										class="result"
									>
										<span class="result-title">{s.title ?? s.name ?? s.display_name ?? s.username ?? ''}</span>
										<span class="result-meta">
											{#if s.content_type || s.type}
												<span class="result-type">{s.content_type ?? s.type}</span>
											{/if}
											{#if s.excerpt}<span class="result-excerpt">{s.excerpt}</span>{/if}
										</span>
									</Command.Item>
								{/each}
								{#if suggestions.length === 0}
									<p class="hint">{t('search.no_results')}</p>
								{/if}
								<Command.Item value="__all__" onSelect={submitSearch} class="result result-all">
									<Search size={14} />
									<span class="result-title">{t('search.view_all', 'See all results')}</span>
								</Command.Item>
							</ul>
							<div class="footer">
								<span class="kbd-hint"><CornerDownLeft class="rtl:-scale-x-100" size={12} /> {t('search.kbd_enter', 'enter')}</span>
								<span class="kbd-hint">{t('search.kbd_navigate', '↑↓ navigate')}</span>
								<span class="kbd-hint">{t('search.kbd_close', 'esc close')}</span>
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
		inset-inline-start: 50%;
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
		text-align: start;
		color: inherit;
		font: inherit;
	}
	/* Command.Item exposes data-highlighted for the keyboard-active state. */
	:global(.palette .result[data-highlighted]) { background: var(--color-surface-subtle); }
	:global(.palette .result-all) {
		flex-direction: row;
		align-items: center;
		gap: 0.5rem;
		border-top: 1px solid var(--color-border);
		border-radius: 0 0 var(--radius) var(--radius);
		color: var(--color-muted);
	}
	.result-title { font-size: 0.875rem; font-weight: 500; color: var(--color-text); }
	.result-meta { display: flex; align-items: center; gap: 0.5rem; }
	.result-type {
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
