<script lang="ts">
	// Ref: japaconnect-engineering/sections/08-forum-qa-solved.md (polls)
	// Renders a forum poll with options and live vote counts. The poll data
	// is fetched by the parent (server load) and passed in. Voting is done
	// client-side via POST /forum/polls/{id}/vote; changing or retracting a
	// vote uses POST /poll-options/{id}/vote / DELETE /poll-options/{id}/vote.
	// Live updates are pushed over the WebSocket room for the thread's space.
	import { t } from '$lib/i18n';
	import { api } from '$lib/api/client';
	import { untrack } from 'svelte';
	import { BarChart3, Loader2 } from 'lucide-svelte';
	import { requireAuthOrRedirect } from '$lib/auth/guest';
	import { realtime } from '$lib/realtime/realtime.svelte';
	import type { PollWithResults, PollResult } from '$lib/types';

	interface Props {
		pollData: PollWithResults;
		/** Optional space ID — when set, the block subscribes to live poll results. */
		spaceId?: string;
	}
	let { pollData, spaceId }: Props = $props();

	let poll = $state(untrack(() => pollData.poll));
	let results = $state<PollResult[]>(untrack(() => pollData.results));
	let selected = $state<Set<string>>(untrack(() => new Set(pollData.user_votes ?? [])));
	let voting = $state(false);
	let hasVoted = $state(untrack(() => (pollData.user_votes ?? []).length > 0));
	let error = $state('');

	// Reseed when the server payload changes (invalidateAll after creating
	// the poll, editing the thread, etc.) — the one-time untrack seed would
	// otherwise keep showing stale results and a stale voted state.
	$effect(() => {
		const next = pollData;
		untrack(() => {
			poll = next.poll;
			results = next.results;
			selected = new Set(next.user_votes ?? []);
			hasVoted = (next.user_votes ?? []).length > 0;
		});
	});

	// Subscribe to the space room and listen for live poll result updates.
	// Capture spaceId once per run — it's a lazily-evaluated prop getter, so
	// re-reading it in the teardown could hit a detached parent (undefined)
	// or unsubscribe a room that was never subscribed if the id changed.
	$effect(() => {
		const id = spaceId;
		if (!id) return;
		realtime.subscribeSpace(id);
		return () => realtime.unsubscribeSpace(id);
	});

	$effect(() => {
		const handler = (ev: { poll_id: string; results: PollResult[] }) => {
			if (ev.poll_id !== poll.id) return;
			results = ev.results.map((r) => ({
				option_id: r.option_id,
				label: r.label,
				position: r.position,
				vote_count: r.vote_count
			}));
		};
		realtime.onPollUpdated(handler);
		return () => realtime.offPollUpdated(handler);
	});

	const totalVotes = $derived(results.reduce((sum, r) => sum + r.vote_count, 0));
	const isClosed = $derived(!!poll.close_at && new Date(poll.close_at) < new Date());

	async function refreshResults() {
		const res = await api.get<PollWithResults>(`/forum/polls/${poll.id}`);
		if (res.data) {
			poll = res.data.poll;
			results = res.data.results;
			selected = new Set(res.data.user_votes ?? []);
			hasVoted = (res.data.user_votes ?? []).length > 0;
		}
	}

	// Pre-vote selection toggle (stage a choice, then press Vote).
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
			// Vote on each selected option (supports multi-select). Each
			// call is checked individually so a partial failure surfaces
			// instead of silently refreshing stale results.
			const ids = [...selected];
			let failures = 0;
			for (const optionId of ids) {
				const res = await api.post(`/forum/polls/${poll.id}/vote`, { option_id: optionId });
				if (res.error) failures += 1;
			}
			if (failures === 0) {
				await refreshResults();
			} else if (failures === ids.length) {
				error = t('forum.poll_error', 'Failed to submit your vote. Please try again.');
			} else {
				await refreshResults();
				error = t('forum.poll_partial_error', 'Some votes were not recorded. Please try again.');
			}
			voting = false;
		} catch {
			error = t('forum.poll_error', 'Failed to submit your vote. Please try again.');
			voting = false;
		}
	}

	// Post-vote interactions: clicking the option you already voted on
	// retracts that vote (DELETE /poll-options/{id}/vote); clicking a
	// different option changes your vote (single-select polls replace the
	// prior vote server-side; multi-select adds another). Ref: Section 08.
	async function changeVote(optionId: string) {
		if (voting || isClosed) return;
		if (!(await requireAuthOrRedirect())) return;
		voting = true;
		error = '';
		try {
			const res = selected.has(optionId)
				? await api.delete(`/poll-options/${optionId}/vote`)
				: await api.post(`/forum/polls/${poll.id}/vote`, { option_id: optionId });
			if (res.error) {
				error = t('forum.poll_error', 'Failed to submit your vote. Please try again.');
				voting = false;
			} else {
				await refreshResults();
				voting = false;
			}
		} catch {
			error = t('forum.poll_error', 'Failed to submit your vote. Please try again.');
			voting = false;
		}
	}

	function onOptionClick(optionId: string) {
		if (isClosed || voting) return;
		if (hasVoted) changeVote(optionId);
		else toggle(optionId);
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
		{#if voting}
			<Loader2 size={14} class="animate-spin text-[var(--color-muted)]" />
		{/if}
	</div>

	{#if isClosed}
		<p class="mb-2 text-xs text-[var(--color-muted)]">{t('forum.poll_closed')}</p>
	{/if}

	<ul class="space-y-2">
		{#each results as r (r.option_id)}
			<li>
				<button
					type="button"
					onclick={() => onOptionClick(r.option_id)}
					disabled={isClosed || voting}
					class="w-full text-start"
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
			{#if hasVoted && !isClosed}
				· {t('forum.poll_change_hint', 'Click an option to change or retract your vote')}
			{/if}
		</p>
	{/if}
</div>
