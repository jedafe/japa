<script lang="ts">
	// Ref: japaconnect-engineering/sections/22-admin-panel.md (Appeals)
	// Lists the user's submitted appeals (GET /me/appeals) with status pills
	// and links to the linked staff ticket (the conversation thread).
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { t } from '$lib/i18n';
	import EmptyState from '$lib/components/ui/EmptyState.svelte';
	import PaginationControls from '$lib/components/ui/PaginationControls.svelte';
	import AppealButton from '$lib/components/appeals/AppealButton.svelte';
	import { ticketHref } from '$lib/tickets';
	import { AlertTriangle, Scale, Ticket } from 'lucide-svelte';
	import type { AppealEligibleItem, AppealListItem } from '$lib/types';

	let { data } = $props();
	const appeals = $derived<AppealListItem[]>(data.appeals);
	const eligible = $derived<AppealEligibleItem[]>(data.eligible);
	const userStatus = $derived<string>(data.userStatus);
	const restricted = $derived(userStatus !== 'active');
	const pageNum = $derived(data.page);
	const hasMore = $derived(data.hasMore);
	const showPagination = $derived(pageNum > 1 || hasMore);

	const STATUS_KEY: Record<AppealListItem['status'], string> = {
		pending: 'appeals.status.pending',
		upheld: 'appeals.status.upheld',
		overturn: 'appeals.status.overturn',
		dismissed: 'appeals.status.dismissed'
	};

	const ENTITY_KEY: Record<string, string> = {
		content: 'appeals.entity.content',
		content_removal: 'appeals.entity.content_removal',
		comment: 'appeals.entity.comment',
		user: 'appeals.entity.user',
		ban: 'appeals.entity.ban',
		suspension: 'appeals.entity.suspension',
		warning: 'appeals.entity.warning',
		operation_ban: 'appeals.entity.operation_ban',
		service_claim: 'appeals.entity.service_claim',
		org_claim: 'appeals.entity.org_claim',
		video_channel_claim: 'appeals.entity.video_channel_claim',
		media: 'appeals.entity.media',
		wiki_revision: 'appeals.entity.wiki_revision',
		content_report: 'appeals.entity.content_report',
		scam_report: 'appeals.entity.scam_report',
		academy_instructor: 'appeals.entity.academy_instructor',
		academy_track: 'appeals.entity.academy_track',
		review: 'appeals.entity.review',
		mutual_aid_post: 'appeals.entity.mutual_aid_post',
		video_source: 'appeals.entity.video_source',
		roadmap_item: 'appeals.entity.roadmap_item',
		organization: 'appeals.entity.organization',
		space: 'appeals.entity.space',
		affiliate_member: 'appeals.entity.affiliate_member',
		referral: 'appeals.entity.referral',
		withdrawal: 'appeals.entity.withdrawal',
		ad_campaign: 'appeals.entity.ad_campaign',
		content_review: 'appeals.entity.content_review',
		community_org: 'appeals.entity.community_org',
		community_org_claim: 'appeals.entity.community_org_claim'
	};

	function statusClass(status: AppealListItem['status']): string {
		switch (status) {
			case 'pending':
				return 'bg-[var(--color-warning)]/10 text-[var(--color-warning)]';
			case 'overturn':
				return 'bg-[var(--color-success)]/10 text-[var(--color-success)]';
			case 'upheld':
			case 'dismissed':
				return 'bg-[var(--color-surface-subtle)] text-[var(--color-muted)]';
		}
	}

	function changePage(next: number) {
		const url = new URL(page.url);
		url.searchParams.set('page', String(next));
		goto(url.toString());
	}
</script>

<svelte:head>
	<title>{t('appeals.list_title', 'My appeals — {site}')}</title>
</svelte:head>

