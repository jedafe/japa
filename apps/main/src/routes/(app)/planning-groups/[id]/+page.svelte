<script lang="ts">
	import HintIcon from '$lib/components/ui/HintIcon.svelte';
	import { untrack } from 'svelte';
	// Ref: japaconnect-engineering/sections/82-japa-planning-groups.md
	import { t } from '$lib/i18n';
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { api } from '$lib/api/client';
	import {
		Users,
		Plane,
		Calendar,
		Lock,
		Unlock,
		ArrowLeft,
		Plus,
		Check,
		Trash2,
		Pencil,
		LogIn,
		LogOut,
		Target,
		ClipboardList
	} from 'lucide-svelte';
	import { EmptyState } from '$lib/components/ui';
	import type { PlanningMilestone, PlanningCheckin } from '$lib/types';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();

	const group = $derived(data.group);
	const members = $derived(data.members);
	const isOwner = $derived(
		!!group && !!data.currentUserId && group.owner_id === data.currentUserId
	);
	// Resolve the owner's username from the members list (owner_id is a user UUID).
	const ownerName = $derived(
		members?.find((m) => m.user_id === group?.owner_id)?.username ?? group?.owner_id ?? ''
	);

	const title = $derived(
		group ? t('planning_groups.detail_title', '{name} — {site}').replace('{name}', group.name) : t('planning_groups.title', 'Planning Groups — {site}')
	);
	const description = $derived(group?.description ?? '');

	// --- Join / Leave ---
	let joinLoading = $state(false);
	let joinError = $state('');
	let isMember = $state(
		untrack(() => !!members?.some((m) => m.user_id === data.currentUserId))
	);
	let memberCount = $state(untrack(() => members?.length ?? 0));

	$effect(() => {
		isMember = !!members?.some((m) => m.user_id === data.currentUserId);
		memberCount = members?.length ?? 0;
	});

	async function joinGroup() {
		if (!group) return;
		joinLoading = true;
		joinError = '';
		const res = await api.post<{ ok: boolean }>(`/planning-groups/${group.id}/join`);
		if (res.data) {
			isMember = true;
			memberCount += 1;
		} else {
			joinError = res.error?.message ?? t('planning_groups.errors.join_failed', 'Failed to join group.');
		}
		joinLoading = false;
	}

	async function leaveGroup() {
		if (!group) return;
		joinLoading = true;
		joinError = '';
		const res = await api.delete<{ ok: boolean }>(`/planning-groups/${group.id}/leave`);
		if (res.data) {
			isMember = false;
			memberCount = Math.max(0, memberCount - 1);
		} else {
			joinError = res.error?.message ?? t('planning_groups.errors.leave_failed', 'Failed to leave group.');
		}
		joinLoading = false;
	}

	// --- Delete (owner) ---
	let deleteLoading = $state(false);
	async function deleteGroup() {
		if (!group) return;
		if (!confirm(t('planning_groups.confirm_delete', 'Delete this planning group? This cannot be undone.'))) return;
		deleteLoading = true;
		const res = await api.delete<{ ok: boolean }>(`/planning-groups/${group.id}`);
		if (res.data) {
			await goto('/planning-groups');
		} else {
			joinError = res.error?.message ?? t('planning_groups.errors.delete_failed', 'Failed to delete group.');
		}
		deleteLoading = false;
	}

	// --- Milestones ---
	let milestones = $state<PlanningMilestone[]>(untrack(() => data.milestones));
	let milestoneTitle = $state('');
	let milestoneLoading = $state(false);
	let milestoneError = $state('');

	async function addMilestone(e: SubmitEvent) {
		e.preventDefault();
		if (!group || !milestoneTitle.trim()) return;
		milestoneLoading = true;
		milestoneError = '';
		const res = await api.post<PlanningMilestone>(`/planning-groups/${group.id}/milestones`, {
			title: milestoneTitle.trim()
		});
		if (res.data) {
			milestones = [...milestones, res.data];
			milestoneTitle = '';
		} else {
			milestoneError = res.error?.message ?? t('planning_groups.errors.milestone_failed', 'Failed to add milestone.');
		}
		milestoneLoading = false;
	}

	async function toggleMilestone(m: PlanningMilestone) {
		if (!group) return;
		const newCompleted = !m.completed;
		milestones = milestones.map((x) =>
			x.id === m.id ? { ...x, completed: newCompleted, completed_at: newCompleted ? new Date().toISOString() : undefined } : x
		);
		const res = await api.patch<PlanningMilestone>(
			`/planning-groups/${group.id}/milestones/${m.id}`,
			{ completed: newCompleted }
		);
		if (!res.data) {
			// Revert on failure
			milestones = milestones.map((x) =>
				x.id === m.id ? { ...x, completed: m.completed, completed_at: m.completed_at } : x
			);
		}
	}

	// --- Check-ins ---
	let checkins = $state<PlanningCheckin[]>(untrack(() => data.checkins));
	let checkinNote = $state('');
	let checkinLoading = $state(false);
	let checkinError = $state('');
	const MAX_CHECKIN = 200;
	const checkinRemaining = $derived(MAX_CHECKIN - checkinNote.length);

	async function submitCheckin(e: SubmitEvent) {
		e.preventDefault();
		if (!group || !checkinNote.trim()) return;
		checkinLoading = true;
		checkinError = '';
		const res = await api.post<PlanningCheckin>(`/planning-groups/${group.id}/checkins`, {
			note: checkinNote.trim()
		});
		if (res.data) {
			checkins = [{ ...res.data, username: members?.find((m) => m.user_id === data.currentUserId)?.username ?? undefined }, ...checkins];
			checkinNote = '';
		} else {
			checkinError = res.error?.message ?? t('planning_groups.errors.checkin_failed', 'Failed to submit check-in.');
		}
		checkinLoading = false;
	}

	const inputClass =
		'w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)] outline-none focus:border-[var(--color-primary)] focus:ring-2 focus:ring-[var(--color-primary)]/20';
