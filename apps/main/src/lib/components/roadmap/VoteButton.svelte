<script lang="ts">
	// Ref: japaconnect-engineering/sections/113-public-product-roadmap.md (Votes)
	// Canny-style vote pill: ▲ + count. Logged-in users toggle via
	// POST/DELETE /roadmap/items/{id}/vote (optimistic, rolled back on error);
	// guests are sent to login with a redirect back to the current page.
	// Non-votable statuses (pending/shipped/declined/archived — §113) render
	// the tally but the button is inert.
	import { untrack } from 'svelte';
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { ChevronUp } from 'lucide-svelte';
	import { api } from '$lib/api/client';
	import { t } from '$lib/i18n';
	import { roadmapVotableStatuses } from '$lib/enumOptions';
	import type { RoadmapBoardItem } from '$lib/types';

	interface Props {
		item: RoadmapBoardItem;
		authed: boolean;
	}
	let { item, authed }: Props = $props();

	let voted = $state(untrack(() => item.has_voted));
	let count = $state(untrack(() => item.vote_count));
	let pending = $state(false);

	// Re-sync when the SSR payload changes (invalidateAll after moderation).
	$effect(() => {
		voted = item.has_voted;
		count = item.vote_count;
	});

	const votable = $derived(roadmapVotableStatuses.includes(item.status));

	async function toggle() {
		if (!authed) {
			goto(`/login?redirect=${encodeURIComponent(page.url.pathname + page.url.search)}`);
			return;
		}
		if (!votable || pending) return;
		pending = true;
		const prevVoted = voted;
		const prevCount = count;
		const next = !voted;
		voted = next;
		count = prevCount + (next ? 1 : -1);

		const res = next
			? await api.post<{ vote_count: number }>(`/roadmap/items/${item.id}/vote`)
			: await api.delete<{ vote_count: number }>(`/roadmap/items/${item.id}/vote`);
		pending = false;

		if (res.error) {
			// A mid-page session expiry sends the member to login instead of a
			// silent rollback (§113: votes are logged-in only).
			if (res.status === 401 || res.error.code === 'unauthorized') {
				voted = prevVoted;
				count = prevCount;
				goto(`/login?redirect=${encodeURIComponent(page.url.pathname + page.url.search)}`);
				return;
			}
			// 409 codes mean the server state disagreed with the optimistic
			// guess — converge to the server's answer instead of a blind
			// rollback (already_voted ⇒ the tally already includes this vote;
			// not_voted ⇒ it does not).
			if (res.error.code === 'already_voted') {
				voted = true;
				count = prevCount;
			} else if (res.error.code === 'not_voted') {
				voted = false;
				count = Math.max(0, prevCount - 1);
			} else {
				// Roll back the optimistic update.
				voted = prevVoted;
				count = prevCount;
			}
			return;
		}
		if (res.data && typeof res.data.vote_count === 'number') {
			count = res.data.vote_count;
		}
	}
</script>

<button
	type="button"
	onclick={(e) => {
		e.preventDefault();
		e.stopPropagation();
		toggle();
	}}
	disabled={pending || !votable}
	aria-pressed={voted}
	aria-label={voted ? t('roadmap.unvote', 'Remove vote') : t('roadmap.vote', 'Vote')}
	title={!authed
		? t('roadmap.sign_in_to_vote', 'Sign in to vote')
		: !votable
			? t('roadmap.vote_closed', 'Voting is closed for this item')
			: voted
				? t('roadmap.unvote', 'Remove vote')
				: t('roadmap.vote', 'Vote')}
	class="inline-flex w-12 shrink-0 flex-col items-center justify-center gap-0.5 rounded-[var(--radius)] border px-2 py-1.5 text-xs font-semibold transition-colors [font-variant-numeric:tabular-nums] disabled:opacity-60 {voted
		? 'border-[var(--color-primary)] bg-[var(--color-primary)]/10 text-[var(--color-primary)]'
		: 'border-[var(--color-border)] bg-[var(--color-surface)] text-[var(--color-text-subtle)] hover:border-[var(--color-primary)]/50 hover:text-[var(--color-primary)]'}"
>
	<ChevronUp size={16} strokeWidth={2.5} />
	<span>{count}</span>
</button>
