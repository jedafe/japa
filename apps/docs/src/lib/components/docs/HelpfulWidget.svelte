<script lang="ts">
	import { onMount } from 'svelte';
	import { ThumbsUp, ThumbsDown } from 'lucide-svelte';
	import { t } from '$lib/i18n';

	interface Props {
		/** Page ID for localStorage persistence of the vote. */
		pageId: string;
		/** ISO timestamp of last review, or null. */
		lastReviewedAt?: string | null;
		lastReviewedBy?: string | null;
	}

	let { pageId, lastReviewedAt = null, lastReviewedBy = null }: Props = $props();

	let vote = $state<'up' | 'down' | null>(null);

	const storageKey = $derived(`jc:docs:vote:${pageId}`);

	// Load saved vote from localStorage after hydration (browser-only).
	onMount(() => {
		try {
			const saved = localStorage.getItem(storageKey);
			if (saved === 'up' || saved === 'down') vote = saved;
		} catch {
			// localStorage unavailable (private browsing, etc.)
		}
	});

	function persist(v: 'up' | 'down' | null) {
		vote = v;
		try {
			if (v) localStorage.setItem(storageKey, v);
			else localStorage.removeItem(storageKey);
		} catch {
			// localStorage unavailable
		}
	}

	function voteUp() {
		persist(vote === 'up' ? null : 'up');
	}
	function voteDown() {
		persist(vote === 'down' ? null : 'down');
	}

	let reviewedLabel = $derived(
		lastReviewedAt
			? t('page.last_reviewed', '', {
					date: new Date(lastReviewedAt).toLocaleDateString(undefined, {
						year: 'numeric',
						month: 'short',
						day: 'numeric'
					})
				})
			: t('page.not_reviewed')
	);
</script>

<div class="helpful">
	<div class="helpful-row">
		<span class="helpful-label">{t('a11y.helpful')}</span>
		<div class="helpful-buttons">
			<button
				type="button"
				class="vote-btn"
				class:active={vote === 'up'}
				onclick={voteUp}
				aria-label={t('a11y.helpful_yes')}
				aria-pressed={vote === 'up'}
			>
				<ThumbsUp size={16} />
			</button>
			<button
				type="button"
				class="vote-btn"
				class:active={vote === 'down'}
				onclick={voteDown}
				aria-label={t('a11y.helpful_no')}
				aria-pressed={vote === 'down'}
			>
				<ThumbsDown size={16} />
			</button>
		</div>
	</div>
	<p class="reviewed">
		{reviewedLabel}{#if lastReviewedBy && lastReviewedAt} · {lastReviewedBy}{/if}
	</p>
</div>

<style>
	.helpful {
		margin-top: 2.5rem;
		padding-top: 1.5rem;
		border-top: 1px solid var(--color-border);
	}
	.helpful-row {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 1rem;
	}
	.helpful-label {
		font-size: 0.875rem;
		font-weight: 500;
		color: var(--color-text);
	}
	.helpful-buttons { display: flex; gap: 0.5rem; }
	.vote-btn {
		display: inline-flex;
		align-items: center;
		gap: 0.25rem;
		height: 2.25rem;
		padding: 0 0.75rem;
		border-radius: var(--radius);
		border: 1px solid var(--color-border);
		background: var(--color-surface);
		color: var(--color-muted);
		cursor: pointer;
	}
	.vote-btn:hover { border-color: var(--color-border-input-hover); color: var(--color-text); }
	.vote-btn.active {
		border-color: var(--color-primary);
		color: var(--color-primary);
		background: color-mix(in srgb, var(--color-primary) 8%, transparent);
	}
	.reviewed {
		margin: 0.75rem 0 0;
		font-size: 0.75rem;
		color: var(--color-muted);
	}
</style>
