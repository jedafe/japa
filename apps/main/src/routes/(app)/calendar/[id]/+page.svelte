// Ref: japaconnect-engineering/sections/86-community-calendar.md
<script lang="ts">
	import { t } from '$lib/i18n';
	import { page } from '$app/state';
	import { api } from '$lib/api/client';
	import {
		ArrowLeft,
		CalendarDays,
			Repeat,
		ExternalLink,
		Bookmark,
		BookmarkCheck,
		AlertTriangle,
		BadgeCheck,
		Loader2
	} from 'lucide-svelte';
	import EmptyState from '$lib/components/ui/EmptyState.svelte';
	import type { CalendarEntry, CalendarCategory } from '$lib/types';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();

	const entry = $derived(data.entry);
	const authed = $derived(data.authed);

	const title = $derived(
		entry ? `${entry.title} — ${t('calendar.heading', 'Community Calendar')}` : t('calendar.heading', 'Community Calendar')
	);
	const description = $derived(entry?.description ?? entry?.title ?? '');

	const categoryLabels: Record<CalendarCategory, string> = {
		visa_window: t('calendar.cat_visa_window', 'Visa window'),
		embassy_closure: t('calendar.cat_embassy_closure', 'Embassy closure'),
		application_deadline: t('calendar.cat_application_deadline', 'Application deadline'),
		exam_window: t('calendar.cat_exam_window', 'Exam window'),
		immigration_office: t('calendar.cat_immigration_office', 'Immigration office'),
		other: t('calendar.cat_other', 'Other')
	};

	const statusLabels: Record<string, string> = {
		pending: t('calendar.status_pending', 'Pending'),
		verified: t('calendar.status_verified', 'Verified'),
		rejected: t('calendar.status_rejected', 'Rejected'),
		archived: t('calendar.status_archived', 'Archived')
	};

	function statusColor(status: string): string {
		if (status === 'verified') return 'bg-[var(--color-success)]/10 text-[var(--color-success)]';
		if (status === 'rejected') return 'bg-[var(--color-danger)]/10 text-[var(--color-danger)]';
		if (status === 'archived') return 'bg-[var(--color-surface-subtle)] text-[var(--color-text-subtle)]';
		return 'bg-amber-500/10 text-amber-600';
	}

	function fmtDate(iso: string): string {
		try {
			return new Date(iso).toLocaleString(undefined, {
				year: 'numeric',
				month: 'short',
				day: 'numeric',
				hour: '2-digit',
				minute: '2-digit'
			});
		} catch {
			return iso;
		}
	}

	function isStale(e: CalendarEntry): boolean {
		return false;
	}

	let saved = $state(false);
	let saving = $state(false);

	async function toggleSave() {
		if (!authed || !entry) return;
		saving = true;
		try {
			if (saved) {
				await api.delete(`/calendar/${entry.id}/save`);
				saved = false;
			} else {
				await api.post(`/calendar/${entry.id}/save`);
				saved = true;
			}
		} finally {
			saving = false;
		}
	}

	const jsonLd = $derived(
		entry
			? JSON.stringify({
					'@context': 'https://schema.org',
					'@type': 'Event',
					name: entry.title,
					description: entry.description ?? undefined,
					startDate: entry.occurs_at,
					...(entry.ends_at ? { endDate: entry.ends_at } : {}),
					url: page.url.href
				})
			: ''
	);
</script>

