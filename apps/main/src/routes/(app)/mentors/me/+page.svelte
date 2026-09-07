<script lang="ts">
	// Ref: japaconnect-engineering/sections/102-mentorship-matching.md
	import { untrack } from 'svelte';
	import { t, i18n } from '$lib/i18n';
	import { invalidateAll, goto } from '$app/navigation';
	import { page } from '$app/state';
	import {
		GraduationCap,
		ArrowLeft,
		UserPlus,
		Clock,
		Check,
		X,
		Ban,
		CheckCircle2
	} from 'lucide-svelte';
	import { EmptyState } from '$lib/components/ui';
import { Tabs } from 'bits-ui';
	import {
		acceptMentorship,
		declineMentorship,
		cancelOutgoingMentorship,
		cancelMentorship,
		completeMentorship
	} from '$lib/api/hub';
	import { relativeTime } from '$lib/feed/time';
	import type { MentorProfile, MentorshipConnection } from '$lib/types';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();

	const profile = $derived<MentorProfile | null>(data.profile);
	const outgoing = $derived<MentorshipConnection[]>(data.outgoing);
	const incoming = $derived<MentorshipConnection[]>(data.incoming);

	// The current user's id, used to tell incoming (mentor) from outgoing (mentee)
	// and to decide which actions are available on a connection.
	const currentUser = $derived(page.data.user);
	const currentUserId = $derived(currentUser?.id ?? '');

	const isMentor = $derived(profile?.is_mentor === true);

	const title = $derived(t('mentors.me_title', 'My Mentorship — JapaConnect'));
	const description = $derived(
		t('mentors.me_description', 'Manage your mentor profile and mentorship connections.')
	);

	// Per-connection action state: track which connection id is currently being
	// mutated so we can disable just that one button. Seeded from data so a
	// re-run after invalidateAll resets cleanly.
	let pendingConnId = $state<string | null>(untrack(() => null));
	let actionError = $state<string | null>(untrack(() => null));

	// Reset transient action state whenever the server payload changes.
	$effect(() => {
		void data.outgoing;
		void data.incoming;
		pendingConnId = null;
		actionError = null;
	});

	function capacityPct(p: MentorProfile): number {
		const cap = p.mentor_capacity ?? 0;
		const active = p.active_mentee_count ?? 0;
		if (cap <= 0) return 0;
		return Math.min(100, Math.round((active / cap) * 100));
	}

	function capacityLabel(p: MentorProfile): string {
		const cap = p.mentor_capacity ?? 0;
		const active = p.active_mentee_count ?? 0;
		return `${active}/${cap}`;
	}

	function statusBadgeClass(status: string): string {
		const base = 'inline-flex items-center rounded-full px-2.5 py-0.5 text-xs font-medium';
		switch (status) {
			case 'active':
				return `${base} bg-green-500/10 text-green-600`;
			case 'pending':
				return `${base} bg-amber-500/10 text-amber-600`;
			case 'completed':
				return `${base} bg-[var(--color-primary)]/10 text-[var(--color-primary)]`;
			case 'cancelled':
				return `${base} bg-[var(--color-surface-subtle)] text-[var(--color-muted)]`;
			case 'declined':
				return `${base} bg-red-500/10 text-red-600`;
			default:
				return `${base} bg-[var(--color-surface-subtle)] text-[var(--color-text-subtle)]`;
		}
	}

	function statusLabel(status: string): string {
		switch (status) {
			case 'active':
				return t('mentors.status_active', 'Active');
			case 'pending':
				return t('mentors.status_pending', 'Pending');
			case 'completed':
				return t('mentors.status_completed', 'Completed');
			case 'cancelled':
				return t('mentors.status_cancelled', 'Cancelled');
			case 'declined':
				return t('mentors.status_declined', 'Declined');
			default:
				return status;
		}
	}

	/** Display name for the "other party" on a connection relative to the viewer. */
	function counterpartName(c: MentorshipConnection): string {
		// Outgoing: the viewer is the mentee, counterpart is the mentor.
		// Incoming: the viewer is the mentor, counterpart is the mentee.
		if (c.mentee_id === currentUserId) {
			return c.mentor_display_name || c.mentor_username || t('mentors.unknown_mentor', 'Mentor');
		}
		return c.mentee_display_name || c.mentee_username || t('mentors.unknown_mentee', 'Mentee');
	}

	function counterpartUsername(c: MentorshipConnection): string | null {
		if (c.mentee_id === currentUserId) return c.mentor_username ?? null;
		return c.mentee_username ?? null;
	}

	function counterpartHref(c: MentorshipConnection): string | null {
		const u = counterpartUsername(c);
		return u ? `/mentors/${u}` : null;
	}

	function isOutgoing(c: MentorshipConnection): boolean {
		return c.mentee_id === currentUserId;
	}

	function fmtTime(raw: string | null): string {
		if (!raw) return '';
		return relativeTime(raw, new Date(), i18n.locale);
	}

	async function runAction<T>(connId: string, fn: () => ReturnType<typeof acceptMentorship>) {
		if (pendingConnId) return null as T | null;
		pendingConnId = connId;
		actionError = null;
		const res = (await fn()) as { data: T | null; error: { message: string } | null };
		pendingConnId = null;
		if (res.error) {
			actionError =
				res.error.message || t('mentors.action_error', 'Action failed. Please try again.');
			return null;
		}
		await invalidateAll();
		return (res.data ?? null) as T | null;
	}

	async function acceptConnection(id: string) {
		const conn = await runAction<MentorshipConnection>(id, () => acceptMentorship(id));
		// §102 / ADR-110: backend auto-opens a DM thread on accept. Navigate
		// the user to that DM so they can start messaging immediately.
		if (conn) {
			if (conn.dm_conversation_id) {
				goto(`/messages?c=${encodeURIComponent(conn.dm_conversation_id)}`);
			} else {
				// Fallback: if the DM was not created, go to the messages list.
				goto('/messages');
			}
		}
	}

	async function rejectConnection(id: string) {
		// Mentor declines a pending request (pending → declined).
		await runAction<unknown>(id, () => declineMentorship(id));
	}

	async function cancelOutgoingPending(id: string) {
		// §102: pending outgoing requests are cancelled via
		// DELETE /me/mentorship/outgoing/:id (not PATCH /mentorship/:id/cancel,
		// which is for active connections only).
		await runAction<unknown>(id, () => cancelOutgoingMentorship(id));
	}

	async function cancelActiveConnection(id: string) {
		// §102: active connections are cancelled via
		// PATCH /mentorship/:id/cancel.
		await runAction<unknown>(id, () => cancelMentorship(id));
	}

	async function completeConnection(id: string) {
		await runAction<unknown>(id, () => completeMentorship(id));
	}
