<script lang="ts">
	import { ThumbsUp, ThumbsDown, LogIn } from 'lucide-svelte';
	import { t } from '$lib/i18n';
	import { getCookie } from '$lib/api/client';
	import { page } from '$app/state';
	import { env } from '$env/dynamic/public';

	interface Props {
		/** Page ID for localStorage persistence of the vote. */
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
	}

	let { pageId, audience, slug, question = '', lastReviewedAt = null }: Props = $props();

	let vote = $state<'up' | 'down' | null>(null);
	let voteError = $state('');
	let submitting = $state(false);

	const storageKey = $derived(`jc:docs:vote:${pageId}`);

	// /docs/{audience}/{slug}/feedback requires a session (RequireAuth), so
	// guests get a sign-in link instead of vote buttons that can only 401.
	const user = $derived(page.data?.user ?? null);
	const mainOrigin = (env.PUBLIC_MAIN_ORIGIN ?? 'http://localhost:3000').replace(/\/$/, '');
	const loginHref = $derived(
		`${mainOrigin}/login?redirect=${encodeURIComponent(page.url.href)}`
	);

	// Load saved vote from localStorage after hydration (browser-only).
	// Uses $effect keyed on pageId so the vote resets on client-side
	// navigation (same route, different params) — onMount only runs once
	// per component instance and would leave the previous page's vote
	// displayed after a page change.
	$effect(() => {
		// Track pageId so the effect re-runs when it changes.
		const _pageId = pageId;
		vote = null;
		voteError = '';
		try {
			const saved = localStorage.getItem(`jc:docs:vote:${_pageId}`);
			if (saved === 'up' || saved === 'down') vote = saved;
		} catch {
			// localStorage unavailable (private browsing, etc.)
		}
	});

	// The backend only increments aggregate helpful/unhelpful counters — it
	// has no un-vote or vote-switch contract (internal/docs SubmitDocPageFeedback).
	// So once a vote is recorded the widget locks: clicking the active or
	// other button again is a no-op rather than lying about undo semantics.
	async function submitVote(v: 'up' | 'down') {
		// In-flight + already-voted guards — a double-click would double-count
		// the vote (DOC-011) and there is no undo endpoint.
		if (submitting || vote) return;
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
			// Persist only after the server confirms (BUG-HUNT-012: failed
			// votes were being persisted).
			vote = v;
			try {
				localStorage.setItem(storageKey, v);
			} catch {
				// localStorage unavailable
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
			<a href={loginHref} class="signin-link">
				<LogIn size={14} />
				{t('page.feedback_sign_in', 'Sign in to vote')}
			</a>
		{:else if vote}
			<span class="vote-thanks">
				{#if vote === 'up'}<ThumbsUp size={14} />{:else}<ThumbsDown size={14} />{/if}
				{t('page.feedback_recorded', 'Thanks for your feedback')}
			</span>
		{:else}
			<div class="helpful-buttons">
				<button
					type="button"
					class="vote-btn"
					onclick={() => submitVote('up')}
					disabled={submitting}
					aria-label={t('a11y.helpful_yes')}
				>
					<ThumbsUp size={16} />
				</button>
				<button
					type="button"
					class="vote-btn"
					onclick={() => submitVote('down')}
					disabled={submitting}
					aria-label={t('a11y.helpful_no')}
				>
					<ThumbsDown size={16} />
				</button>
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
	.vote-thanks {
		display: inline-flex;
		align-items: center;
		gap: 0.375rem;
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
