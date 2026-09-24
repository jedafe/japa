<script lang="ts">
	import { t } from '$lib/i18n';
	import { page } from '$app/state';
	import { Briefcase, FileText } from 'lucide-svelte';
	import { goto } from '$app/navigation';
	import { EmptyState, PaginationControls, RelativeTime } from '$lib/components/ui';
	import type { JobApplication } from '$lib/types';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();

	const title = $derived(t('jobs.my_applications', 'My Applications — JapaConnect'));
	const applications = $derived<JobApplication[]>(data.applications);
	const meta = $derived(data.meta);

	// Statuses mirror the job_applications CHECK constraint (Section 63):
	// applied, reviewing, shortlisted, rejected, offered, withdrawn.
	const statusColor = (status: string): string => {
		switch (status) {
			case 'offered': return 'text-[var(--color-success)]';
			case 'shortlisted': return 'text-[var(--color-info)]';
			case 'rejected': return 'text-[var(--color-danger)]';
			case 'reviewing': return 'text-[var(--color-info)]';
			default: return 'text-[var(--color-muted)]';
		}
	};
</script>

<svelte:head>
	<title>{title}</title>
	<meta name="robots" content="noindex" />
</svelte:head>

<div class="mx-auto w-full max-w-3xl">
	<div class="border-b border-[var(--color-border)] px-4 py-3">
		<h1 class="flex items-center gap-2 text-lg font-semibold text-[var(--color-text)]">
			<FileText size={20} /> {t('jobs.my_applications', 'My Applications')}
		</h1>
	</div>

	<div class="p-4">
		{#if applications.length === 0}
			<EmptyState
				icon={Briefcase}
				title={t('jobs.no_applications', 'No applications yet')}
				body={t('jobs.no_applications_body', 'Browse the jobs board to apply.')}
				action={{ label: t('jobs.browse', 'Browse jobs'), href: '/jobs' }}
			/>
		{:else}
			<div class="space-y-3">
				{#each applications as app (app.id)}
					<div class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
						<div class="flex items-start justify-between gap-3">
							<div class="min-w-0 flex-1">
								{#if app.job_slug}
									<a href="/jobs/{app.job_slug}" class="text-sm font-medium text-[var(--color-text)] hover:underline">
										{app.job_title || t('jobs.untitled', 'Untitled job')}
									</a>
								{:else}
									<span class="text-sm font-medium text-[var(--color-text)]">
										{app.job_title || t('jobs.untitled', 'Untitled job')}
									</span>
								{/if}
								<p class="mt-1 text-xs text-[var(--color-muted)]">
									{t('jobs.applied_on', 'Applied')} <RelativeTime iso={app.applied_at} />
								</p>
							</div>
							<span class="shrink-0 text-xs font-medium capitalize {statusColor(app.status)}">
								{t(`jobs.status_${app.status}`, app.status)}
							</span>
						</div>
					</div>
				{/each}
			</div>

			{#if meta?.total && meta.total > (meta.limit ?? 20)}
				<div class="mt-4">
					<PaginationControls
						page={data.currentPage}
						totalPages={Math.ceil((meta.total ?? 0) / (meta.limit ?? 20))}
						onPageChange={(p) => {
							const params = new URLSearchParams(page.url.searchParams);
							params.set('page', String(p));
							goto(`/me/applications?${params.toString()}`);
						}}
					/>
				</div>
			{/if}
		{/if}
	</div>
</div>
