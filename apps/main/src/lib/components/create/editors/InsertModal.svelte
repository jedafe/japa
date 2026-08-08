<script lang="ts">
	// InsertModal — replaces window.prompt/window.alert for YouTube, Link,
	// and Coupon inserts in the block editor and WYSIWYG toolbar.
	// Uses bits-ui Dialog for a consistent modal experience.
	import { Dialog } from 'bits-ui';
	import { untrack } from 'svelte';
	import { t } from '$lib/i18n';
	import { X, Search, Loader2, Link as LinkIcon, Video, Ticket, ExternalLink, Check } from 'lucide-svelte';
	import { searchHits } from '$lib/api/create';
	import type { SearchHit } from '$lib/types';

	type InsertKind = 'youtube' | 'link' | 'coupon';

	interface Props {
		kind: InsertKind;
		/** For link inserts, the current href (if any) to pre-fill. */
		currentUrl?: string;
		onConfirm: (value: string) => void;
		onClose: () => void;
	}

	let { kind, currentUrl = '', onConfirm, onClose }: Props = $props();

	let inputValue = $state(untrack(() => currentUrl));
	let couponQuery = $state('');
	let couponResults = $state<SearchHit[]>([]);
	let couponSelected = $state<SearchHit | null>(null);
	let searching = $state(false);
	let searchTimer: ReturnType<typeof setTimeout> | null = null;
	let error = $state<string | null>(null);

	const youtubeIcon = Video;

	const config = {
		youtube: {
			title: t('create.editor.insert.youtube_title', 'Insert YouTube Video'),
			label: t('create.editor.insert.url_label', 'YouTube URL'),
			placeholder: 'https://www.youtube.com/watch?v=…',
			icon: youtubeIcon,
			buttonText: t('create.editor.insert.insert', 'Insert'),
			hint: t('create.editor.insert.youtube_hint', 'Paste a YouTube video or Shorts URL')
		},
		link: {
			title: t('create.editor.insert.link_title', 'Insert Link'),
			label: t('create.editor.insert.url_label', 'URL'),
			placeholder: 'https://example.com',
			icon: LinkIcon,
			buttonText: t('create.editor.insert.insert', 'Insert'),
			hint: t('create.editor.insert.link_hint', 'Paste a URL. HTTP(S) links with no selection become a preview card.')
		},
		coupon: {
			title: t('create.editor.insert.coupon_title', 'Insert Coupon'),
			label: t('create.editor.insert.search_label', 'Search coupons'),
			placeholder: t('create.editor.insert.coupon_placeholder', 'Name or code…'),
			icon: Ticket,
			buttonText: t('create.editor.insert.insert', 'Insert'),
			hint: t('create.editor.insert.coupon_hint', 'Search for a coupon by name or code, then select it.')
		}
	};

	const cfg = $derived(config[kind]);

	function handleConfirm() {
		if (kind === 'coupon') {
			if (couponSelected) {
				onConfirm(couponSelected.id + '|' + couponSelected.label);
			}
		} else {
			const url = inputValue.trim();
			if (url) onConfirm(url);
		}
	}

	function handleCouponSearch(query: string) {
		couponQuery = query;
		couponSelected = null;
		if (searchTimer) clearTimeout(searchTimer);
		if (query.trim().length < 2) {
			couponResults = [];
			return;
		}
		searchTimer = setTimeout(async () => {
			searching = true;
			error = null;
			try {
				couponResults = await searchHits('coupons', query.trim());
			} catch (e) {
				error = e instanceof Error ? e.message : t('create.editor.insert.search_failed', 'Search failed');
				couponResults = [];
			} finally {
				searching = false;
			}
		}, 300);
	}

	function selectCoupon(hit: SearchHit) {
		couponSelected = hit;
	}

	function handleKeydown(e: KeyboardEvent) {
		if (e.key === 'Enter' && (kind !== 'coupon' || couponSelected)) {
			e.preventDefault();
			handleConfirm();
		}
	}
</script>

