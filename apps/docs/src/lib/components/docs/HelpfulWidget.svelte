<script lang="ts">
	import { untrack } from 'svelte';
	import { ThumbsUp, ThumbsDown, LogIn } from 'lucide-svelte';
	import { t } from '$lib/i18n';
	import { getCookie } from '$lib/api/client';
	import { page } from '$app/state';
	import { env } from '$env/dynamic/public';
	import type { DocFeedback } from '$lib/types';

	interface Props {
		/** Page ID — kept for the keyed reset effect. */
		pageId: string;
		/** Docs audience (e.g. 'users'). Used to POST feedback. */
		audience: string;
		/** Docs page slug. Used to POST feedback. */
		slug: string;
		/** Optional override for the prompt label (sidebar widget `question` prop). */
		question?: string;
		/** ISO timestamp of last review, or null. */
		lastReviewedAt?: string | null;
		/** Reviewer user id (UUID) — accepted for API shape parity but not
		 *  displayed: the backend sends an internal id, not a display name. */
		lastReviewedBy?: string | null;
		/** Server-fetched feedback state — counts + the caller's own vote
		 *  (§44 amendment 2, doc_page_votes). */
		feedback?: DocFeedback | null;
	}

	let {
		pageId,
		audience,
		slug,
		question = '',
		lastReviewedAt = null,
		feedback = null
	}: Props = $props();

	let vote = $state<'up' | 'down' | null>(untrack(() => feedback?.my_vote ?? null));
	let helpfulYes = $state(untrack(() => feedback?.helpful_yes ?? 0));
	let helpfulNo = $state(untrack(() => feedback?.helpful_no ?? 0));
	let voteError = $state('');
	let submitting = $state(false);

	// /docs/{audience}/{slug}/feedback POST requires a session (RequireAuth);
	// guests get a sign-in link instead of vote buttons that can only 401.
	const user = $derived(page.data?.user ?? null);
	const mainOrigin = (env.PUBLIC_MAIN_ORIGIN ?? 'http://localhost:3000').replace(/\/$/, '');
	const loginHref = $derived(
		`${mainOrigin}/login?redirect=${encodeURIComponent(page.url.href)}`
	);

	// Re-sync from fresh server props on client-side navigation (same route,
	// different params) — keyed on pageId so the previous page's vote doesn't
	// bleed through.
	$effect(() => {
		const _pageId = pageId;
		vote = feedback?.my_vote ?? null;
		helpfulYes = feedback?.helpful_yes ?? 0;
		helpfulNo = feedback?.helpful_no ?? 0;
		voteError = '';
	});

	// §44 amendment 2: the backend dedupes votes per (page, user) — repeating
	// the same vote is a no-op, switching moves the counter. The widget stays
	// interactive after a vote so users can correct it; the response carries
	// the authoritative counters.
	async function submitVote(v: 'up' | 'down') {
		if (submitting || v === vote) return;
		voteError = '';

		submitting = true;
		try {
			const res = await fetch('/api/docs/feedback', {
				method: 'POST',
				headers: {
					'content-type': 'application/json',
					'x-csrf-token': getCookie('csrf_token') ?? ''
				},
				body: JSON.stringify({ audience, slug, vote: v })
			});
			if (!res.ok) {
				// SvelteKit endpoint errors serialize as {message} (thrown error())
				// or {error:{message}} (backend envelope forwarded verbatim) —
				// read both so "Sign in to leave feedback" reaches the user.
				const json = await res.json().catch(() => ({}));
				throw new Error(json?.error?.message ?? json?.message ?? 'Feedback failed');
			}
			const json = (await res.json()) as { data?: DocFeedback };
			if (json.data) {
				vote = json.data.my_vote ?? v;
				helpfulYes = json.data.helpful_yes;
				helpfulNo = json.data.helpful_no;
			} else {
				vote = v;
			}
		} catch (err) {
			voteError = err instanceof Error ? err.message : 'Feedback failed';
		} finally {
			submitting = false;
		}
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
	{#if voteError}
		<p class="vote-error">{voteError}</p>
	{/if}
	<div class="helpful-row">
		<span class="helpful-label">{question || t('a11y.helpful')}</span>
		{#if !user}
			<span class="counts">
				{#if helpfulYes + helpfulNo > 0}
					<span class="count"><ThumbsUp size={12} />{helpfulYes}</span>
					<span class="count"><ThumbsDown size={12} />{helpfulNo}</span>
				{/if}
			</span>
			<a href={loginHref} class="signin-link">
				<LogIn size={14} />
				{t('page.feedback_sign_in', 'Sign in to vote')}
			</a>
		{:else}
			<div class="helpful-buttons" class:voted={vote !== null}>
				<button
					type="button"
					class="vote-btn"
					class:active={vote === 'up'}
					onclick={() => submitVote('up')}
					disabled={submitting}
					aria-pressed={vote === 'up'}
					aria-label={t('a11y.helpful_yes')}
				>
					<ThumbsUp size={16} />
					{#if helpfulYes > 0}<span class="btn-count">{helpfulYes}</span>{/if}
				</button>
				<button
					type="button"
					class="vote-btn"
					class:active={vote === 'down'}
					onclick={() => submitVote('down')}
					disabled={submitting}
					aria-pressed={vote === 'down'}
					aria-label={t('a11y.helpful_no')}
				>
					<ThumbsDown size={16} />
					{#if helpfulNo > 0}<span class="btn-count">{helpfulNo}</span>{/if}
				</button>
				{#if vote}
					<span class="vote-thanks">{t('page.feedback_recorded', 'Thanks for your feedback')}</span>
				{/if}
			</div>
		{/if}
	</div>
	<p class="reviewed">
		{reviewedLabel}
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
		flex-wrap: wrap;
	}
	.helpful-label {
		font-size: 0.875rem;
		font-weight: 500;
		color: var(--color-text);
	}
	.helpful-buttons { display: flex; align-items: center; gap: 0.5rem; }
	.vote-btn {
		display: inline-flex;
		align-items: center;
		gap: 0.375rem;
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
	.btn-count {
		font-size: 0.75rem;
		font-weight: 600;
		font-variant-numeric: tabular-nums;
	}
	.counts { display: inline-flex; gap: 0.625rem; }
	.count {
		display: inline-flex;
		align-items: center;
		gap: 0.25rem;
		font-size: 0.75rem;
		color: var(--color-muted);
		font-variant-numeric: tabular-nums;
	}
	.vote-thanks {
		font-size: 0.8125rem;
		font-weight: 500;
		color: var(--color-primary);
	}
	.signin-link {
		display: inline-flex;
		align-items: center;
		gap: 0.375rem;
		font-size: 0.8125rem;
		font-weight: 500;
		color: var(--color-primary);
		text-decoration: none;
		padding: 0.375rem 0.625rem;
		border: 1px solid var(--color-border);
		border-radius: var(--radius);
	}
	.signin-link:hover { border-color: var(--color-primary); }
	.reviewed {
		margin: 0.75rem 0 0;
		font-size: 0.75rem;
		color: var(--color-muted);
	}
	.vote-error {
		margin: 0 0 0.75rem;
		font-size: 0.75rem;
		color: var(--color-danger);
	}
</style>