<svelte:head>
	<title>{title}</title>
	<meta name="description" content={description} />
	<meta property="og:title" content={entry?.title ?? title} />
	<meta property="og:description" content={description} />
	<meta property="og:type" content="article" />
	<meta property="og:url" content={page.url.href} />
	{#if jsonLd}
		{@html `<script type="application/ld+json">${jsonLd}</script>`}
	{/if}
</svelte:head>

<div class="mx-auto w-full max-w-3xl">
	{#if data.loadError || !entry}
		<div class="p-4">
			<EmptyState
				icon={CalendarDays}
				title={t('calendar.load_error', 'Could not load calendar entry')}
				body={t('calendar.load_error_body', 'Please try again later.')}
				action={{ label: t('calendar.back', 'Back to calendar'), href: '/calendar' }}
			/>
		</div>
	{:else}
		<div class="p-4">
			<a
				href="/calendar"
				class="inline-flex items-center gap-1 text-sm text-[var(--color-muted)] transition-colors hover:text-[var(--color-text)]"
			>
				<ArrowLeft size={16} /> {t('calendar.back', 'Back to calendar')}
			</a>
		</div>

		<article class="px-4 pb-8">
			<!-- Title + badges -->
			<div class="flex items-start gap-2">
				<h1 class="text-2xl font-bold text-[var(--color-text)]">{entry.title}</h1>
				{#if entry.status === 'verified'}
					<BadgeCheck size={22} class="mt-1 shrink-0 text-[var(--color-primary)]" />
				{/if}
			</div>

			<div class="mt-3 flex flex-wrap items-center gap-2">
				<span class="rounded-full bg-[var(--color-surface-subtle)] px-2 py-0.5 text-xs font-medium text-[var(--color-text-subtle)]">
					{categoryLabels[entry.category as CalendarCategory] ?? entry.category}
				</span>
				<span class="rounded-full px-2 py-0.5 text-xs font-medium {statusColor(entry.status ?? 'pending')}">
					{statusLabels[entry.status ?? 'pending'] ?? entry.status}
				</span>
				{#if entry.is_recurring}
					<span class="inline-flex items-center gap-1 rounded-full bg-[var(--color-primary)]/10 px-2 py-0.5 text-xs font-medium text-[var(--color-primary)]">
						<Repeat size={12} />
						{entry.recurrence_pattern ?? t('calendar.recurring', 'Recurring')}
					</span>
				{/if}
			</div>

			{#if isStale(entry) || entry.status === 'pending'}
				<p class="mt-3 inline-flex items-center gap-1.5 rounded-[var(--radius)] border border-amber-500/30 bg-amber-500/10 px-3 py-2 text-sm text-amber-600">
					<AlertTriangle size={14} />
					{isStale(entry)
						? t('calendar.stale_warning', 'Entry may be stale — verify with the official source.')
						: t('calendar.unverified_warning', 'Unverified — confirm with the official source.')}
				</p>
			{/if}

			<!-- Meta grid -->
			<dl class="mt-6 grid gap-3 sm:grid-cols-2">
				<div class="flex items-center gap-2 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface-subtle)] px-3 py-2">
					<CalendarDays size={16} class="shrink-0 text-[var(--color-muted)]" />
					<div class="min-w-0">
						<dt class="text-xs text-[var(--color-muted)]">{t('calendar.occurs_at', 'Starts')}</dt>
						<dd class="truncate text-sm font-medium text-[var(--color-text)]">{fmtDate(entry.occurs_at)}</dd>
					</div>
				</div>
				{#if entry.ends_at}
					<div class="flex items-center gap-2 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface-subtle)] px-3 py-2">
						<CalendarDays size={16} class="shrink-0 text-[var(--color-muted)]" />
						<div class="min-w-0">
							<dt class="text-xs text-[var(--color-muted)]">{t('calendar.ends_at', 'Ends')}</dt>
							<dd class="truncate text-sm font-medium text-[var(--color-text)]">{fmtDate(entry.ends_at)}</dd>
						</div>
					</div>
				{/if}

			</dl>

			<!-- Description -->
			{#if entry.description}
				<div class="mt-6 whitespace-pre-wrap text-sm leading-relaxed text-[var(--color-text)]">
					{entry.description}
				</div>
			{/if}

			<!-- Actions -->
			<div class="mt-6 flex flex-wrap items-center gap-3 border-t border-[var(--color-border)] pt-4">
				<a
					href={entry.source_url}
					target="_blank"
					rel="noopener noreferrer nofollow"
					class="inline-flex items-center gap-2 rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-2 text-sm font-medium text-[var(--color-primary-contrast)] transition-colors hover:bg-[var(--color-primary-hover)]"
				>
					<ExternalLink size={16} />
					{t('calendar.view_source', 'View official source')}
				</a>
				{#if authed}
					<button
						type="button"
						onclick={toggleSave}
						disabled={saving}
						class="inline-flex items-center gap-2 rounded-[var(--radius)] border border-[var(--color-border)] px-4 py-2 text-sm font-medium text-[var(--color-text)] transition-colors hover:bg-[var(--color-surface-subtle)] disabled:opacity-50"
					>
						{#if saving}
							<Loader2 size={16} class="animate-spin" />
						{:else if saved}
							<BookmarkCheck size={16} class="text-[var(--color-primary)]" />
						{:else}
							<Bookmark size={16} />
						{/if}
						{saved ? t('common.saved', 'Saved') : t('common.save', 'Save')}
					</button>
				{/if}
			</div>
		</article>
	{/if}
</div>
