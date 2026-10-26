<script lang="ts">
	// /dashboard/reports — the user's own submitted content reports and their
	// current status (pending / reviewing / actioned / dismissed). Read-only;
	// users cannot edit or withdraw reports after submission.
	//
	// Ref: japaconnect-engineering/sections/51-content-reports-flagging.md
	import { t } from '$lib/i18n';
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { Flag, FileText } from 'lucide-svelte';
	import { EmptyState, RelativeTime, PaginationControls } from '$lib/components/ui';
	import AutocompleteSelect from '$lib/components/ui/AutocompleteSelect.svelte';
	import AppealButton from '$lib/components/appeals/AppealButton.svelte';
	import { contentHref } from '$lib/utils/contentLink';
	import type { MyReport } from '$lib/types';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();

	const title = $derived(t('my_reports.title', 'My Reports — {site}'));
	const reports = $derived<MyReport[]>(data.reports);
	const pageNum = $derived(data.page);
	const pageSize = $derived(data.pageSize);
	const showPagination = $derived(pageNum > 1 || data.hasMore);

	const pageSizeOptions = [
		{ value: '10', label: '10' },
		{ value: '20', label: '20' },
		{ value: '50', label: '50' }
	];

	function buildURL(overrides: Record<string, string | null>) {
		const url = new URL(page.url);
		for (const [key, value] of Object.entries(overrides)) {
			if (value === null || value === '') url.searchParams.delete(key);
			else url.searchParams.set(key, value);
		}
		return url.toString();
	}

	function statusBadgeClass(status: string): string {
		switch (status) {
			case 'pending':
				return 'bg-[color-mix(in_srgb,var(--color-warning)_15%,transparent)] text-[var(--color-warning)]';
			case 'reviewing':
				return 'bg-[color-mix(in_srgb,var(--color-info)_15%,transparent)] text-[var(--color-info)]';
			case 'actioned':
				return 'bg-[color-mix(in_srgb,var(--color-success)_15%,transparent)] text-[var(--color-success)]';
			case 'dismissed':
				return 'bg-[color-mix(in_srgb,var(--color-muted)_15%,transparent)] text-[var(--color-muted)]';
			default:
				return 'bg-[color-mix(in_srgb,var(--color-muted)_15%,transparent)] text-[var(--color-muted)]';
		}
	}

	function statusLabel(status: string): string {
		switch (status) {
			case 'pending':
				return t('my_reports.status_pending', 'Pending');
			case 'reviewing':
				return t('my_reports.status_reviewing', 'Reviewing');
			case 'actioned':
				return t('my_reports.status_actioned', 'Actioned');
			case 'dismissed':
				return t('my_reports.status_dismissed', 'Dismissed');
			default:
				return status;
		}
	}

	function entityTypeLabel(entityType: string): string {
		switch (entityType) {
			case 'content':
				return t('my_reports.entity_content', 'Content');
			case 'comment':
				return t('my_reports.entity_comment', 'Comment');
			case 'user':
				return t('my_reports.entity_user', 'User');
			case 'space':
				return t('my_reports.entity_space', 'Space');
			case 'message':
				return t('my_reports.entity_message', 'Message');
			case 'source':
				return t('my_reports.entity_source', 'Source');
			case 'forum_reply':
				return t('my_reports.entity_forum_reply', 'Forum reply');
			default:
				return entityType;
		}
	}

	/** Deep-link to the reported entity on the public site, mirroring the
	 *  moderation-queue resolution (comments/replies link their parent). */
	function entityHref(report: MyReport): string | null {
		const slug = report.entity_slug;
		if (!slug) return null;
		switch (report.entity_type) {
			case 'user':
				return `/u/${slug}`;
			case 'space':
				return `/spaces/${slug}`;
			case 'source':
				return `/sources/${slug}`;
			case 'forum_reply':
				return slug ? `/forum/${slug}` : null;
			case 'message':
				return null;
			default:
				if (report.thread_slug) return `/forum/${report.thread_slug}`;
				return contentHref({ slug, content_type: report.entity_content_type });
		}
	}