<Dialog.Root open={true} onOpenChange={(open) => { if (!open) onClose(); }}>
	<Dialog.Portal>
		<Dialog.Overlay class="insert-overlay" />
		<Dialog.Content class="insert-content">
			<Dialog.Title class="insert-title">
				{#if cfg.icon}<cfg.icon size={18} />{/if}
				{cfg.title}
			</Dialog.Title>
			<Dialog.Close class="insert-close">
				<X size={16} />
			</Dialog.Close>

			<div class="insert-body">
				{#if kind === 'coupon'}
					<label class="insert-label" for="coupon-search">{cfg.label}</label>
					<div class="insert-search-row">
						<Search size={16} class="insert-search-icon" />
						<input
							id="coupon-search"
							type="text"
							class="insert-input insert-input-search"
							placeholder={cfg.placeholder}
							value={couponQuery}
							oninput={(e) => handleCouponSearch(e.currentTarget.value)}
							autocomplete="off"
						/>
						{#if searching}
							<Loader2 size={16} class="insert-spinner" />
						{/if}
					</div>

					{#if error}
						<p class="insert-error">{error}</p>
					{/if}

					{#if couponResults.length > 0}
						<div class="insert-results">
							{#each couponResults as hit (hit.id)}
								<button
									type="button"
									class="insert-result-item"
									class:active={couponSelected?.id === hit.id}
									onclick={() => selectCoupon(hit)}
								>
									<Ticket size={16} />
									<span class="insert-result-label">{hit.label}</span>
									{#if couponSelected?.id === hit.id}
										<Check size={16} class="insert-result-check" />
									{/if}
								</button>
							{/each}
						</div>
					{:else if couponQuery.trim().length >= 2 && !searching}
						<p class="insert-empty">{t('create.editor.insert.no_coupons', 'No coupons found')}</p>
					{/if}

					<p class="insert-hint">{cfg.hint}</p>
				{:else}
					<label class="insert-label" for="insert-url">{cfg.label}</label>
					<div class="insert-search-row">
						{#if cfg.icon}<cfg.icon size={16} class="insert-search-icon" />{/if}
						<input
							id="insert-url"
							type="url"
							class="insert-input insert-input-search"
							placeholder={cfg.placeholder}
							bind:value={inputValue}
							onkeydown={handleKeydown}
							autocomplete="off"
						/>
					</div>
					<p class="insert-hint">{cfg.hint}</p>
				{/if}
			</div>

			<div class="insert-actions">
				<button type="button" class="insert-btn insert-btn-cancel" onclick={onClose}>
					{t('create.editor.insert.cancel', 'Cancel')}
				</button>
				<button
					type="button"
					class="insert-btn insert-btn-confirm"
					disabled={kind === 'coupon' ? !couponSelected : !inputValue.trim()}
					onclick={handleConfirm}
				>
					{#if kind === 'link'}
						<ExternalLink size={15} />
					{:else if cfg.icon}
						<cfg.icon size={15} />
					{/if}
					{cfg.buttonText}
				</button>
			</div>
		</Dialog.Content>
	</Dialog.Portal>
</Dialog.Root>

<style>
	:global(.insert-overlay) {
		position: fixed;
		inset: 0;
		z-index: 200;
		background: rgb(0 0 0 / 0.4);
		backdrop-filter: blur(2px);
	}

	:global(.insert-content) {
		position: fixed;
		z-index: 201;
		top: 50%;
		left: 50%;
		transform: translate(-50%, -50%);
		width: 90%;
		max-width: 460px;
		display: flex;
		flex-direction: column;
		gap: 0;
		border: 1px solid var(--color-border);
		border-radius: 12px;
		background: var(--color-surface);
		box-shadow: 0 20px 60px rgb(0 0 0 / 0.25);
		animation: insert-pop 0.15s ease-out;
	}

	@keyframes insert-pop {
		from { opacity: 0; transform: translate(-50%, -48%) scale(0.96); }
		to   { opacity: 1; transform: translate(-50%, -50%) scale(1); }
	}

	:global(.insert-title) {
		display: flex;
		align-items: center;
		gap: 8px;
		padding: 16px 20px 0;
		font-size: 1rem;
		font-weight: 600;
		color: var(--color-text);
	}

	:global(.insert-close) {
		position: absolute;
		top: 14px;
		right: 14px;
		display: flex;
		align-items: center;
		justify-content: center;
		width: 28px;
		height: 28px;
		border: none;
		border-radius: 6px;
		background: transparent;
		color: var(--color-text-subtle);
		cursor: pointer;
		transition: background 0.12s, color 0.12s;
	}
	:global(.insert-close:hover) {
		background: var(--color-surface-subtle);
		color: var(--color-text);
	}

	:global(.insert-body) {
		padding: 16px 20px;
		display: flex;
		flex-direction: column;
		gap: 10px;
	}

	:global(.insert-label) {
		font-size: 0.8125rem;
		font-weight: 500;
		color: var(--color-text-subtle);
	}

	:global(.insert-search-row) {
		position: relative;
		display: flex;
		align-items: center;
		gap: 8px;
		padding: 0 12px;
		border: 1px solid var(--color-border);
		border-radius: 8px;
		background: var(--color-surface);
		transition: border-color 0.15s;
	}
	:global(.insert-search-row:focus-within) {
		border-color: var(--color-primary);
	}

	:global(.insert-search-icon) {
		flex-shrink: 0;
		color: var(--color-text-subtle);
	}

	:global(.insert-input) {
		flex: 1;
		width: 100%;
		padding: 10px 0;
		border: none;
		background: transparent;
		color: var(--color-text);
		font-size: 0.875rem;
		outline: none;
	}
	:global(.insert-input::placeholder) {
		color: var(--color-text-subtle);
	}

	:global(.insert-spinner) {
		flex-shrink: 0;
		color: var(--color-text-subtle);
		animation: spin 0.8s linear infinite;
	}
	@keyframes spin { to { transform: rotate(360deg); } }

	:global(.insert-hint) {
		font-size: 0.75rem;
		color: var(--color-text-subtle);
		line-height: 1.4;
	}

	:global(.insert-error) {
		font-size: 0.75rem;
		color: var(--color-danger);
	}

	:global(.insert-empty) {
		font-size: 0.8125rem;
		color: var(--color-text-subtle);
		padding: 8px 0;
	}

	:global(.insert-results) {
		display: flex;
		flex-direction: column;
		gap: 4px;
		max-height: 200px;
		overflow-y: auto;
		padding: 4px 0;
	}

	:global(.insert-result-item) {
		display: flex;
		align-items: center;
		gap: 10px;
		padding: 8px 12px;
		border: 1px solid transparent;
		border-radius: 8px;
		background: transparent;
		color: var(--color-text);
		font-size: 0.875rem;
		text-align: left;
		cursor: pointer;
		transition: background 0.1s, border-color 0.1s;
	}
	:global(.insert-result-item:hover) {
		background: var(--color-surface-subtle);
	}
	:global(.insert-result-item.active) {
		border-color: var(--color-primary);
		background: color-mix(in srgb, var(--color-primary) 8%, transparent);
	}

	:global(.insert-result-label) {
		flex: 1;
		min-width: 0;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}

	:global(.insert-result-check) {
		flex-shrink: 0;
		color: var(--color-primary);
	}

	:global(.insert-actions) {
		display: flex;
		justify-content: flex-end;
		gap: 8px;
		padding: 0 20px 16px;
	}

	:global(.insert-btn) {
		display: inline-flex;
		align-items: center;
		gap: 6px;
		padding: 8px 16px;
		border: 1px solid var(--color-border);
		border-radius: 8px;
		font-size: 0.8125rem;
		font-weight: 500;
		cursor: pointer;
		transition: background 0.12s, border-color 0.12s, opacity 0.12s;
	}
	:global(.insert-btn:disabled) {
		opacity: 0.5;
		cursor: not-allowed;
	}

	:global(.insert-btn-cancel) {
		background: transparent;
		color: var(--color-text);
	}
	:global(.insert-btn-cancel:hover) {
		background: var(--color-surface-subtle);
	}

	:global(.insert-btn-confirm) {
		background: var(--color-primary);
		border-color: var(--color-primary);
		color: var(--color-surface);
	}
	:global(.insert-btn-confirm:hover:not(:disabled)) {
		opacity: 0.9;
	}
</style>
