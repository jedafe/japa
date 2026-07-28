<script lang="ts">
	import { ThumbsUp, ThumbsDown } from 'lucide-svelte';
	import { t } from '$lib/i18n';

	interface Props {
		/** ISO timestamp of last review, or null. */
		lastReviewedAt?: string | null;
		lastReviewedBy?: string | null;
	}

	let { lastReviewedAt = null, lastReviewedBy = null }: Props = $props();

	let vote = $state<'up' | 'down' | null>(null);

	function voteUp() {
		vote = vote === 'up' ? null : 'up';
	}
	function voteDown() {
		vote = vote === 'down' ? null : 'down';
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
		display: grid;
		place-items: center;
		height: 2.25rem;
		width: 2.25rem;
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