</script>

<svelte:head>
	<title>{title}</title>
	<meta name="robots" content="noindex" />
</svelte:head>

<div class="mx-auto w-full max-w-3xl">
	<div class="mx-4 border-b border-[var(--color-border)] px-4 py-3">
		<h1 class="flex items-center gap-2 text-lg font-semibold text-[var(--color-text)]">
			<Flag size={20} /> {t('my_reports.heading', 'My Reports')}
		</h1>
		<p class="mt-1 text-xs text-[var(--color-muted)]">
			{t('my_reports.description', 'Track the status of content you have reported.')}
		</p>
	</div>

	<div class="p-4">
		<div class="mb-3 flex items-center justify-end gap-2">
			<AutocompleteSelect
				class="w-20"
				value={String(pageSize)}
				options={pageSizeOptions}
				ariaLabel={t('my_reports.page_size', 'Rows per page')}
				onValueChange={(v) => goto(buildURL({ page_size: v, page: null }))}
			/>
		</div>
		{#if data.loadError}
			<p class="rounded-[var(--radius)] bg-red-500/10 px-3 py-2 text-sm text-red-500" role="alert">
				{t('my_reports.load_error', 'Failed to load your reports. Please try again later.')}
			</p>
		{:else if reports.length === 0}
			<EmptyState
				icon={FileText}
				title={t('my_reports.empty', 'No reports submitted')}
				body={t('my_reports.empty_body', 'Content you report will appear here with its review status.')}
			/>
		{:else}
			<div class="space-y-3">
				{#each reports as report (report.id)}
					<div
						class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4"
					>
						<div class="flex items-start justify-between gap-3">
							<div class="min-w-0 flex-1">
								<div class="flex items-center gap-2">
									<span class="text-sm font-medium text-[var(--color-text)]">
										{entityTypeLabel(report.entity_type)}
									</span>
									<span class="text-xs text-[var(--color-muted)]">
										&middot; {t('my_reports.reason', 'Reason')}: {report.reason}
									</span>
								</div>
								{#if entityHref(report) && report.entity_title}
									<a
										href={entityHref(report)}
										class="mt-1 block truncate text-sm text-[var(--color-primary)] hover:underline"
									>{report.entity_title}</a>
								{:else if report.entity_title}
									<p class="mt-1 truncate text-sm text-[var(--color-text-subtle)]">{report.entity_title}</p>
								{/if}
								{#if report.notes}
									<p class="mt-1 text-sm text-[var(--color-text-subtle)]">{report.notes}</p>
								{/if}
								<p class="mt-1 text-xs text-[var(--color-muted)]">
									{t('my_reports.submitted', 'Submitted')} <RelativeTime iso={report.created_at} />
								</p>
								{#if report.resolution_note}
									<div
										class="mt-2 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-background)] p-2"
									>
										<p class="text-xs font-medium text-[var(--color-muted)]">
											{t('my_reports.resolution', 'Resolution')}
										</p>
										<p class="mt-0.5 text-sm text-[var(--color-text)]">{report.resolution_note}</p>
									</div>
								{/if}
							</div>
							<span
								class="shrink-0 rounded-full px-2.5 py-0.5 text-xs font-medium {statusBadgeClass(report.status)}"
							>
								{statusLabel(report.status)}
							</span>
						</div>
						{#if report.status === 'dismissed'}
							<div class="mt-3 flex justify-end">
								<AppealButton
									entityType="content_report"
									entityId={report.id}
									label={t('appeals.appeal_dismissal', 'Appeal dismissal')}
									class="!px-3 !py-1.5 text-xs"
								/>
							</div>
						{/if}
					</div>
				{/each}
			</div>
			{#if showPagination}
				<div class="mt-4 flex justify-center">
					<PaginationControls
						page={pageNum}
						hasMore={data.hasMore}
						onPageChange={(p) => goto(buildURL({ page: String(p) }))}
					/>
				</div>
			{/if}
		{/if}
	</div>
</div>
