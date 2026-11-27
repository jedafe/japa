<script lang="ts">
	/**
	 * InviteModal — member-to-member space invite (§05 hub-redesign,
	 * custom spaces only). Search picks a member via /search?type=user;
	 * POST /spaces/{slug}/invites files the pending invite + notification.
	 * Duplicate invites surface the backend's 409 invite_pending inline.
	 */
	import { onDestroy } from 'svelte';
	import { Modal } from '#lib/components/ui/index.js';
	import { t } from '#lib/i18n/index.js';
	import { UserPlus, Search, Loader2, Check } from 'lucide-svelte';
	import { api } from '#lib/api/client.js';
	import { searchHits } from '#lib/api/create.js';
	import VerifiedBadge from '#lib/components/ui/VerifiedBadge.svelte';
	import { remoteImageUrl } from '#lib/utils/remoteImage.js';
	import type { SearchHit, Space } from '#lib/types/index.js';

	interface Props {
		open?: boolean;
		space: Space;
	}
	let { open = $bindable(false), space }: Props = $props();

	let query = $state('');
	let hits = $state<SearchHit[]>([]);
	let searching = $state(false);
	let searchTimer: ReturnType<typeof setTimeout> | null = null;
	// Invited ids stick for the session so the row flips to "Invited" and
	// can't be double-posted (the backend would 409 a second attempt).
	let invitedIds = $state<Set<string>>(new Set());
	let invitingId = $state<string | null>(null);
	let errorMsg = $state('');

	function onQueryInput(e: Event) {
		query = (e.currentTarget as HTMLInputElement).value;
		if (searchTimer) clearTimeout(searchTimer);
		if (query.trim().length < 2) {
			hits = [];
			return;
		}
		searching = true;
		searchTimer = setTimeout(async () => {
			hits = await searchHits('users', query.trim());
			searching = false;
		}, 250);
	}

	// A pending debounce must not fire after the modal unmounts.
	onDestroy(() => {
		if (searchTimer) clearTimeout(searchTimer);
	});

	async function invite(hit: SearchHit) {
		if (invitingId || invitedIds.has(hit.id)) return;
		invitingId = hit.id;
		errorMsg = '';
		const res = await api.post<{ id: string }>(`/spaces/${space.slug}/invites`, {
			user_id: hit.id
		});
		if (res.error) {
			errorMsg = res.error.code === 'invite_pending'
				? t('space.invite.pending', '{name} already has a pending invite.').replace(
						'{name}',
						hit.label
					)
				: res.error.message;
		} else {
			invitedIds = new Set([...invitedIds, hit.id]);
		}
		invitingId = null;
	}
</script>

<Modal
	bind:open
	icon={UserPlus}
	title={t('space.invite.title', 'Invite to {space}').replace('{space}', space.name)}
	overlayClass="backdrop-blur-sm"
	size="max-w-[480px]"
>
	<div class="space-y-3">
		<div class="flex items-center gap-2 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 focus-within:border-[var(--color-primary)]">
			<Search size={16} class="text-[var(--color-muted)]" />
			<input
				type="search"
				value={query}
				oninput={onQueryInput}
				placeholder={t('space.invite.search_placeholder', 'Search members to invite…')}
				class="w-full appearance-none border-0 bg-transparent text-sm text-[var(--color-text)] outline-none ring-0 placeholder:text-[var(--color-muted)]"
			/>
			{#if searching}
				<Loader2 size={14} class="animate-spin text-[var(--color-muted)]" />
			{/if}
		</div>

		{#if errorMsg}
			<p class="rounded-lg bg-red-50 px-3 py-2 text-xs text-red-700 dark:bg-red-950/30 dark:text-red-400" role="alert">{errorMsg}</p>
		{/if}

		{#if hits.length > 0}
			<ul class="max-h-72 divide-y divide-[var(--color-border)] overflow-y-auto rounded-[var(--radius)] border border-[var(--color-border)]">
				{#each hits as hit (hit.id)}
					<li class="flex items-center gap-3 px-3 py-2.5">
						{#if hit.avatar_url}
							<img src={remoteImageUrl(hit.avatar_url)} alt="" class="h-9 w-9 shrink-0 rounded-full object-cover" />
						{:else}
							<span class="grid h-9 w-9 shrink-0 place-items-center rounded-full bg-[var(--color-surface-subtle)] text-sm font-medium text-[var(--color-text-subtle)]">
								{hit.label.charAt(0).toUpperCase()}
							</span>
						{/if}
						<span class="min-w-0 flex-1">
							<span class="block truncate text-sm font-medium text-[var(--color-text)]">{hit.label}</span>
							{#if hit.handle}
								<span class="block truncate text-xs text-[var(--color-muted)]">@{hit.handle}</span>
							{/if}
						</span>
						<button
							type="button"
							disabled={invitingId === hit.id || invitedIds.has(hit.id)}
							onclick={() => invite(hit)}
							class="inline-flex shrink-0 items-center gap-1 rounded-full px-3 py-1 text-xs font-medium transition-colors disabled:opacity-60 {invitedIds.has(hit.id)
								? 'border border-[var(--color-border)] text-[var(--color-muted)]'
								: 'bg-[var(--color-primary)] text-[var(--color-primary-contrast)] hover:bg-[var(--color-primary-hover)]'}"
						>
							{#if invitingId === hit.id}
								<Loader2 size={12} class="animate-spin" />
							{:else if invitedIds.has(hit.id)}
								<Check size={12} /> {t('space.invite.sent', 'Invited')}
							{:else}
								<UserPlus size={12} /> {t('space.invite.action', 'Invite')}
							{/if}
						</button>
					</li>
				{/each}
			</ul>
		{:else if query.trim().length >= 2 && !searching}
			<p class="py-6 text-center text-sm text-[var(--color-muted)]">{t('space.invite.no_results', 'No members found')}</p>
		{/if}
	</div>
</Modal>
