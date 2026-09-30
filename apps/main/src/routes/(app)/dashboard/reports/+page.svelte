<script lang="ts">
	// /dashboard/reports — the user's own submitted content reports and their
	// current status (pending / reviewing / actioned / dismissed). Read-only;
	// users cannot edit or withdraw reports after submission.
	//
	// Ref: japaconnect-engineering/sections/51-content-reports-flagging.md
	import { t } from '$lib/i18n';
	import { page } from '$app/state';
	import { Flag, FileText } from 'lucide-svelte';
	import { EmptyState, RelativeTime } from '$lib/components/ui';
	import type { MyReport } from '$lib/types';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();

	const title = $derived(t('my_reports.title', 'My Reports — JapaConnect'));
	const reports = $derived<MyReport[]>(data.reports);

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
			default:
				return entityType;
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
		{#if reports.length === 0}
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
					</div>
				{/each}
			</div>
		{/if}
	</div>
</div>
