<script lang="ts">
	import { ArrowUp, ArrowDown } from 'lucide-svelte';
	import { api } from '$lib/api/client';
	import { t } from '$lib/i18n';
	import type { ForumVoteEntity, ForumVote } from '$lib/types';

	interface Props {
		/** The entity being voted on. */
		entityType: ForumVoteEntity;
		entityId: string;
		/** Initial aggregate score from the server. */
		score: number;
		/** Optional initial vote direction for the current user (1, -1, or 0). */
		userVote?: 1 | -1 | 0;
		/** Compact (inline row) or vertical (thread gutter) layout. */
		layout?: 'vertical' | 'horizontal';
		/** Auth gating: only render interactive controls when true. */
		authed: boolean;
	}
	let {
		entityType,
		entityId,
		score: initialScore,
		userVote: initialVote = 0,
		layout = 'horizontal',
		authed
	}: Props = $props();

	let score = $state(0);
	let userVote = $state<1 | -1 | 0>(0);
	let pending = $state(false);

	// Sync from the server payload on mount and whenever it changes
	// (e.g. navigation between threads). Declared as a derived read so Svelte
	// does not warn about capturing the initial prop value only.
	$effect(() => {
		score = initialScore;
		userVote = initialVote;
	});

	async function vote(value: 1 | -1) {
		if (!authed || pending) return;
		// Optimistic: toggling the same direction retracts it (score reverts);
		// switching directions adjusts by 2.
		const prev = userVote;
		const next = value === prev ? 0 : value;
		const optimisticScore = score + (next - prev);
		score = optimisticScore;
		userVote = next;
		pending = true;

		const res =
			next === 0
				? await api.delete<unknown>(`/forum/${entityId}/vote?entity_type=${encodeURIComponent(entityType)}`)
				: await api.post<ForumVote>(`/forum/${entityId}/vote`, {
						entity_type: entityType,
						value: next
					});
		pending = false;

		if (res.error) {
			// Rollback on failure.
			score = initialScore;
			userVote = prev;
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
</script>

{#if layout === 'vertical'}
	<div class="flex w-7 flex-col items-center gap-0.5 pt-0.5 text-[var(--color-muted)]">
		<button
			type="button"
			onclick={() => vote(1)}
			disabled={!authed || pending}
			aria-pressed={upActive}
			aria-label={t('forum.upvote', 'Upvote')}
			class={upClass}
		>
			<ArrowUp size={18} />
		</button>
		<span class="text-xs font-semibold text-[var(--color-text-subtle)] [font-variant-numeric:tabular-nums]">
			{score}
		</span>
		<button
			type="button"
			onclick={() => vote(-1)}
			disabled={!authed || pending}
			aria-pressed={downActive}
			aria-label={t('forum.downvote', 'Downvote')}
			class={downClass}
		>
			<ArrowDown size={18} />
		</button>
	</div>
{:else}
	<div class="flex items-center gap-1 text-[var(--color-muted)] [font-variant-numeric:tabular-nums]">
		<button
			type="button"
			onclick={() => vote(1)}
			disabled={!authed || pending}
			aria-pressed={upActive}
			aria-label={t('forum.upvote', 'Upvote')}
			class={upClass}
		>
			<ArrowUp size={14} />
		</button>
		<span class="min-w-[1.25rem] text-center text-xs font-semibold text-[var(--color-text-subtle)]">
			{score}
		</span>
		<button
			type="button"
			onclick={() => vote(-1)}
			disabled={!authed || pending}
			aria-pressed={downActive}
			aria-label={t('forum.downvote', 'Downvote')}
			class={downClass}
		>
			<ArrowDown size={14} />
		</button>
	</div>
{/if}
