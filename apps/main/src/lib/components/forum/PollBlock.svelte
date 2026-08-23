<script lang="ts">
	// Ref: japaconnect-engineering/sections/08-forum-qa-solved.md (polls)
	// Renders a forum poll with options and live vote counts. The poll data
	// is fetched by the parent (server load) and passed in. Voting is done
	// client-side via POST /forum/polls/{id}/vote (or /poll-options/{id}/vote).
	import { t } from '$lib/i18n';
	import { api } from '$lib/api/client';
	import { untrack } from 'svelte';
	import { BarChart3 } from 'lucide-svelte';
	import { requireAuthOrRedirect } from '$lib/auth/guest';
	import type { PollWithResults, PollResult } from '$lib/types';

	interface Props {
		pollData: PollWithResults;
	}
	let { pollData }: Props = $props();

	let poll = $state(untrack(() => pollData.poll));
	let results = $state<PollResult[]>(untrack(() => pollData.results));
	let selected = $state<Set<string>>(new Set());
	let voting = $state(false);
	let hasVoted = $state(false);
	let error = $state('');

	const totalVotes = $derived(results.reduce((sum, r) => sum + r.vote_count, 0));
	const isClosed = $derived(!!poll.close_at && new Date(poll.close_at) < new Date());

	function toggle(optionId: string) {
		if (hasVoted || isClosed) return;
		if (poll.is_multi_select) {
			if (selected.has(optionId)) selected.delete(optionId);
			else selected.add(optionId);
			selected = new Set(selected);
		} else {
			selected = new Set([optionId]);
		}
	}

	async function vote() {
		if (selected.size === 0 || voting) return;
		// Guests can't vote — redirect to login instead of hitting a 401.
		if (!(await requireAuthOrRedirect())) return;
		voting = true;
		error = '';
		try {
			// Vote on each selected option (supports multi-select).
			for (const optionId of selected) {
				await api.post(`/poll-options/${optionId}/vote`);
			}
			// Refresh results.
			const res = await api.get<PollWithResults>(`/forum/polls/${poll.id}`);
			if (res.data) {
				poll = res.data.poll;
				results = res.data.results;
			}
			hasVoted = true;
		} catch {
			error = t('forum.poll_error');
		} finally {
			voting = false;
		}
	}

	function pct(count: number): number {
		if (totalVotes === 0) return 0;
		return Math.round((count / totalVotes) * 100);
	}
</script>

<div class="rounded-[12px] border border-[var(--color-border)] bg-[var(--color-surface-subtle)] p-4">
	<div class="mb-3 flex items-center gap-2">
		<BarChart3 size={16} class="text-[var(--color-primary)]" />
		<h3 class="text-sm font-semibold text-[var(--color-text)]">{poll.question}</h3>
	</div>

	{#if isClosed}
		<p class="mb-2 text-xs text-[var(--color-muted)]">{t('forum.poll_closed')}</p>
	{/if}

	<ul class="space-y-2">
		{#each results as r (r.option_id)}
			<li>
				<button
					type="button"
					onclick={() => toggle(r.option_id)}
					disabled={hasVoted || isClosed}
					class="w-full text-left"
				>
					<div class="flex items-center gap-2">
						<span
							class="grid h-4 w-4 shrink-0 place-items-center border border-[var(--color-border)] rounded
							{poll.is_multi_select ? '' : 'rounded-full'}
							{selected.has(r.option_id) ? 'border-[var(--color-primary)] bg-[var(--color-primary)]' : ''}"
						>
							{#if selected.has(r.option_id)}
								<span class="h-2 w-2 rounded-full bg-white"></span>
							{/if}
						</span>
						<span class="flex-1 text-sm text-[var(--color-text)]">{r.label}</span>
						{#if hasVoted || isClosed}
							<span class="text-xs font-medium text-[var(--color-muted)]">{pct(r.vote_count)}%</span>
						{/if}
					</div>
					{#if hasVoted || isClosed}
						<div class="mt-1 h-1.5 overflow-hidden rounded-full bg-[var(--color-border)]">
							<div class="h-full rounded-full bg-[var(--color-primary)] transition-all" style="width: {pct(r.vote_count)}%"></div>
						</div>
					{/if}
				</button>
			</li>
		{/each}
	</ul>

	{#if error}
		<p class="mt-2 text-sm text-red-500" role="alert">{error}</p>
	{/if}

	{#if !hasVoted && !isClosed}
		<div class="mt-3 flex items-center justify-between">
			<span class="text-xs text-[var(--color-muted)]">
				{poll.is_multi_select ? t('forum.poll_multi') : t('forum.poll_single')}
			</span>
			<button
				type="button"
				onclick={vote}
				disabled={selected.size === 0 || voting}
				class="rounded-[var(--radius)] bg-[var(--color-primary)] px-3 py-1 text-xs font-medium text-white transition hover:opacity-90 disabled:opacity-50"
			>
				{voting ? '...' : t('forum.poll_vote')}
			</button>
		</div>
	{/if}

	{#if hasVoted || isClosed}
		<p class="mt-2 text-xs text-[var(--color-muted)]">
			{totalVotes} {t('forum.poll_votes')}
		</p>
	{/if}
</div>
