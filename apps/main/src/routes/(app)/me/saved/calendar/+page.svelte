<script lang="ts">
	// Ref: japaconnect-engineering/sections/86-community-calendar.md
	// BUG-003: saved calendar entries dashboard page.
	import { t } from '$lib/i18n';
	import { goto } from '$app/navigation';
	import { CalendarDays, ExternalLink, AlertTriangle, Bookmark } from 'lucide-svelte';
	import EmptyState from '$lib/components/ui/EmptyState.svelte';
	import type { CalendarEntry, CalendarCategory } from '$lib/types';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();

	const title = $derived(t('calendar.saved_title', 'Saved Calendar Entries — JapaConnect'));

	const categoryLabels: Record<CalendarCategory, string> = {
		visa_window: t('calendar.cat_visa_window', 'Visa window'),
		embassy_closure: t('calendar.cat_embassy_closure', 'Embassy closure'),
		application_deadline: t('calendar.cat_application_deadline', 'Application deadline'),
		exam_window: t('calendar.cat_exam_window', 'Exam window'),
		immigration_office: t('calendar.cat_immigration_office', 'Immigration office'),
		other: t('calendar.cat_other', 'Other')
	};

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

	function isStale(entry: CalendarEntry): boolean {
		return entry.status === 'unverified';
	}
</script>

<svelte:head>
	<title>{title}</title>
	<meta name="description" content={t('calendar.saved_description', 'Calendar entries you have saved.')} />
	<meta name="robots" content="noindex" />
</svelte:head>

<div class="mx-auto w-full max-w-3xl">
	<div class="flex items-center justify-between border-b border-[var(--color-border)] px-4 py-3">
		<div class="min-w-0">
			<h1 class="flex items-center gap-2 text-lg font-semibold text-[var(--color-text)]">
				<Bookmark size={20} /> {t('calendar.saved_heading', 'Saved Calendar Entries')}
			</h1>
			<p class="mt-1 text-sm text-[var(--color-muted)]">
				{t('calendar.saved_description', 'Calendar entries you have saved.')}
			</p>
		</div>
		<a
			href="/calendar"
			class="inline-flex items-center gap-1.5 rounded-[var(--radius)] border border-[var(--color-border)] px-3 py-1.5 text-xs font-medium text-[var(--color-text)] transition-colors hover:bg-[var(--color-surface-subtle)]"
		>
			<CalendarDays size={14} /> {t('calendar.browse_all', 'Browse all')}
		</a>
	</div>

	<div class="p-4">
		{#if data.loadError}
			<EmptyState
				icon={CalendarDays}
				title={t('calendar.load_error', 'Could not load saved entries')}
				body={t('calendar.load_error_body', 'Please try again later.')}
			/>
		{:else if data.entries.length === 0}
			<EmptyState
				icon={Bookmark}
				title={t('calendar.saved_empty', 'No saved entries')}
				body={t('calendar.saved_empty_body', 'Save calendar entries to find them here later.')}
				action={{ label: t('calendar.browse_all', 'Browse calendar'), href: '/calendar' }}
			/>
		{:else}
			<div class="divide-y divide-[var(--color-border)] rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)]">
				{#each data.entries as entry (entry.id)}
					<div class="flex items-start gap-3 px-4 py-3">
						<a href="/calendar/{entry.id}" class="min-w-0 flex-1">
							<div class="flex items-start justify-between gap-2">
								<h2 class="text-sm font-semibold text-[var(--color-text)] hover:text-[var(--color-primary)]">
									{entry.title}
								</h2>
								<span class="shrink-0 rounded-full bg-[var(--color-surface-subtle)] px-2 py-0.5 text-xs font-medium text-[var(--color-text-subtle)]">
									{categoryLabels[entry.category as CalendarCategory] ?? entry.category}
								</span>
							</div>
							<div class="mt-1.5 flex flex-wrap items-center gap-x-3 gap-y-1 text-xs text-[var(--color-muted)]">
								<span class="inline-flex items-center gap-1">
									<CalendarDays size={12} />
									{fmtDate(entry.occurs_at)}
								</span>
							</div>
							{#if isStale(entry)}
								<p class="mt-1.5 inline-flex items-center gap-1 text-xs text-amber-600">
									<AlertTriangle size={12} />
									{t('calendar.stale_warning', 'Entry may be stale — verify with the official source.')}
								</p>
							{/if}
						</a>
						{#if entry.source_url}
							<a
								href={entry.source_url}
								target="_blank"
								rel="noopener noreferrer nofollow"
								class="inline-flex shrink-0 items-center gap-1 rounded-[var(--radius)] border border-[var(--color-border)] px-2 py-1 text-xs font-medium text-[var(--color-text)] transition-colors hover:bg-[var(--color-surface-subtle)]"
								aria-label={t('calendar.source', 'Official source')}
							>
								<ExternalLink size={12} /> {t('calendar.source', 'Source')}
							</a>
						{/if}
					</div>
				{/each}
			</div>
		{/if}
	</div>
</div>
