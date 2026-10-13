<script lang="ts">
	import { ArrowBigUp, ArrowBigDown } from 'lucide-svelte';
	import { api } from '$lib/api/client';
	import { t } from '$lib/i18n';
	import type { ForumThread, ForumVoteEntity, ForumVote } from '$lib/types';

	interface Props {
		/** The entity being voted on. */
		entityType: ForumVoteEntity;
		/** The thread/reply id the vote API expects. Optional for thread votes
		 * rendered on feed cards, where the item id is the backing content id —
		 * pass `slug` instead and the thread id is resolved lazily on the first
		 * vote via GET /forum/{slug}. */
		entityId?: string;
		/** Thread slug — used to lazily resolve the thread id when `entityId` is
		 * not a forum thread id (feed cards). Only applies to entityType 'thread'. */
		slug?: string;
		/** Initial aggregate score from the server. */
		score: number;
		/** Optional initial vote direction for the current user (1, -1, or 0). */
		userVote?: 1 | -1 | 0;
		/** `vertical` = stacked rail · `horizontal` = compact inline row ·
		 * `rail` = ReactionRail-style round buttons (single-post action bar). */
		layout?: 'vertical' | 'horizontal' | 'rail';
		/** Auth gating: only render interactive controls when true. */
		authed: boolean;
	}
	let {
		entityType,
		entityId,
		slug,
		score: initialScore,
		userVote: initialVote = 0,
		layout = 'horizontal',
		authed
	}: Props = $props();

	// Score and the viewer's vote track the server props directly — derived
	// state stays in sync across navigations without an imperative $effect.
	let optimisticDelta = $state(0);
	let optimisticVote = $state<1 | -1 | 0 | null>(null);
	let score = $derived(initialScore + optimisticDelta);
	let userVote = $derived<1 | -1 | 0>(optimisticVote ?? initialVote);
	let pending = $state(false);

	let resolvedId = $state('');
	let resolving: Promise<string | null> | null = null;

	// Feed cards carry the backing contents.id, but the vote API keys on
	// forum_threads.id. Resolve the real thread id (and hydrate the viewer's
	// existing vote) on the first interaction so no extra request is made for
	// cards that are never voted on.
	async function resolveEntityId(): Promise<string | null> {
		if (entityId) return entityId;
		if (resolvedId) return resolvedId;
		if (entityType !== 'thread' || !slug) return null;
		resolving ??= (async () => {
			const res = await api.getEnvelope<ForumThread>(`/forum/${slug}`);
			if (!res.data?.id) return null;
			resolvedId = res.data.id;
			optimisticDelta = res.data.vote_score - initialScore;
			const uv = (res.envelope?.['user_votes'] as Record<string, number> | undefined)?.[
				res.data.id
			];
			if (uv === 1 || uv === -1) optimisticVote = uv;
			return resolvedId;
		})();
		return resolving;
	}

	async function vote(value: 1 | -1) {
		if (!authed || pending) return;
		pending = true;
		const id = await resolveEntityId();
		if (!id) {
			pending = false;
			return;
		}
		// Optimistic: toggling the same direction retracts it (score reverts);
		// switching directions adjusts by 2.
		const prevVote = userVote;
		const prevDelta = optimisticDelta;
		const next = value === prevVote ? 0 : value;
		optimisticVote = next;
		optimisticDelta = prevDelta + (next - prevVote);

		const res =
			next === 0
				? await api.delete<unknown>(`/forum/${id}/vote?entity_type=${encodeURIComponent(entityType)}`)
				: await api.post<ForumVote>(`/forum/${id}/vote`, {
						entity_type: entityType,
						value: next
					});
		pending = false;

		if (res.error) {
			// Rollback on failure.
			optimisticVote = prevVote === initialVote ? null : prevVote;
			optimisticDelta = prevDelta;
		}
	}

	const upActive = $derived(userVote === 1);
	const downActive = $derived(userVote === -1);
	const baseBtn =
		'grid place-items-center transition-colors disabled:opacity-50 disabled:cursor-not-allowed';
	const upClass = $derived(
		`${baseBtn} ${upActive ? 'text-[var(--color-success)]' : 'text-[var(--color-muted)] hover:text-[var(--color-success)]'}`
	);
	const downClass = $derived(
		`${baseBtn} ${downActive ? 'text-[var(--color-danger)]' : 'text-[var(--color-muted)] hover:text-[var(--color-danger)]'}`
	);
	// ReactionRail-style round cells (blog single action bar parity).
	const railUpClass = $derived(
		`${baseBtn} h-9 w-9 rounded-full ${upActive ? 'bg-[var(--color-success)]/10 text-[var(--color-success)]' : 'text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)] hover:text-[var(--color-success)]'}`
	);
	const railDownClass = $derived(
		`${baseBtn} h-9 w-9 rounded-full ${downActive ? 'bg-[var(--color-danger)]/10 text-[var(--color-danger)]' : 'text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)] hover:text-[var(--color-danger)]'}`
	);