<section class="space-y-4">
	<header class="flex flex-wrap items-center justify-between gap-3">
		<h1 class="flex items-center gap-2 text-lg font-medium text-[var(--color-text)]">
			<Scale size={18} aria-hidden="true" />
			{t('appeals.list_title', 'My appeals')}
		</h1>
		<a
			href="/dashboard/tickets"
			class="inline-flex items-center gap-1.5 rounded-[var(--radius)] border border-[var(--color-border)] px-3 py-1.5 text-sm font-medium text-[var(--color-text)] transition hover:bg-[var(--color-surface-subtle)]"
		>
			<Ticket class="h-4 w-4" aria-hidden="true" />
			{t('appeals.view_tickets', 'Support tickets')}
		</a>
	</header>

	{#if restricted}
		<div
			class="flex items-start gap-3 rounded-[var(--radius-card)] border border-[var(--color-danger)]/40 bg-[var(--color-danger)]/10 p-4"
			role="alert"
		>
			<AlertTriangle size={18} class="mt-0.5 shrink-0 text-[var(--color-danger)]" aria-hidden="true" />
			<div class="min-w-0">
				<p class="text-sm font-medium text-[var(--color-text)]">
					{t(`appeals.restricted.${userStatus}`, 'Your account is restricted')}
				</p>
				<p class="mt-0.5 text-sm text-[var(--color-muted)]">
					{t(
						'appeals.restricted_body',
						'Most of {site} is unavailable while this restriction is active. You can contest it below — a staff member will review your appeal.'
					)}
				</p>
			</div>
		</div>
	{/if}

	{#if eligible.length > 0}
		<section
			class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4"
		>
			<h2 class="text-sm font-medium text-[var(--color-text)]">
				{t('appeals.eligible_title', 'Decisions you can appeal')}
			</h2>
			<ul class="mt-3 space-y-2">
				{#each eligible as item (item.entity_type + item.entity_id)}
					<li class="flex items-center justify-between gap-3">
						<div class="min-w-0">
							<p class="text-sm text-[var(--color-text)]">{item.label}</p>
							{#if item.detail}
								<p class="mt-0.5 line-clamp-1 text-xs text-[var(--color-muted)]">{item.detail}</p>
							{/if}
						</div>
						<AppealButton
							entityType={item.entity_type}
							entityId={item.entity_id}
							class="shrink-0 !px-3 !py-1.5"
						/>
					</li>
				{/each}
			</ul>
		</section>
	{/if}

	{#if appeals.length === 0}
		<EmptyState
			icon={Scale}
			title={t('appeals.empty_title', 'No appeals')}
			body={t(
				'appeals.empty_body',
				'Appeals you submit against rejected claims or moderation decisions will appear here.'
			)}
		/>
	{:else}
		<ul class="space-y-2">
			{#each appeals as appeal (appeal.id)}
				<li
					class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4"
				>
					<div class="flex items-start justify-between gap-3">
						<div class="min-w-0 flex-1">
							<p class="text-sm font-medium text-[var(--color-text)]">
								{appeal.label ||
									t(ENTITY_KEY[appeal.entity_type] ?? 'appeals.entity.other', appeal.entity_type)}
							</p>
							{#if appeal.label}
								<p class="mt-0.5 text-xs text-[var(--color-muted)]">
									{t(
										ENTITY_KEY[appeal.entity_type] ?? 'appeals.entity.other',
										appeal.entity_type
									)}
								</p>
							{/if}
							<p class="mt-0.5 line-clamp-2 text-xs text-[var(--color-muted)]">{appeal.reason}</p>
							{#if appeal.resolution_note}
								<p class="mt-1 text-xs text-[var(--color-text-subtle)]">{appeal.resolution_note}</p>
							{/if}
						</div>
						<span
							class="shrink-0 rounded-full px-2 py-0.5 text-xs font-medium {statusClass(
								appeal.status
							)}"
						>
							{t(STATUS_KEY[appeal.status])}
						</span>
					</div>
					<div class="mt-2 flex items-center gap-3 text-xs text-[var(--color-muted)]">
						<span>#{appeal.id.slice(0, 8)}</span>
						<span>·</span>
						<time>{new Date(appeal.created_at).toLocaleDateString()}</time>
						{#if appeal.ticket_id}
							<span>·</span>
							<a
								href={ticketHref({ id: appeal.ticket_id, ticket_number: appeal.ticket_number })}
								class="text-[var(--color-primary)] hover:underline"
							>
								{t('appeals.view_ticket', 'View ticket')}
							</a>
						{/if}
					</div>
				</li>
			{/each}
		</ul>
		{#if showPagination}
			<div class="flex justify-center pt-2">
				<PaginationControls page={pageNum} {hasMore} onPageChange={changePage} />
			</div>
		{/if}
	{/if}
</section>