</script>

<svelte:head>
	<title>{title}</title>
	<meta name="description" content={description} />
	<meta property="og:title" content={title} />
	<meta property="og:description" content={description} />
	<meta property="og:type" content="website" />
	<meta property="og:url" content={page.url.href} />
</svelte:head>

<div class="mx-auto w-full max-w-3xl">
	{#if data.loadError || !group}
		<div class="p-4">
			<p class="py-12 text-center text-[var(--color-text-subtle)]">
				{t('planning_groups.load_error', 'Failed to load planning group. Please try again later.')}
			</p>
		</div>
	{:else}
		<!-- Header -->
		<div class="border-b border-[var(--color-border)] px-4 py-4">
			<a
				href="/planning-groups"
				class="mb-2 inline-flex items-center gap-1.5 text-sm text-[var(--color-text-subtle)] transition-colors hover:text-[var(--color-text)]"
			>
				<ArrowLeft class="rtl:-scale-x-100" size={14} /> {t('planning_groups.back', 'All planning groups')}
			</a>
			<div class="flex items-start justify-between gap-3">
				<div class="min-w-0 flex-1">
					<div class="flex items-center gap-2">
						<h1 class="truncate text-xl font-semibold text-[var(--color-text)]">{group.name}</h1>
						<span
							class="inline-flex shrink-0 items-center gap-1 rounded-[var(--radius)] bg-[var(--color-surface-subtle)] px-1.5 py-0.5 text-xs font-medium text-[var(--color-text-subtle)]"
						>
							{#if group.is_open_join}
								<Unlock size={12} /> {t('planning_groups.open_join', 'Open')}
							{:else}
								<Lock size={12} /> {t('planning_groups.closed_join', 'Invite only')}
							{/if}
						</span>
					</div>
					{#if group.description}
						<p class="mt-2 text-sm text-[var(--color-text-subtle)]">{group.description}</p>
					{/if}
				</div>
			</div>

			<!-- Meta row -->
			<div class="mt-3 flex flex-wrap items-center gap-x-4 gap-y-1 text-xs text-[var(--color-muted)]">
				{#if group.visa_category}
					<span class="inline-flex items-center gap-1"><Plane size={12} /> {group.visa_category}</span>
				{/if}
				{#if group.target_departure_window}
					<span class="inline-flex items-center gap-1"><Calendar size={12} /> {group.target_departure_window}</span>
				{/if}
				<span class="inline-flex items-center gap-1"><Users size={12} /> {memberCount}/{group.max_members ?? 0}</span>
				<span>{t('planning_groups.owner', 'by {name}').replace('{name}', ownerName)}</span>
			</div>

			{#if joinError}
				<p class="mt-3 rounded-[var(--radius)] bg-red-500/10 px-3 py-2 text-sm text-red-500" role="alert">
					{joinError}
				</p>
			{/if}

			<!-- Actions -->
			<div class="mt-4 flex flex-wrap items-center gap-2">
				{#if data.authed}
					{#if isOwner}
						<a
							href="/planning-groups/{group.id}/edit"
							class="inline-flex items-center gap-1.5 rounded-[var(--radius)] border border-[var(--color-border)] px-3 py-1.5 text-sm font-medium text-[var(--color-text)] transition-colors hover:bg-[var(--color-surface-subtle)]"
						>
							<Pencil size={14} /> {t('planning_groups.edit', 'Edit')}
						</a>
						<button
							type="button"
							onclick={deleteGroup}
							disabled={deleteLoading}
							class="inline-flex items-center gap-1.5 rounded-[var(--radius)] border border-red-500/30 px-3 py-1.5 text-sm font-medium text-red-500 transition-colors hover:bg-red-500/10 disabled:opacity-50"
						>
							<Trash2 size={14} /> {t('planning_groups.delete', 'Delete')}
						</button>
					{:else if isMember}
						<button
							type="button"
							onclick={leaveGroup}
							disabled={joinLoading}
							class="inline-flex items-center gap-1.5 rounded-[var(--radius)] border border-[var(--color-border)] px-3 py-1.5 text-sm font-medium text-[var(--color-text)] transition-colors hover:bg-[var(--color-surface-subtle)] disabled:opacity-50"
						>
							<LogOut class="rtl:-scale-x-100" size={14} /> {joinLoading ? '...' : t('planning_groups.leave', 'Leave')}
						</button>
					{:else}
						<button
							type="button"
							onclick={joinGroup}
							disabled={joinLoading || memberCount >= (group.max_members ?? 0)}
							class="inline-flex items-center gap-1.5 rounded-[var(--radius)] bg-[var(--color-primary)] px-3 py-1.5 text-sm font-medium text-[var(--color-primary-contrast)] transition-colors hover:bg-[var(--color-primary-hover)] disabled:opacity-50"
						>
							<LogIn class="rtl:-scale-x-100" size={14} /> {joinLoading ? '...' : t('planning_groups.join', 'Join')}
						</button>
					{/if}
				{:else}
					<a
						href="/login?redirect=/planning-groups/{group.id}"
						class="inline-flex items-center gap-1.5 rounded-[var(--radius)] bg-[var(--color-primary)] px-3 py-1.5 text-sm font-medium text-[var(--color-primary-contrast)] transition-colors hover:bg-[var(--color-primary-hover)]"
					>
						<LogIn class="rtl:-scale-x-100" size={14} /> {t('planning_groups.join', 'Join')}
					</a>
				{/if}
			</div>
		</div>

		<div class="grid grid-cols-1 gap-4 p-4 lg:grid-cols-2">
			<!-- Milestones -->
			<section class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
				<h2 class="flex items-center gap-2 text-sm font-semibold text-[var(--color-text)]">
					<Target size={16} /> {t('planning_groups.milestones', 'Milestones')}
				</h2>

				{#if data.authed && isMember}
					<form onsubmit={addMilestone} class="mt-3 flex gap-2" novalidate>
						<input
							type="text"
							value={milestoneTitle}
							oninput={(e) => (milestoneTitle = e.currentTarget.value)}
							placeholder={t('planning_groups.milestone_placeholder', 'Add a milestone…')}
							class={inputClass}
						/>
							<HintIcon text={t('planning_groups.milestone_hint', 'A shared deadline or step for the group.')} />
						<button
							type="submit"
							disabled={milestoneLoading || !milestoneTitle.trim()}
							class="inline-flex shrink-0 items-center gap-1 rounded-[var(--radius)] bg-[var(--color-primary)] px-3 py-2 text-sm font-medium text-[var(--color-primary-contrast)] transition-colors hover:bg-[var(--color-primary-hover)] disabled:opacity-50"
						>
							<Plus size={14} /> {t('common.add', 'Add')}
						</button>
					</form>
					{#if milestoneError}
						<p class="mt-2 text-sm text-red-500" role="alert">{milestoneError}</p>
					{/if}
				{/if}

				{#if milestones.length === 0}
					<p class="mt-4 text-sm text-[var(--color-muted)]">
						{t('planning_groups.no_milestones', 'No milestones yet.')}
					</p>
				{:else}
					<ul class="mt-3 space-y-2">
						{#each milestones as m (m.id)}
							<li class="flex items-center gap-2 text-sm text-[var(--color-text)]">
								{#if data.authed && isMember}
									<button
										type="button"
										onclick={() => toggleMilestone(m)}
										class="grid h-5 w-5 shrink-0 place-items-center rounded border {m.completed
											? 'border-[var(--color-primary)] bg-[var(--color-primary)] text-[var(--color-primary-contrast)]'
											: 'border-[var(--color-border)] text-transparent hover:border-[var(--color-primary)]'}"
										aria-label={t('planning_groups.toggle_milestone', 'Toggle milestone')}
									>
										<Check size={12} />
									</button>
								{:else}
									<span
										class="grid h-5 w-5 shrink-0 place-items-center rounded border {m.completed
											? 'border-[var(--color-primary)] bg-[var(--color-primary)] text-[var(--color-primary-contrast)]'
											: 'border-[var(--color-border)] text-transparent'}"
									>
										<Check size={12} />
									</span>
								{/if}
								<span class={m.completed ? 'line-through text-[var(--color-muted)]' : ''}>
									{m.title}
								</span>
							</li>
						{/each}
					</ul>
				{/if}
			</section>

			<!-- Weekly check-ins -->
			<section class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
				<h2 class="flex items-center gap-2 text-sm font-semibold text-[var(--color-text)]">
					<ClipboardList size={16} /> {t('planning_groups.checkins', 'This week\'s check-ins')}
				</h2>

				{#if data.authed && isMember}
					<form onsubmit={submitCheckin} class="mt-3 space-y-2" novalidate>
						<textarea
							value={checkinNote}
							oninput={(e) => (checkinNote = e.currentTarget.value)}
							rows={3}
							maxlength={MAX_CHECKIN}
							placeholder={t('planning_groups.checkin_placeholder', 'Share your progress this week (max 200 chars)…')}
							class={inputClass}
						></textarea>
						<div class="flex items-center justify-between">
							<span class="text-xs text-[var(--color-muted)]">{checkinRemaining}</span>
							<button
								type="submit"
								disabled={checkinLoading || !checkinNote.trim()}
								class="inline-flex items-center gap-1 rounded-[var(--radius)] bg-[var(--color-primary)] px-3 py-1.5 text-sm font-medium text-[var(--color-primary-contrast)] transition-colors hover:bg-[var(--color-primary-hover)] disabled:opacity-50"
							>
								<Plus size={14} /> {t('planning_groups.submit_checkin', 'Submit')}
							</button>
						</div>
					</form>
					{#if checkinError}
						<p class="mt-2 text-sm text-red-500" role="alert">{checkinError}</p>
					{/if}
				{/if}

				{#if checkins.length === 0}
					<p class="mt-4 text-sm text-[var(--color-muted)]">
						{t('planning_groups.no_checkins', 'No check-ins this week yet.')}
					</p>
				{:else}
					<ul class="mt-3 space-y-3">
						{#each checkins as c (c.id)}
							<li class="rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface-subtle)] p-3">
								<div class="flex items-center justify-between text-xs text-[var(--color-muted)]">
									<span class="font-medium text-[var(--color-text-subtle)]">{c.username ?? t('planning_groups.member', 'Member')}</span>
									<span>{new Date(c.created_at).toLocaleDateString()}</span>
								</div>
								<p class="mt-1 text-sm text-[var(--color-text)]">{c.note}</p>
							</li>
						{/each}
					</ul>
				{/if}
			</section>
		</div>

		<!-- Members -->
		{#if members && members.length > 0}
			<div class="px-4 pb-8">
				<section class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
					<h2 class="flex items-center gap-2 text-sm font-semibold text-[var(--color-text)]">
						<Users size={16} /> {t('planning_groups.members', 'Members')} ({members.length})
					</h2>
					<ul class="mt-3 flex flex-wrap gap-2">
						{#each members as m (m.user_id)}
							<li class="inline-flex items-center gap-1.5 rounded-full border border-[var(--color-border)] bg-[var(--color-surface-subtle)] px-3 py-1 text-xs text-[var(--color-text)]">
								{m.username}
								{#if m.user_id === group.owner_id}
									<span class="font-medium text-[var(--color-primary)]">★</span>
								{/if}
							</li>
						{/each}
					</ul>
				</section>
			</div>
		{/if}
	{/if}
</div>
