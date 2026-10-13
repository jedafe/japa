<script lang="ts">
	import { t } from '$lib/i18n';
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { enhance } from '$app/forms';
	import { Briefcase, FileText, ArrowLeft } from 'lucide-svelte';
	import { EmptyState, PaginationControls, RelativeTime } from '$lib/components/ui';
	import SelectField from '$lib/components/ui/SelectField.svelte';
	import { safeHttpUrl } from '$lib/utils/safeUrl';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();

	const org = $derived(data.org);
	const job = $derived(data.job);
	const applications = $derived(data.applications);
	const meta = $derived(data.meta);
	const statusFilter = $derived(data.statusFilter);

	// Per-row status drafts so changing the select does not mutate the
	// loaded list; the hidden input submits the draft value.
	let statusDrafts = $state<Record<string, string>>({});

	const title = $derived(
		`${job?.title ?? t('jobs.applications_title', 'Job applications')} — ${org.name}`
	);

	const statusOptions = $derived(
		['applied', 'reviewing', 'shortlisted', 'rejected', 'offered', 'withdrawn'].map((s) => ({
			value: s,
			label: t(`jobs.status_${s}`, s)
		}))
	);
	const filterOptions = $derived([
		{ value: '', label: t('jobs.all_statuses', 'All statuses') },
		...statusOptions
	]);

	const statusColor = (status: string): string => {
		switch (status) {
			case 'offered':
				return 'text-[var(--color-success)]';
			case 'shortlisted':
				return 'text-[var(--color-info)]';
			case 'rejected':
				return 'text-[var(--color-danger)]';
			case 'reviewing':
				return 'text-[var(--color-warning,var(--color-info))]';
			default:
				return 'text-[var(--color-muted)]';
		}
	};

	function applyStatusFilter(v: string) {
		const params = new URLSearchParams(page.url.searchParams);
		if (v) params.set('status', v);
		else params.delete('status');
		params.delete('page');
		goto(`${page.url.pathname}?${params.toString()}`, { keepFocus: true, noScroll: true });
	}
</script>

<svelte:head>
	<title>{title} — JapaConnect</title>
	<meta name="robots" content="noindex" />
</svelte:head>

<div class="mx-auto w-full max-w-3xl">
	<div class="mx-4 border-b border-[var(--color-border)] px-4 py-3">
		<a href="/org/{org.slug}?tab=jobs" class="flex items-center gap-1 text-xs text-[var(--color-muted)] hover:underline">
			<ArrowLeft size={12} /> {t('jobs.back_to_org_jobs', 'Back to org jobs')}
		</a>
		<h1 class="mt-2 flex items-center gap-2 text-lg font-semibold text-[var(--color-text)]">
			<FileText size={20} />
			{job?.title ?? t('jobs.applications_title', 'Job applications')}
		</h1>
		<p class="mt-0.5 text-xs text-[var(--color-muted)]">{org.name}</p>
	</div>

	<div class="mx-4 border-b border-[var(--color-border)] px-4 py-2">
		<div class="w-48">
			<SelectField value={statusFilter} options={filterOptions} onValueChange={applyStatusFilter} />
		</div>
	</div>

	<div class="p-4">
		{#if applications.length === 0}
			<EmptyState
				icon={Briefcase}
				title={t('jobs.no_applications', 'No applications yet')}
				body={t('jobs.no_applications_org_body', 'Applications submitted for this job will appear here.')}
			/>
		{:else}
			<div class="space-y-3">
				{#each applications as app (app.id)}
					<div class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
						<div class="flex items-start justify-between gap-3">
							<div class="min-w-0 flex-1">
								<span class="block truncate text-sm font-medium text-[var(--color-text)]">
									{app.applicant_username ?? app.applicant_email ?? app.applicant_id ?? t('jobs.unknown_applicant', 'Applicant')}
								</span>
								<p class="mt-1 text-xs text-[var(--color-muted)]">
									{t('jobs.applied_on', 'Applied')} <RelativeTime iso={app.applied_at} />
									{#if app.applicant_email && app.applicant_username}
										· {app.applicant_email}
									{/if}
								</p>
							</div>
							<span class="shrink-0 text-xs font-medium capitalize {statusColor(app.status)}">
								{t(`jobs.status_${app.status}`, app.status)}
							</span>
						</div>
						{#if app.submission}
							<div class="mt-3 space-y-1.5 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface-subtle)] p-3 text-xs">
								{#if typeof app.submission.resume_url === 'string' && safeHttpUrl(app.submission.resume_url)}
									<a
										href={safeHttpUrl(app.submission.resume_url)}
										target="_blank"
										rel="noopener noreferrer"
										class="inline-flex items-center gap-1.5 text-[var(--color-primary)] hover:underline"
									>
										<FileText size={12} />
										{typeof app.submission.resume_filename === 'string' && app.submission.resume_filename
											? app.submission.resume_filename
											: t('jobs.view_resume', 'View CV / Resume')}
									</a>
								{/if}
								{#if typeof app.submission.cover_letter === 'string' && app.submission.cover_letter}
									<p class="whitespace-pre-wrap text-[var(--color-text)]">
										<span class="font-medium text-[var(--color-muted)]">{t('jobs.cover_letter', 'Cover letter')}:</span>
										{app.submission.cover_letter}
									</p>
								{/if}
							</div>
						{/if}
						<form method="POST" action="?/updateStatus" use:enhance class="mt-3 flex flex-wrap items-end gap-2">
							<input type="hidden" name="app_id" value={app.id} />
							<div class="w-40">
								<SelectField
									value={statusDrafts[app.id] ?? app.status}
									options={statusOptions}
									onValueChange={(v) => (statusDrafts[app.id] = v)}
								/>
								<input type="hidden" name="status" value={statusDrafts[app.id] ?? app.status} />
							</div>
							<input
								type="text"
								name="employer_notes"
								value={app.employer_notes ?? ''}
								placeholder={t('jobs.employer_notes', 'Employer notes')}
								class="min-w-0 flex-1 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-1.5 text-sm text-[var(--color-text)]"
							/>
							<button
								type="submit"
								class="rounded-[var(--radius)] bg-[var(--color-primary)] px-3 py-1.5 text-xs font-medium text-[var(--color-primary-contrast)] hover:opacity-90"
							>
								{t('common.save', 'Save')}
							</button>
						</form>
					</div>
				{/each}
			</div>

			{#if meta?.total && meta.total > (meta.limit ?? 20)}
				<div class="mt-4">
					<PaginationControls
						page={meta.page ?? 1}
						totalPages={Math.ceil((meta.total ?? 0) / (meta.limit ?? 20))}
						onPageChange={(p) => {
							const params = new URLSearchParams(page.url.searchParams);
							params.set('page', String(p));
							goto(`${page.url.pathname}?${params.toString()}`);
						}}
					/>
				</div>
			{/if}
		{/if}
	</div>
</div>