</script>

{#if layout === 'vertical'}
	<div class="flex w-7 flex-col items-center gap-0.5 pt-0.5 text-[var(--color-muted)]">
		<button
			type="button"
			onclick={(e) => { e.stopPropagation(); vote(1); }}
			onkeydown={(e) => e.stopPropagation()}
			disabled={!authed || pending}
			aria-pressed={upActive}
			aria-label={t('forum.upvote', 'Upvote')}
			class={upClass}
		>
			<ArrowBigUp size={18} fill={upActive ? 'currentColor' : 'none'} />
		</button>
		<span class="text-xs font-semibold text-[var(--color-text-subtle)] [font-variant-numeric:tabular-nums]">
			{score}
		</span>
		<button
			type="button"
			onclick={(e) => { e.stopPropagation(); vote(-1); }}
			onkeydown={(e) => e.stopPropagation()}
			disabled={!authed || pending}
			aria-pressed={downActive}
			aria-label={t('forum.downvote', 'Downvote')}
			class={downClass}
		>
			<ArrowBigDown size={18} fill={downActive ? 'currentColor' : 'none'} />
		</button>
	</div>
{:else if layout === 'rail'}
	<div class="flex items-center text-[var(--color-muted)] [font-variant-numeric:tabular-nums]">
		<button
			type="button"
			onclick={(e) => { e.stopPropagation(); vote(1); }}
			onkeydown={(e) => e.stopPropagation()}
			disabled={!authed || pending}
			aria-pressed={upActive}
			aria-label={t('forum.upvote', 'Upvote')}
			class={railUpClass}
		>
			<ArrowBigUp size={18} fill={upActive ? 'currentColor' : 'none'} />
		</button>
		<span class="min-w-[1.5rem] text-center text-[11px] font-medium text-[var(--color-text-subtle)]">
			{score}
		</span>
		<button
			type="button"
			onclick={(e) => { e.stopPropagation(); vote(-1); }}
			onkeydown={(e) => e.stopPropagation()}
			disabled={!authed || pending}
			aria-pressed={downActive}
			aria-label={t('forum.downvote', 'Downvote')}
			class={railDownClass}
		>
			<ArrowBigDown size={18} fill={downActive ? 'currentColor' : 'none'} />
		</button>
	</div>
{:else}
	<div class="flex items-center gap-1 text-[var(--color-muted)] [font-variant-numeric:tabular-nums]">
		<button
			type="button"
			onclick={(e) => { e.stopPropagation(); vote(1); }}
			onkeydown={(e) => e.stopPropagation()}
			disabled={!authed || pending}
			aria-pressed={upActive}
			aria-label={t('forum.upvote', 'Upvote')}
			class={upClass}
		>
			<ArrowBigUp size={18} fill={upActive ? 'currentColor' : 'none'} />
		</button>
		<span class="min-w-[1.25rem] text-center text-xs font-semibold text-[var(--color-text-subtle)]">
			{score}
		</span>
		<button
			type="button"
			onclick={(e) => { e.stopPropagation(); vote(-1); }}
			onkeydown={(e) => e.stopPropagation()}
			disabled={!authed || pending}
			aria-pressed={downActive}
			aria-label={t('forum.downvote', 'Downvote')}
			class={downClass}
		>
			<ArrowBigDown size={18} fill={downActive ? 'currentColor' : 'none'} />
		</button>
	</div>
{/if}