</script>

<svelte:head>
	<title>{title}</title>
	<meta name="description" content={description} />
	<meta property="og:title" content={title} />
	<meta property="og:description" content={description} />
	<meta property="og:type" content="website" />
</svelte:head>

<div class="mx-auto w-full max-w-full">
	<!-- Header -->
	<div class="flex items-center justify-between border-b border-[var(--color-border)] px-4 py-3">
		<h1 class="flex items-center gap-2 text-lg font-semibold text-[var(--color-text)]">
			<GraduationCap size={20} class="text-[var(--color-primary)]" />
			{t('mentors.me_heading', 'My Mentorship')}
		</h1>
		<a
			href="/mentors"
			class="inline-flex items-center gap-1.5 rounded-[var(--radius)] border border-[var(--color-border)] px-3 py-1.5 text-xs font-medium text-[var(--color-text)] transition-colors hover:bg-[var(--color-surface-subtle)]"
		>
			<ArrowLeft size={14} /> {t('mentors.browse', 'Browse mentors')}
		</a>
	</div>

	<div class="p-4 space-y-6">
		{#if data.loadError}
			<EmptyState
				icon={GraduationCap}
				title={t('mentors.me_load_error', 'Could not load your mentorship')}
				body={t('mentors.me_load_error_body', 'Please try again later.')}
				action={{ label: t('mentors.browse', 'Browse mentors'), href: '/mentors' }}
			/>
		{:else}
			<!-- Profile card -->
			{#if profile}
				<section
					class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-6"
				>
					<div class="flex items-start gap-4">
						{#if profile.avatar_url}
							<img
								src={profile.avatar_url}
								alt={profile.display_name ?? ''}
								class="h-16 w-16 rounded-full object-cover"
							/>
						{:else}
							<div
								class="grid h-16 w-16 place-items-center rounded-full bg-[var(--color-surface-subtle)] text-[var(--color-muted)]"
							>
								<GraduationCap size={28} />
							</div>
						{/if}
						<div class="min-w-0 flex-1">
							<h2 class="text-xl font-semibold text-[var(--color-text)]">
								{profile.display_name ?? t('mentors.me_profile', 'Your mentor profile')}
							</h2>
							<div class="mt-1 flex flex-wrap items-center gap-2 text-sm text-[var(--color-muted)]">
								{#if isMentor}
									<span
										class="inline-flex items-center rounded-full bg-green-500/10 px-2 py-0.5 text-[10px] font-medium text-green-600"
									>
										{t('mentors.me_active_mentor', 'Active mentor')}
									</span>
								{/if}
								{#if profile.mentor_since}
									<span class="inline-flex items-center gap-1">
										<Clock size={13} />
										{t('mentors.me_since', 'Since')} {fmtTime(profile.mentor_since)}
									</span>
								{/if}
							</div>
						</div>
					</div>

					{#if profile.mentor_bio}
						<div class="mt-4">
							<h3 class="text-sm font-medium text-[var(--color-text)]">
								{t('mentors.bio', 'About')}
							</h3>
							<p class="mt-1 text-sm text-[var(--color-text-subtle)]">{profile.mentor_bio}</p>
						</div>
					{/if}

					{#if profile.mentor_areas && profile.mentor_areas.length > 0}
						<div class="mt-4">
							<h3 class="text-sm font-medium text-[var(--color-text)]">
								{t('mentors.areas', 'Mentorship areas')}
							</h3>
							<div class="mt-1.5 flex flex-wrap gap-1.5">
								{#each profile.mentor_areas as area (area)}
									<span
										class="inline-flex items-center rounded-full border border-[var(--color-border)] bg-[var(--color-surface-subtle)] px-2.5 py-0.5 text-xs font-medium text-[var(--color-text-subtle)]"
									>
										{area}
									</span>
								{/each}
							</div>
						</div>
					{/if}

					<!-- Capacity -->
					{#if profile.mentor_capacity !== null && profile.mentor_capacity !== undefined}
						<div class="mt-4">
							<h3 class="text-sm font-medium text-[var(--color-text)]">
								{t('mentors.capacity', 'Mentees')}
							</h3>
							<div class="mt-1.5 flex items-center gap-2">
								<div
									class="h-2 w-40 overflow-hidden rounded-full bg-[var(--color-surface-subtle)]"
								>
									<div
										class="h-full rounded-full bg-[var(--color-primary)]"
										style="width: {capacityPct(profile)}%"
									></div>
								</div>
								<span class="text-xs text-[var(--color-muted)]">
									{capacityLabel(profile)}
								</span>
							</div>
						</div>
					{/if}
				</section>
			{:else}
				<!-- No profile yet: prompt to become a mentor -->
				<section
					class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-6 text-center"
				>
					<div
						class="mx-auto grid h-14 w-14 place-items-center rounded-full bg-[var(--color-surface-subtle)] text-[var(--color-muted)]"
					>
						<GraduationCap size={26} strokeWidth={1.5} />
					</div>
					<h2 class="mt-3 text-lg font-medium text-[var(--color-text)]">
						{t('mentors.me_become_title', 'Become a mentor')}
					</h2>
					<p class="mx-auto mt-1 max-w-sm text-sm text-[var(--color-text-subtle)]">
						{t(
							'mentors.me_become_body',
							'Share your experience and guide fellow diaspora members on their journey.'
						)}
					</p>
					<a
						href="/mentors"
						class="mt-4 inline-flex items-center gap-1.5 rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-2 text-sm font-medium text-[var(--color-primary-contrast)] transition-colors hover:bg-[var(--color-primary-hover)]"
					>
						<UserPlus size={16} /> {t('mentors.become_mentor', 'Become a Mentor')}
					</a>
				</section>
			{/if}

			{#if actionError}
				<p
					class="rounded-[var(--radius)] bg-red-500/10 px-3 py-2 text-sm text-red-500"
					role="alert"
				>
					{actionError}
				</p>
			{/if}

			<!-- Outgoing/incoming tabs -->
			<Tabs.Root value="outgoing">
				<Tabs.List
					class="mb-4 flex gap-1 overflow-x-auto overflow-y-hidden border-b border-[var(--color-border)]"
				>
					<Tabs.Trigger
						value="outgoing"
						class="px-3 py-2 text-sm font-medium text-[var(--color-text-subtle)] data-[state=active]:border-b-2 data-[state=active]:border-[var(--color-primary)] data-[state=active]:text-[var(--color-text)]"
					>
						{t('mentors.me_outgoing', 'Your requests')}
						{#if outgoing.length > 0}
							<span class="text-xs font-normal text-[var(--color-muted)]">({outgoing.length})</span>
						{/if}
					</Tabs.Trigger>
					{#if isMentor}
						<Tabs.Trigger
							value="incoming"
							class="px-3 py-2 text-sm font-medium text-[var(--color-text-subtle)] data-[state=active]:border-b-2 data-[state=active]:border-[var(--color-primary)] data-[state=active]:text-[var(--color-text)]"
						>
							{t('mentors.me_incoming', 'Requests to you')}
							{#if incoming.length > 0}
								<span class="text-xs font-normal text-[var(--color-muted)]">({incoming.length})</span>
							{/if}
						</Tabs.Trigger>
					{/if}
				</Tabs.List>

				<Tabs.Content value="outgoing">
					<section>
						<h2 class="mb-3 flex items-center gap-2 text-base font-semibold text-[var(--color-text)]">
							{t('mentors.me_outgoing', 'Your requests')}
							{#if outgoing.length > 0}
								<span class="text-xs font-normal text-[var(--color-muted)]">({outgoing.length})</span>
							{/if}
						</h2>

				{#if outgoing.length === 0}
					<EmptyState
						icon={GraduationCap}
						title={t('mentors.me_outgoing_empty', 'No mentorship requests yet')}
						body={t(
							'mentors.me_outgoing_empty_body',
							'Browse mentors and send a request to get started.'
						)}
						action={{ label: t('mentors.browse', 'Browse mentors'), href: '/mentors' }}
					/>
				{:else}
					<div class="space-y-3">
						{#each outgoing as conn (conn.id)}
							{@const href = counterpartHref(conn)}
							<div
								class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4"
							>
								<div class="flex flex-wrap items-start justify-between gap-3">
									<div class="min-w-0">
										<div class="flex flex-wrap items-center gap-2">
											{#if href}
												<a
													href={href}
													class="text-sm font-semibold text-[var(--color-text)] hover:text-[var(--color-primary)]"
												>
													{counterpartName(conn)}
												</a>
											{:else}
												<span class="text-sm font-semibold text-[var(--color-text)]">
													{counterpartName(conn)}
												</span>
											{/if}
											<span class={statusBadgeClass(conn.status)}>
												{statusLabel(conn.status)}
											</span>
										</div>
										{#if conn.goals}
											<p class="mt-1 line-clamp-3 text-sm text-[var(--color-text-subtle)]">
												{conn.goals}
											</p>
										{/if}
										<p class="mt-1.5 text-xs text-[var(--color-muted)]">
											{t('mentors.me_requested', 'Requested')} {fmtTime(conn.created_at)}
										</p>
									</div>

									<!-- Actions: mentee may cancel pending (DELETE) or active (PATCH cancel) -->
									<div class="flex shrink-0 flex-wrap items-center gap-2">
										{#if conn.status === 'pending'}
											<button
												type="button"
												disabled={pendingConnId === conn.id}
												onclick={() => cancelOutgoingPending(conn.id)}
												class="inline-flex items-center gap-1.5 rounded-[var(--radius)] border border-[var(--color-border)] px-3 py-1.5 text-xs font-medium text-[var(--color-text-subtle)] transition-colors hover:bg-[var(--color-surface-subtle)] disabled:opacity-50"
											>
												<Ban size={13} />
												{t('mentors.me_cancel', 'Cancel')}
											</button>
										{:else if conn.status === 'active'}
											<button
												type="button"
												disabled={pendingConnId === conn.id}
												onclick={() => cancelActiveConnection(conn.id)}
												class="inline-flex items-center gap-1.5 rounded-[var(--radius)] border border-[var(--color-border)] px-3 py-1.5 text-xs font-medium text-[var(--color-text-subtle)] transition-colors hover:bg-[var(--color-surface-subtle)] disabled:opacity-50"
											>
												<Ban size={13} />
												{t('mentors.me_cancel', 'Cancel')}
											</button>
										{/if}
										{#if conn.status === 'active'}
											<button
												type="button"
												disabled={pendingConnId === conn.id}
												onclick={() => completeConnection(conn.id)}
												class="inline-flex items-center gap-1.5 rounded-[var(--radius)] bg-[var(--color-primary)] px-3 py-1.5 text-xs font-medium text-[var(--color-primary-contrast)] transition-colors hover:bg-[var(--color-primary-hover)] disabled:opacity-50"
											>
												<CheckCircle2 size={13} />
												{t('mentors.me_complete', 'Mark completed')}
											</button>
										{/if}
									</div>
								</div>
							</div>
						{/each}
					</div>
				{/if}
					</section>
				</Tabs.Content>

				<!-- Incoming connections (only relevant if the user is a mentor) -->
				{#if isMentor}
					<Tabs.Content value="incoming">
						<section>
					<h2 class="mb-3 flex items-center gap-2 text-base font-semibold text-[var(--color-text)]">
						{t('mentors.me_incoming', 'Requests to you')}
						{#if incoming.length > 0}
							<span class="text-xs font-normal text-[var(--color-muted)]">({incoming.length})</span>
						{/if}
					</h2>

					{#if incoming.length === 0}
						<EmptyState
							icon={GraduationCap}
							title={t('mentors.me_incoming_empty', 'No incoming requests')}
							body={t(
								'mentors.me_incoming_empty_body',
								'When someone requests you as a mentor, it will appear here.'
							)}
						/>
					{:else}
						<div class="space-y-3">
							{#each incoming as conn (conn.id)}
								{@const href = counterpartHref(conn)}
								<div
									class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4"
								>
									<div class="flex flex-wrap items-start justify-between gap-3">
										<div class="min-w-0">
											<div class="flex flex-wrap items-center gap-2">
												{#if href}
													<a
														href={href}
														class="text-sm font-semibold text-[var(--color-text)] hover:text-[var(--color-primary)]"
													>
														{counterpartName(conn)}
													</a>
												{:else}
													<span class="text-sm font-semibold text-[var(--color-text)]">
														{counterpartName(conn)}
													</span>
												{/if}
												<span class={statusBadgeClass(conn.status)}>
													{statusLabel(conn.status)}
												</span>
											</div>
											{#if conn.goals}
												<p class="mt-1 line-clamp-3 text-sm text-[var(--color-text-subtle)]">
													{conn.goals}
												</p>
											{/if}
											<p class="mt-1.5 text-xs text-[var(--color-muted)]">
												{t('mentors.me_requested', 'Requested')} {fmtTime(conn.created_at)}
											</p>
										</div>

										<!-- Actions: mentor may accept/reject pending requests -->
										<div class="flex shrink-0 flex-wrap items-center gap-2">
											{#if conn.status === 'pending'}
												<button
													type="button"
													disabled={pendingConnId === conn.id}
													onclick={() => acceptConnection(conn.id)}
													class="inline-flex items-center gap-1.5 rounded-[var(--radius)] bg-[var(--color-primary)] px-3 py-1.5 text-xs font-medium text-[var(--color-primary-contrast)] transition-colors hover:bg-[var(--color-primary-hover)] disabled:opacity-50"
												>
													<Check size={13} />
													{t('mentors.me_accept', 'Accept')}
												</button>
												<button
													type="button"
													disabled={pendingConnId === conn.id}
													onclick={() => rejectConnection(conn.id)}
													class="inline-flex items-center gap-1.5 rounded-[var(--radius)] border border-[var(--color-border)] px-3 py-1.5 text-xs font-medium text-[var(--color-text-subtle)] transition-colors hover:bg-[var(--color-surface-subtle)] disabled:opacity-50"
												>
													<X size={13} />
													{t('mentors.me_reject', 'Reject')}
												</button>
											{:else if conn.status === 'active'}
												<button
													type="button"
													disabled={pendingConnId === conn.id}
													onclick={() => completeConnection(conn.id)}
													class="inline-flex items-center gap-1.5 rounded-[var(--radius)] bg-[var(--color-primary)] px-3 py-1.5 text-xs font-medium text-[var(--color-primary-contrast)] transition-colors hover:bg-[var(--color-primary-hover)] disabled:opacity-50"
												>
													<CheckCircle2 size={13} />
													{t('mentors.me_complete', 'Mark completed')}
												</button>
												<button
													type="button"
													disabled={pendingConnId === conn.id}
													onclick={() => cancelActiveConnection(conn.id)}
													class="inline-flex items-center gap-1.5 rounded-[var(--radius)] border border-[var(--color-border)] px-3 py-1.5 text-xs font-medium text-[var(--color-text-subtle)] transition-colors hover:bg-[var(--color-surface-subtle)] disabled:opacity-50"
												>
													<Ban size={13} />
													{t('mentors.me_cancel', 'Cancel')}
												</button>
											{/if}
										</div>
									</div>
								</div>
							{/each}
						</div>
					{/if}
				</section>
				</Tabs.Content>
			{/if}
		</Tabs.Root>
	{/if}
</div>
</div>
